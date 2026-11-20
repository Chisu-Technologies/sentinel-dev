package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
)

func tarGz(t *testing.T, name string, content []byte, typeflag byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: typeflag}); err != nil {
		t.Fatal(err)
	}
	tw.Write(content)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func zipped(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(content)
	zw.Close()
	return buf.Bytes()
}

func sumOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// release serves one release: files under /download/<tag>/ and the latest tag.
func release(t *testing.T, tag string, files map[string][]byte) Source {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/"+Repo+"/releases/latest" {
			w.Write([]byte(`{"tag_name":"` + tag + `"}`))
			return
		}
		name, ok := strings.CutPrefix(r.URL.Path, "/download/"+tag+"/")
		data, found := files[name]
		if !ok || !found {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return Source{API: srv.URL, Download: srv.URL + "/download", HTTP: srv.Client()}
}

func installed(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(path, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func linuxOptions(path string, runner execx.Runner) Options {
	return Options{GOOS: "linux", GOARCH: "amd64", Executable: path, Runner: runner}
}

// nothingToCheck has neither gh nor cosign: only the checksum is checked.
var nothingToCheck = &execxtest.Fake{Missing: map[string]bool{"cosign": true, "gh": true}}

func TestNewerComparesReleaseVersionsOnly(t *testing.T) {
	cases := []struct {
		current, tag string
		want         bool
	}{
		{"v0.3.2", "v0.3.3", true},
		{"v0.3.3", "v0.3.3", false},
		{"v0.10.0", "v0.9.9", false},
		{"v0.9.9", "v0.10.0", true},
		{"v1.0.0", "v0.99.0", false},
		{"dev", "v9.9.9", false},
		{"v0.3.2", "latest", false},
	}
	for _, c := range cases {
		if got := Newer(c.current, c.tag); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.current, c.tag, got)
		}
	}
}

func TestLatestReadsTheTag(t *testing.T) {
	src := release(t, "v0.3.3", nil)
	tag, err := src.Latest(context.Background())
	if err != nil || tag != "v0.3.3" {
		t.Fatalf("tag %q, err %v", tag, err)
	}
}

func TestInstallReplacesTheBinaryWhenTheChecksumMatches(t *testing.T) {
	archive := tarGz(t, "sentinel", []byte("new binary"), tar.TypeReg)
	src := release(t, "v0.3.3", map[string][]byte{
		"sentinel_linux_amd64.tar.gz": archive,
		"SHA256SUMS":                  []byte(sumOf(archive) + "  sentinel_linux_amd64.tar.gz\n"),
	})
	path := installed(t)

	result, err := src.Install(context.Background(), "v0.3.3", linuxOptions(path, nothingToCheck))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new binary" {
		t.Errorf("binary = %q", got)
	}
	if result.SignatureChecked {
		t.Error("claimed a signature check without gh or cosign")
	}
}

func TestInstallOnWindowsReadsTheZip(t *testing.T) {
	archive := zipped(t, "sentinel.exe", []byte("new exe"))
	src := release(t, "v0.3.3", map[string][]byte{
		"sentinel_windows_amd64.zip": archive,
		"SHA256SUMS":                 []byte(sumOf(archive) + " *sentinel_windows_amd64.zip\n"),
	})
	path := filepath.Join(t.TempDir(), "sentinel.exe")
	os.WriteFile(path, []byte("old exe"), 0o755)

	_, err := src.Install(context.Background(), "v0.3.3", Options{GOOS: "windows", GOARCH: "amd64", Executable: path, Runner: nothingToCheck})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new exe" {
		t.Errorf("binary = %q", got)
	}
}

func TestInstallKeepsTheOldBinaryWhenTheChecksumDiffers(t *testing.T) {
	archive := tarGz(t, "sentinel", []byte("tampered"), tar.TypeReg)
	src := release(t, "v0.3.3", map[string][]byte{
		"sentinel_linux_amd64.tar.gz": archive,
		"SHA256SUMS":                  []byte(sumOf([]byte("something else")) + "  sentinel_linux_amd64.tar.gz\n"),
	})
	path := installed(t)

	_, err := src.Install(context.Background(), "v0.3.3", linuxOptions(path, nothingToCheck))
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old binary" {
		t.Errorf("binary = %q", got)
	}
}

func TestInstallChecksTheSignatureWhenCosignIsInstalled(t *testing.T) {
	archive := tarGz(t, "sentinel", []byte("new binary"), tar.TypeReg)
	files := map[string][]byte{
		"sentinel_linux_amd64.tar.gz": archive,
		"SHA256SUMS":                  []byte(sumOf(archive) + "  sentinel_linux_amd64.tar.gz\n"),
		"SHA256SUMS.sigstore.json":    []byte(`{}`),
	}

	// Without gh, the way the installers fall back.
	valid := &execxtest.Fake{Missing: map[string]bool{"gh": true}}
	result, err := release(t, "v0.3.3", files).Install(context.Background(), "v0.3.3", linuxOptions(installed(t), valid))
	if err != nil || !result.SignatureChecked {
		t.Fatalf("result %+v, err %v", result, err)
	}
	args := strings.Join(valid.Calls[0].Args, " ")
	if valid.Calls[0].Name != "cosign" || !strings.Contains(args, "--certificate-identity-regexp "+signerPattern) || !strings.Contains(args, "--certificate-oidc-issuer "+signerIssuer) {
		t.Errorf("cosign ran as %s %s", valid.Calls[0].Name, args)
	}

	invalid := &execxtest.Fake{Missing: map[string]bool{"gh": true}, Results: map[string]execx.Result{"cosign": {ExitCode: 1}}}
	path := installed(t)
	_, err = release(t, "v0.3.3", files).Install(context.Background(), "v0.3.3", linuxOptions(path, invalid))
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old binary" {
		t.Errorf("binary = %q", got)
	}
}

