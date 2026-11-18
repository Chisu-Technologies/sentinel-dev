package local

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Databases are the three an installation keeps, as central backs them up
// (deploy/production/backup-postgres.sh in Sentinel): the platform and
// Temporal's two.
var Databases = []string{"sentinel", "temporal", "temporal_visibility"}

const (
	sumsFile = "SHA256SUMS"
	// containerDumpDir is where a dump waits inside the postgres container
	// while it is copied. A dump never passes through this process's memory.
	containerDumpDir = "/tmp/sentinel-backup"
	dbUser           = "sentinel"
)

// Backup dumps the three databases into a new folder under parent and returns
// it. The folder appears only when every dump is copied and summed: a run
// that fails leaves a hidden temporary folder at most, never a backup that
// looks whole and is not. Nothing older is deleted.
//
// The dumps do not carry the secrets in Dir/.env: the master key in it is
// what reads the encrypted columns, so a restore needs that same file.
func (i *Install) Backup(ctx context.Context, parent string, now time.Time) (string, error) {
	if err := i.ready(ctx); err != nil {
		return "", err
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	stamp := now.UTC().Format("20060102T150405Z")
	final := filepath.Join(parent, "sentinel-"+stamp)
	if _, err := os.Lstat(final); err == nil {
		return "", fmt.Errorf("%s already exists", final)
	}
	temporary, err := os.MkdirTemp(parent, ".sentinel-"+stamp+".")
	if err != nil {
		return "", err
	}
	done := false
	defer func() {
		if !done {
			os.RemoveAll(temporary)
		}
	}()

	// A stopped installation can be backed up: only the database is started,
	// and one that runs is left as it is.
	if _, err := i.run(ctx, "starting the database", i.compose("up", "-d", "--wait", "postgres")); err != nil {
		return "", err
	}
	if _, err := i.run(ctx, "preparing the dump folder", i.compose("exec", "-T", "postgres", "mkdir", "-p", containerDumpDir)); err != nil {
		return "", err
	}
	defer i.Runner.Run(context.WithoutCancel(ctx), i.compose("exec", "-T", "postgres", "rm", "-rf", containerDumpDir))

	var sums strings.Builder
	for _, db := range Databases {
		i.say("Dumping %s", db)
		inside := containerDumpDir + "/" + db + ".dump"
		if _, err := i.run(ctx, "dumping "+db, i.compose("exec", "-T", "postgres",
			"pg_dump", "--username", dbUser, "--format=custom", "--no-owner", "--no-privileges", "--file", inside, db)); err != nil {
			return "", err
		}
		local := filepath.Join(temporary, db+".dump")
		if _, err := i.run(ctx, "copying the dump of "+db, i.compose("cp", "postgres:"+inside, local)); err != nil {
			return "", err
		}
		if err := os.Chmod(local, 0o600); err != nil {
			return "", err
		}
		sum, err := fileSHA256(local)
		if err != nil {
			return "", fmt.Errorf("the dump of %s was not copied: %w", db, err)
		}
		fmt.Fprintf(&sums, "%s  %s.dump\n", sum, db)
	}
	if err := os.WriteFile(filepath.Join(temporary, sumsFile), []byte(sums.String()), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, final); err != nil {
		return "", err
	}
	done = true
	return final, nil
}

// Restore replaces the three databases with a backup. The backup is checked
// against its own sums before anything is touched: a restore from a damaged
// backup would leave nothing to go back to. Every service is stopped and only
// postgres is started, so nothing writes while it runs. It leaves Sentinel
// stopped; `sentinel up` starts it again.
func (i *Install) Restore(ctx context.Context, backup string) error {
	if err := VerifyBackup(backup); err != nil {
		return err
	}
	if err := i.ready(ctx); err != nil {
		return err
	}
	i.say("Stopping Sentinel")
	if _, err := i.run(ctx, "stopping Sentinel", i.compose("stop")); err != nil {
		return err
	}
	if _, err := i.run(ctx, "starting the database", i.compose("up", "-d", "--wait", "postgres")); err != nil {
		return err
	}
	if _, err := i.run(ctx, "preparing the restore folder", i.compose("exec", "-T", "postgres", "mkdir", "-p", containerDumpDir)); err != nil {
		return err
	}
	defer i.Runner.Run(context.WithoutCancel(ctx), i.compose("exec", "-T", "postgres", "rm", "-rf", containerDumpDir))
	for _, db := range Databases {
		i.say("Restoring %s", db)
		inside := containerDumpDir + "/" + db + ".dump"
		if _, err := i.run(ctx, "copying the dump of "+db, i.compose("cp", filepath.Join(backup, db+".dump"), "postgres:"+inside)); err != nil {
			return err
		}
		if _, err := i.run(ctx, "restoring "+db, i.compose("exec", "-T", "postgres",
			"pg_restore", "--username", dbUser, "--dbname", db,
			"--clean", "--if-exists", "--no-owner", "--no-privileges", "--exit-on-error", inside)); err != nil {
			return fmt.Errorf("%w; Sentinel stays stopped on purpose", err)
		}
	}
	return nil
}

// VerifyBackup checks that a folder holds the three dumps and that each
// matches its line in SHA256SUMS.
func VerifyBackup(dir string) error {
	f, err := os.Open(filepath.Join(dir, sumsFile))
	if err != nil {
		return fmt.Errorf("%s is not a Sentinel backup: it has no %s", dir, sumsFile)
	}
	defer f.Close()
	want := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		sum, name, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "  ")
		if ok {
			want[name] = sum
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, db := range Databases {
		name := db + ".dump"
		expected, ok := want[name]
		if !ok {
			return fmt.Errorf("the backup in %s has no sum for %s", dir, name)
		}
		got, err := fileSHA256(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("the backup in %s has no %s", dir, name)
		}
		if got != expected {
			return fmt.Errorf("%s in %s does not match its sum; refusing to restore from it", name, dir)
		}
	}
	return nil
}

// ready checks Docker and that this folder holds an installation.
func (i *Install) ready(ctx context.Context) error {
	if err := i.CheckDocker(ctx); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(i.Dir, composeFile)); err != nil {
		return errors.New("Sentinel is not installed on this machine")
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
