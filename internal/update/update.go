// Package update replaces the running sentinel with a newer release, after the
// same checks as the one-line installer: the archive must match SHA256SUMS,
// and SHA256SUMS must carry the signature of this repository's release
// workflow whenever cosign is installed to check it.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// Repo is the repository whose releases are installed. The installers name
// the same one.
const Repo = "Chisu-Technologies/sentinel-dev"

const (
	// signerPattern and signerIssuer are the identity the installers require of
	// the signature: the release workflow of Repo, run for a version tag.
	signerPattern = `^https://github\.com/Chisu-Technologies/sentinel-dev/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+$`
	signerIssuer  = "https://token.actions.githubusercontent.com"
	// maxDownload bounds every file read from a release.
	maxDownload = 64 << 20
)

var tagPattern = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

// Source is where releases are read. Tests point it at a local server.
type Source struct {
	// API answers /repos/<Repo>/releases/latest.
	API string
	// Download serves <Download>/<tag>/<file>.
	Download string
	HTTP     *http.Client
}

// GitHub is the source the installers use.
func GitHub(client *http.Client) Source {
	return Source{
		API:      "https://api.github.com",
		Download: "https://github.com/" + Repo + "/releases/download",
		HTTP:     client,
	}
}

// Latest returns the tag of the latest release.
func (s Source) Latest(ctx context.Context) (string, error) {
	data, err := s.get(ctx, s.API+"/repos/"+Repo+"/releases/latest", 1<<20)
	if err != nil {
		return "", fmt.Errorf("could not read the latest release: %w", err)
	}
	var release struct {
		Tag string `json:"tag_name"`
	}
	if json.Unmarshal(data, &release) != nil || !tagPattern.MatchString(release.Tag) {
		return "", errors.New("the latest release has no version tag such as v1.2.3")
	}
	return release.Tag, nil
}

// Newer reports whether tag is a later version than current. A current version
// that is not a release tag (a development build) is never older than anything:
// it is replaced only by the installer, on purpose.
func Newer(current, tag string) bool {
	a, okA := parse(current)
	b, okB := parse(tag)
	if !okA || !okB {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return b[i] > a[i]
		}
	}
	return false
}

// IsRelease reports whether version is a release tag.
func IsRelease(version string) bool {
	_, ok := parse(version)
	return ok
}

func parse(tag string) ([3]int, bool) {
	m := tagPattern.FindStringSubmatch(tag)
	if m == nil {
		return [3]int{}, false
	}
	var out [3]int
	for i := range out {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// Options says how Install checks and where it writes.
type Options struct {
	GOOS, GOARCH string
	// Executable is the binary to replace.
	Executable string
	// Runner runs cosign, when it is installed.
	Runner execx.Runner
	// RequireSignature stops the update when cosign is not installed.
	RequireSignature bool
	// TempDir holds the downloads. Empty means the system's.
	TempDir string
}

// Result says what Install checked.
type Result struct {
	SignatureChecked bool
}

// Install downloads release tag, checks it and replaces opts.Executable with
// the binary it carries. Nothing is replaced unless every check passed, and
// the new binary is renamed into place, so an interruption leaves the old one
// whole.
func (s Source) Install(ctx context.Context, tag string, opts Options) (Result, error) {
	if !tagPattern.MatchString(tag) {
		return Result{}, errors.New("not a release tag")
	}
	asset, binary := assetName(opts.GOOS, opts.GOARCH)
	if asset == "" {
		return Result{}, fmt.Errorf("there is no release of sentinel for %s/%s", opts.GOOS, opts.GOARCH)
	}
	base := s.Download + "/" + tag + "/"
	sums, err := s.get(ctx, base+"SHA256SUMS", 1<<20)
	if err != nil {
		return Result{}, fmt.Errorf("could not read SHA256SUMS of %s: %w", tag, err)
	}
	archive, err := s.get(ctx, base+asset, maxDownload)
	if err != nil {
		return Result{}, fmt.Errorf("could not download %s: %w", asset, err)
	}

	var result Result
	if _, err := opts.Runner.LookPath("cosign"); err != nil {
		if opts.RequireSignature {
			return Result{}, errors.New("SENTINEL_REQUIRE_SIGNATURE is set and cosign is not installed. Nothing was changed")
		}
	} else {
		bundle, err := s.get(ctx, base+"SHA256SUMS.sigstore.json", 1<<20)
		if err != nil {
			return Result{}, errors.New("the release has no signature for SHA256SUMS. Nothing was changed")
		}
		if err := verifySignature(ctx, opts, sums, bundle); err != nil {
			return Result{}, err
		}
		result.SignatureChecked = true
	}

	want := listed(sums, asset)
	if want == "" {
		return Result{}, fmt.Errorf("SHA256SUMS does not list %s. Nothing was changed", asset)
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return Result{}, fmt.Errorf("the checksum of %s does not match SHA256SUMS. Nothing was changed", asset)
	}

	content, err := extract(archive, asset, binary)
	if err != nil {
		return Result{}, err
	}
	return result, replace(opts.Executable, content)
}

func assetName(goos, goarch string) (asset, binary string) {
	if goarch != "amd64" && goarch != "arm64" {
		return "", ""
	}
	switch goos {
	case "windows":
		return "sentinel_windows_" + goarch + ".zip", "sentinel.exe"
	case "linux", "darwin":
		return "sentinel_" + goos + "_" + goarch + ".tar.gz", "sentinel"
	}
	return "", ""
}

// listed returns the checksum SHA256SUMS gives for name, as sha256sum writes it
// in text or binary mode.
func listed(sums []byte, name string) string {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return strings.ToLower(fields[0])
		}
	}
	return ""
}

