package execx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain lets this test binary stand in for an external program: with
// EXECX_HELPER set it behaves like one and never runs the tests. That keeps
// the tests independent of what is installed on the machine.
func TestMain(m *testing.M) {
	switch os.Getenv("EXECX_HELPER") {
	case "":
		os.Exit(m.Run())
	case "echo":
		in, _ := io.ReadAll(os.Stdin)
		fmt.Fprintf(os.Stdout, "out:%s", in)
		fmt.Fprint(os.Stderr, "err:warning")
		os.Exit(0)
	case "exit3":
		os.Exit(3)
	case "sleep":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "where":
		dir, _ := os.Getwd()
		fmt.Fprintf(os.Stdout, "%s|%s", filepath.Base(dir), os.Getenv("EXECX_VALUE"))
		os.Exit(0)
	case "flood":
		// More than any test accepts, then it lingers: only being stopped ends it.
		fmt.Fprint(os.Stdout, strings.Repeat("x", 1<<20))
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(99)
}

func helper(t *testing.T, mode string) string {
	t.Helper()
	t.Setenv("EXECX_HELPER", mode)
	return os.Args[0]
}

func TestRunFeedsStdinAndKeepsBothStreamsApart(t *testing.T) {
	res, err := OS{}.Run(context.Background(), Command{Name: helper(t, "echo"), Stdin: "hello"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Stdout != "out:hello" || res.Stderr != "err:warning" || res.ExitCode != 0 {
		t.Errorf("result = %+v", res)
	}
}

func TestANonZeroExitIsAResultNotAnError(t *testing.T) {
	res, err := OS{}.Run(context.Background(), Command{Name: helper(t, "exit3")})
	if err != nil {
		t.Fatalf("Run returned an error for a program that ran and exited: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
}

func TestAProgramThatCannotBeStartedIsAnError(t *testing.T) {
	if _, err := (OS{}).Run(context.Background(), Command{Name: "sentinel-test-no-such-program"}); err == nil {
		t.Fatal("Run reported success for a program that does not exist")
	}
	if _, err := (OS{}).LookPath("sentinel-test-no-such-program"); err == nil {
		t.Fatal("LookPath found a program that does not exist")
	}
}

func TestCancellingTheContextStopsTheProgram(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := OS{}.Run(ctx, Command{Name: helper(t, "sleep")})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the context error", err)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("Run returned after %s, want it to stop with the context", elapsed)
	}
}

func TestRunUsesTheDirectoryAndTheExtraEnvironment(t *testing.T) {
	dir := t.TempDir()
	res, err := OS{}.Run(context.Background(), Command{
		Name: helper(t, "where"),
		Dir:  dir,
		Env:  []string{"EXECX_VALUE=from the command"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := filepath.Base(dir) + "|from the command"; res.Stdout != want {
		t.Errorf("stdout = %q, want %q", res.Stdout, want)
	}
}

func TestAProgramThatPrintsMoreThanAcceptedIsStopped(t *testing.T) {
	started := time.Now()
	res, err := OS{}.Run(context.Background(), Command{Name: helper(t, "flood"), StdoutLimit: 4096})
	if !errors.Is(err, ErrOutputTooLarge) {
		t.Fatalf("err = %v, want ErrOutputTooLarge", err)
	}
	if res.Stdout != "" {
		t.Errorf("kept %d bytes of an output that was refused", len(res.Stdout))
	}
	if elapsed := time.Since(started); elapsed > 15*time.Second {
		t.Errorf("Run returned after %s, want the program stopped at once", elapsed)
	}
}

func TestOutputWithinTheLimitIsKept(t *testing.T) {
	res, err := OS{}.Run(context.Background(), Command{Name: helper(t, "echo"), Stdin: "hello", StdoutLimit: 9})
	if err != nil || res.Stdout != "out:hello" {
		t.Errorf("result = %+v, err = %v, want the 9 bytes kept", res, err)
	}
}
