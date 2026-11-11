package execx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	}
	os.Exit(99)
}

func helper(t *testing.T, mode string) string {
	t.Helper()
	t.Setenv("EXECX_HELPER", mode)
	return os.Args[0]
}

func TestRunFeedsStdinAndKeepsBothStreamsApart(t *testing.T) {
	res, err := OS{}.Run(context.Background(), "hello", helper(t, "echo"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Stdout != "out:hello" || res.Stderr != "err:warning" || res.ExitCode != 0 {
		t.Errorf("result = %+v", res)
	}
}

func TestANonZeroExitIsAResultNotAnError(t *testing.T) {
	res, err := OS{}.Run(context.Background(), "", helper(t, "exit3"))
	if err != nil {
		t.Fatalf("Run returned an error for a program that ran and exited: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
}

func TestAProgramThatCannotBeStartedIsAnError(t *testing.T) {
	if _, err := (OS{}).Run(context.Background(), "", "sentinel-test-no-such-program"); err == nil {
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
	_, err := OS{}.Run(ctx, "", helper(t, "sleep"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the context error", err)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("Run returned after %s, want it to stop with the context", elapsed)
	}
}
