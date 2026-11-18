package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
	"github.com/Chisu-Technologies/sentinel-dev/internal/local"
)

// fakeBackup writes a folder that verifies as a backup.
func fakeBackup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	var sums strings.Builder
	for _, db := range local.Databases {
		data := []byte("dump of " + db)
		if err := os.WriteFile(filepath.Join(dir, db+".dump"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&sums, "%s  %s.dump\n", hex.EncodeToString(sum[:]), db)
	}
	if err := os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(sums.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func restore(t *testing.T, fake *execxtest.Fake, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), append([]string{"restore"}, args...), Env{
		Stdout: &stdout, Stderr: &stderr, Runner: fake, Build: buildinfo.Info{Version: "v0.0.0-test"},
		Getenv: func(name string) string {
			if name == "SENTINEL_HOME" {
				return filepath.Join(t.TempDir(), "local")
			}
			return ""
		},
	})
	return result{code, stdout.String(), stderr.String()}
}

func TestRestoreNeedsYesAndTouchesNothingWithoutIt(t *testing.T) {
	fake := &execxtest.Fake{}
	got := restore(t, fake, fakeBackup(t))
	if got.code != ExitUsage || !strings.Contains(got.stderr, "--yes") {
		t.Errorf("status = %d, stderr = %q", got.code, got.stderr)
	}
	if len(fake.Calls) != 0 {
		t.Errorf("Docker was used without --yes: %+v", fake.Calls)
	}
}

func TestRestoreRefusesADamagedBackupEvenWithoutYes(t *testing.T) {
	backup := fakeBackup(t)
	if err := os.WriteFile(filepath.Join(backup, "temporal.dump"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := &execxtest.Fake{}
	got := restore(t, fake, "--yes", backup)
	if got.code != ExitFailed || !strings.Contains(got.stderr, "does not match its sum") {
		t.Errorf("status = %d, stderr = %q", got.code, got.stderr)
	}
	if len(fake.Calls) != 0 {
		t.Errorf("Docker was used for a damaged backup: %+v", fake.Calls)
	}
}

func TestRestoreTakesExactlyOneBackup(t *testing.T) {
	for _, args := range [][]string{{"--yes"}, {"--yes", "a", "b"}} {
		if got := restore(t, &execxtest.Fake{}, args...); got.code != ExitUsage {
			t.Errorf("sentinel restore %q: status = %d", args, got.code)
		}
	}
}

func TestBackupTakesNoArguments(t *testing.T) {
	fake := &execxtest.Fake{}
	if got := runIn(t, fake, "", "backup", "somewhere"); got.code != ExitUsage {
		t.Errorf("status = %d", got.code)
	}
	if len(fake.Calls) != 0 {
		t.Errorf("a rejected command line still ran %+v", fake.Calls)
	}
}
