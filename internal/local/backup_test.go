package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/execx"
)

// copier plays `docker compose cp` out of the container: the dump it copies
// holds the database's name, as a real one would hold its data.
type copier struct{ *docker }

func (c copier) Run(ctx context.Context, cmd execx.Command) (execx.Result, error) {
	res, err := c.docker.Run(ctx, cmd)
	if n := len(cmd.Args); err == nil && res.ExitCode == 0 && n >= 3 && cmd.Args[n-3] == "cp" && strings.HasPrefix(cmd.Args[n-2], "postgres:") {
		db := strings.TrimSuffix(filepath.Base(cmd.Args[n-2]), ".dump")
		if werr := os.WriteFile(cmd.Args[n-1], []byte("dump of "+db), 0o644); werr != nil {
			return execx.Result{}, werr
		}
	}
	return res, err
}

func installedForBackup(t *testing.T, d *docker) *Install {
	t.Helper()
	install, _ := newInstall(t, d)
	install.Runner = copier{d}
	if err := os.MkdirAll(install.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(install.Dir, composeFile), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return install
}

var at = time.Date(2026, 11, 18, 19, 3, 52, 0, time.UTC)

func TestABackupDumpsTheThreeDatabasesIntoAVerifiedFolder(t *testing.T) {
	d := &docker{}
	install := installedForBackup(t, d)
	parent := filepath.Join(t.TempDir(), "backups")

	path, err := install.Backup(context.Background(), parent, at)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(parent, "sentinel-20261118T190352Z") {
		t.Errorf("path = %s", path)
	}
	if err := VerifyBackup(path); err != nil {
		t.Errorf("the backup does not verify: %v", err)
	}
	for _, db := range Databases {
		if data, _ := os.ReadFile(filepath.Join(path, db+".dump")); string(data) != "dump of "+db {
			t.Errorf("%s.dump = %q", db, data)
		}
		if d.ran("pg_dump --username sentinel --format=custom --no-owner --no-privileges --file /tmp/sentinel-backup/"+db+".dump "+db) != 1 {
			t.Errorf("%s was not dumped as central dumps it:\n%s", db, strings.Join(d.calls, "\n"))
		}
	}
	if d.ran("up -d --wait postgres") != 1 || d.ran("rm -rf /tmp/sentinel-backup") != 1 {
		t.Errorf("the database was not started first or the dumps were left in it:\n%s", strings.Join(d.calls, "\n"))
	}
	entries, _ := os.ReadDir(parent)
	if len(entries) != 1 {
		t.Errorf("the backups folder holds %d entries, want only the backup", len(entries))
	}
}

func TestAFailedBackupLeavesNothingThatLooksLikeABackup(t *testing.T) {
	d := &docker{rules: []rule{{contains: "pg_dump", result: execx.Result{ExitCode: 1, Stderr: "pg_dump: error: connection failed"}}}}
	install := installedForBackup(t, d)
	parent := filepath.Join(t.TempDir(), "backups")

	if _, err := install.Backup(context.Background(), parent, at); err == nil || !strings.Contains(err.Error(), "connection failed") {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("a failed backup left %v", entries)
	}
	if d.ran("rm -rf /tmp/sentinel-backup") != 1 {
		t.Error("the container's dump folder was not cleaned after a failure")
	}
}

func TestABackupNeedsAnInstallation(t *testing.T) {
	install, _ := newInstall(t, &docker{})
	if _, err := install.Backup(context.Background(), t.TempDir(), at); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("err = %v", err)
	}
}

// backupFolder makes a backup the way Backup does, without Docker.
func backupFolder(t *testing.T) string {
	t.Helper()
	install := installedForBackup(t, &docker{})
	path, err := install.Backup(context.Background(), t.TempDir(), at)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestADamagedOrIncompleteBackupIsRefused(t *testing.T) {
	tampered := backupFolder(t)
	if err := os.WriteFile(filepath.Join(tampered, "temporal.dump"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBackup(tampered); err == nil || !strings.Contains(err.Error(), "temporal.dump") {
		t.Errorf("tampered: %v", err)
	}

	missing := backupFolder(t)
	os.Remove(filepath.Join(missing, "sentinel.dump"))
	if err := VerifyBackup(missing); err == nil || !strings.Contains(err.Error(), "sentinel.dump") {
		t.Errorf("missing dump: %v", err)
	}

	if err := VerifyBackup(t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a Sentinel backup") {
		t.Errorf("no sums: %v", err)
	}
}

func TestARestoreChecksTheBackupBeforeTouchingAnything(t *testing.T) {
	backup := backupFolder(t)
	os.WriteFile(filepath.Join(backup, "sentinel.dump"), []byte("changed"), 0o600)
	d := &docker{}
	install := installedForBackup(t, d)

	if err := install.Restore(context.Background(), backup); err == nil {
		t.Fatal("a damaged backup was restored")
	}
	if len(d.calls) != 0 {
		t.Errorf("Docker was used before the backup was checked: %v", d.calls)
	}
}

func TestARestoreStopsSentinelAndReplacesEachDatabase(t *testing.T) {
	backup := backupFolder(t)
	d := &docker{}
	install := installedForBackup(t, d)

	if err := install.Restore(context.Background(), backup); err != nil {
		t.Fatal(err)
	}
	order := []string{"compose.yaml stop", "up -d --wait postgres"}
	for _, db := range Databases {
		order = append(order,
			"cp "+filepath.Join(backup, db+".dump")+" postgres:/tmp/sentinel-backup/"+db+".dump",
			"pg_restore --username sentinel --dbname "+db+" --clean --if-exists --no-owner --no-privileges --exit-on-error /tmp/sentinel-backup/"+db+".dump")
	}
	at := 0
	for _, c := range d.calls {
		if at < len(order) && strings.Contains(c, order[at]) {
			at++
		}
	}
	if at != len(order) {
		t.Errorf("missing or out of order from %q:\n%s", order[at], strings.Join(d.calls, "\n"))
	}
	if d.ran("up -d --wait --wait-timeout") != 0 {
		t.Error("the restore started Sentinel again; it must leave it stopped")
	}
}

func TestAFailedRestoreSaysSentinelStaysStopped(t *testing.T) {
	backup := backupFolder(t)
	d := &docker{rules: []rule{{contains: "pg_restore", result: execx.Result{ExitCode: 1, Stderr: "pg_restore: error: could not execute query"}}}}
	install := installedForBackup(t, d)

	err := install.Restore(context.Background(), backup)
	if err == nil || !strings.Contains(err.Error(), "stays stopped") || !strings.Contains(err.Error(), "could not execute query") {
		t.Errorf("err = %v", err)
	}
	if d.ran("pg_restore") != 1 {
		t.Errorf("the restore went on after a database failed: %d pg_restore", d.ran("pg_restore"))
	}
}