func verifySignature(ctx context.Context, opts Options, sums, bundle []byte) error {
	dir, err := os.MkdirTemp(opts.TempDir, "sentinel-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	sumsPath := filepath.Join(dir, "SHA256SUMS")
	bundlePath := filepath.Join(dir, "SHA256SUMS.sigstore.json")
	if err := os.WriteFile(sumsPath, sums, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(bundlePath, bundle, 0o600); err != nil {
		return err
	}
	res, err := opts.Runner.Run(ctx, execx.Command{Name: "cosign", Args: []string{
		"verify-blob",
		"--bundle", bundlePath,
		"--certificate-identity-regexp", signerPattern,
		"--certificate-oidc-issuer", signerIssuer,
		sumsPath,
	}})
	if err != nil || res.ExitCode != 0 {
		return errors.New("the signature of SHA256SUMS is not valid. Nothing was changed")
	}
	return nil
}

// extract returns the one binary of the archive. Only a regular file at the
// top of the archive counts: a link or a path elsewhere is not the binary.
func extract(archive []byte, asset, binary string) ([]byte, error) {
	missing := fmt.Errorf("%s has no %s. Nothing was changed", asset, binary)
	if strings.HasSuffix(asset, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, missing
		}
		for _, file := range reader.File {
			if file.Name != binary || !file.Mode().IsRegular() {
				continue
			}
			rc, err := file.Open()
			if err != nil {
				return nil, missing
			}
			defer rc.Close()
			return readAtMost(rc)
		}
		return nil, missing
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, missing
	}
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err != nil {
			return nil, missing
		}
		if header.Name == binary && header.Typeflag == tar.TypeReg {
			return readAtMost(tr)
		}
	}
}

func readAtMost(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxDownload+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDownload {
		return nil, errors.New("the binary in the archive is larger than expected. Nothing was changed")
	}
	return data, nil
}

// replace puts content where the executable is. On Windows the running file
// cannot be overwritten but can be renamed, so it is moved aside first and
// removed on the next update.
func replace(executable string, content []byte) error {
	dir := filepath.Dir(executable)
	staged, err := os.CreateTemp(dir, ".sentinel.new.*")
	if err != nil {
		return fmt.Errorf("cannot write next to %s: %w", executable, err)
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	if _, err := staged.Write(content); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		return err
	}
	old := filepath.Join(dir, ".sentinel.old"+filepath.Ext(executable))
	_ = os.Remove(old)
	if err := os.Rename(executable, old); err != nil {
		return fmt.Errorf("cannot replace %s: %w", executable, err)
	}
	if err := os.Rename(stagedPath, executable); err != nil {
		// Put the old binary back: a failed update must not leave no command.
		_ = os.Rename(old, executable)
		return fmt.Errorf("cannot replace %s: %w", executable, err)
	}
	// Fails while the old binary runs on Windows; the next update removes it.
	_ = os.Remove(old)
	return nil
}

func (s Source) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sentinel-update")
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %d", resp.Request.URL.Host, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("the file is larger than expected")
	}
	return data, nil
}