func TestInstallRequiringASignatureStopsWithoutCosign(t *testing.T) {
	archive := tarGz(t, "sentinel", []byte("new binary"), tar.TypeReg)
	src := release(t, "v0.3.3", map[string][]byte{
		"sentinel_linux_amd64.tar.gz": archive,
		"SHA256SUMS":                  []byte(sumOf(archive) + "  sentinel_linux_amd64.tar.gz\n"),
	})
	path := installed(t)
	opts := linuxOptions(path, nothingToCheck)
	opts.RequireSignature = true

	_, err := src.Install(context.Background(), "v0.3.3", opts)
	if err == nil || !strings.Contains(err.Error(), "SENTINEL_REQUIRE_SIGNATURE") {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old binary" {
		t.Errorf("binary = %q", got)
	}
}

func TestInstallTakesOnlyARegularFileNamedLikeTheBinary(t *testing.T) {
	archive := tarGz(t, "sentinel", nil, tar.TypeSymlink)
	src := release(t, "v0.3.3", map[string][]byte{
		"sentinel_linux_amd64.tar.gz": archive,
		"SHA256SUMS":                  []byte(sumOf(archive) + "  sentinel_linux_amd64.tar.gz\n"),
	})
	path := installed(t)

	_, err := src.Install(context.Background(), "v0.3.3", linuxOptions(path, nothingToCheck))
	if err == nil || !strings.Contains(err.Error(), "has no sentinel") {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old binary" {
		t.Errorf("binary = %q", got)
	}
}

func signedRelease(t *testing.T) Source {
	t.Helper()
	archive := tarGz(t, "sentinel", []byte("new binary"), tar.TypeReg)
	return release(t, "v0.3.3", map[string][]byte{
		"sentinel_linux_amd64.tar.gz": archive,
		"SHA256SUMS":                  []byte(sumOf(archive) + "  sentinel_linux_amd64.tar.gz\n"),
		"SHA256SUMS.sigstore.json":    []byte(`{}`),
	})
}

// The installers ask GitHub first, through the gh that Sentinel already needs;
// the update checks the same way, before cosign.
func TestInstallChecksGitHubsAttestationWithASignedInGh(t *testing.T) {
	gh := &execxtest.Fake{}
	path := installed(t)
	result, err := signedRelease(t).Install(context.Background(), "v0.3.3", linuxOptions(path, gh))
	if err != nil || !result.SignatureChecked || result.CheckedBy != "GitHub" {
		t.Fatalf("result %+v, err %v", result, err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new binary" {
		t.Errorf("binary = %q", got)
	}
	if len(gh.Calls) != 2 || strings.Join(gh.Calls[0].Args, " ") != "auth status" {
		t.Fatalf("calls %+v", gh.Calls)
	}
	verify := gh.Calls[1]
	args := strings.Join(verify.Args, " ")
	want := " --repo " + Repo + " --cert-identity-regex " + signerPattern + " --cert-oidc-issuer " + signerIssuer + " --deny-self-hosted-runners"
	if verify.Name != "gh" || !strings.HasPrefix(args, "attestation verify ") || !strings.HasSuffix(args, want) ||
		filepath.Base(verify.Args[2]) != "sentinel_linux_amd64.tar.gz" {
		t.Errorf("gh ran as %s %s", verify.Name, args)
	}
}

func TestInstallKeepsTheOldBinaryWhenGitHubHasNoValidAttestation(t *testing.T) {
	gh := &execxtest.Fake{Results: map[string]execx.Result{"gh auth status": {}, "gh": {ExitCode: 1}}}
	path := installed(t)
	_, err := signedRelease(t).Install(context.Background(), "v0.3.3", linuxOptions(path, gh))
	if err == nil || !strings.Contains(err.Error(), "no valid attestation") || !strings.Contains(err.Error(), "Nothing was changed") {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old binary" {
		t.Errorf("binary = %q", got)
	}
}

// A gh without a session answers nothing: it is not asked, and the update goes on
// as when there is no gh.
func TestInstallDoesNotAskAGhThatIsNotSignedIn(t *testing.T) {
	gh := &execxtest.Fake{Missing: map[string]bool{"cosign": true}, Results: map[string]execx.Result{"gh auth status": {ExitCode: 1}}}
	result, err := signedRelease(t).Install(context.Background(), "v0.3.3", linuxOptions(installed(t), gh))
	if err != nil || result.SignatureChecked {
		t.Fatalf("result %+v, err %v", result, err)
	}
	for _, call := range gh.Calls {
		if len(call.Args) > 0 && call.Args[0] == "attestation" {
			t.Errorf("asked a gh that is not signed in: %v", call.Args)
		}
	}
}

func TestInstallRequiringASignatureAcceptsASignedInGh(t *testing.T) {
	opts := linuxOptions(installed(t), &execxtest.Fake{Missing: map[string]bool{"cosign": true}})
	opts.RequireSignature = true
	if result, err := signedRelease(t).Install(context.Background(), "v0.3.3", opts); err != nil || result.CheckedBy != "GitHub" {
		t.Fatalf("result %+v, err %v", result, err)
	}
}
