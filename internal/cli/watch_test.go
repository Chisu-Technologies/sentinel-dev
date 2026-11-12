package cli

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo"
	"github.com/Chisu-Technologies/sentinel-dev/internal/execx/execxtest"
)

// watchServer plays one run: each read of the run answers the next status (the
// last repeats) and the log read that follows answers the matching entry of
// logs, where nil means the run has no log yet.
type watchServer struct {
	*httptest.Server
	mu        sync.Mutex
	requests  []string
	statuses  []string
	logs      []*string
	polls     int
	runStatus int
}

func str(s string) *string { return &s }

func newWatchServer(t *testing.T, statuses []string, logs []*string) *watchServer {
	t.Helper()
	w := &watchServer{statuses: statuses, logs: logs}
	w.Server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.requests = append(w.requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/runs/run-7":
			if w.runStatus != 0 {
				rw.WriteHeader(w.runStatus)
				io.WriteString(rw, `{"detail":"Run not found"}`)
				return
			}
			i := min(w.polls, len(w.statuses)-1)
			w.polls++
			io.WriteString(rw, `{"id":"run-7","status":"`+w.statuses[i]+`"}`)
		case "/api/runs/run-7/log":
			i := min(max(w.polls-1, 0), len(w.logs)-1)
			if len(w.logs) == 0 || w.logs[i] == nil {
				http.NotFound(rw, r)
				return
			}
			rw.Header().Set("Content-Type", "text/plain")
			io.WriteString(rw, *w.logs[i])
		default:
			http.NotFound(rw, r)
		}
	}))
	t.Cleanup(w.Close)
	return w
}

func (w *watchServer) requestCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.requests)
}

func watchVars(w *watchServer) map[string]string {
	return map[string]string{"SENTINEL_URL": w.URL, "SENTINEL_API_KEY": verifyKey}
}

// instantSleep returns at once and counts the waits.
func instantSleep(sleeps *int) func(context.Context, time.Duration) error {
	return func(ctx context.Context, d time.Duration) error {
		*sleeps++
		return ctx.Err()
	}
}

func TestWatchWrongArgumentsExitTwoWithoutARequest(t *testing.T) {
	w := newWatchServer(t, []string{"running"}, nil)
	for _, args := range [][]string{
		{"watch"},
		{"watch", "run-7", "run-8"},
		{"watch", "../run-7"},
		{"watch", "run 7"},
		{"watch", "run-7/log"},
		{"watch", strings.Repeat("a", 65)},
		{"watch", "--timeout", "0s", "run-7"},
		{"watch", "--timeout", "-1m", "run-7"},
		{"watch", "--timeout", "soon", "run-7"},
		{"watch", "--nope", "run-7"},
	} {
		got := command(t, watchVars(w), nil, args...)
		if got.code != ExitUsage || got.stdout != "" {
			t.Errorf("sentinel %q = %+v, want status 2 and no output", args, got)
		}
	}
	if n := w.requestCount(); n != 0 {
		t.Errorf("a wrong command line sent %d requests", n)
	}
}

func TestWatchNamesTheMissingVariableAndExitsOne(t *testing.T) {
	w := newWatchServer(t, []string{"running"}, nil)
	for _, name := range []string{"SENTINEL_URL", "SENTINEL_API_KEY"} {
		vars := watchVars(w)
		delete(vars, name)
		got := command(t, vars, nil, "watch", "run-7")
		if got.code != ExitFailed || !strings.Contains(got.stderr, name) {
			t.Errorf("without %s: %+v", name, got)
		}
	}
	if n := w.requestCount(); n != 0 {
		t.Errorf("a missing variable sent %d requests", n)
	}
}

func TestWatchPrintsEachStatusChangeOnceOnStderr(t *testing.T) {
	w := newWatchServer(t, []string{"queued", "queued", "running", "running", "completed"}, nil)
	var sleeps int
	got := command(t, watchVars(w), instantSleep(&sleeps), "watch", "run-7")
	want := "Run run-7: queued\nRun run-7: running\nRun run-7: completed\n"
	if got.stderr != want {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
	if got.code != ExitOK || got.stdout != "" || sleeps != 4 {
		t.Errorf("status %d, stdout %q, %d waits (want 4: one per poll after the first)", got.code, got.stdout, sleeps)
	}
}

func TestWatchPollsEveryFiveSecondsThroughTheInjectedSleep(t *testing.T) {
	w := newWatchServer(t, []string{"running", "completed"}, nil)
	var waited []time.Duration
	command(t, watchVars(w), func(ctx context.Context, d time.Duration) error {
		waited = append(waited, d)
		return nil
	}, "watch", "run-7")
	if len(waited) != 1 || waited[0] != 5*time.Second {
		t.Errorf("waits = %v, want one of 5s", waited)
	}
}

func TestWatchPrintsOnlyTheNewLogTextAcrossThreePolls(t *testing.T) {
	w := newWatchServer(t,
		[]string{"running", "running", "running", "completed"},
		[]*string{
			str("a\nb\n"),    // first chunk: all of it
			str("a\nb\nc\n"), // prefix case: the old chunk plus a line
			str("c\nd\n"),    // overlap case: the window slid, "c\n" was shown
			str("x\ny\n"),    // no overlap: all of it is new
		})
	var sleeps int
	got := command(t, watchVars(w), instantSleep(&sleeps), "watch", "run-7")
	if want := "a\nb\n" + "c\n" + "d\n" + "x\ny\n"; got.stdout != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
	if got.code != ExitOK {
		t.Errorf("status = %d", got.code)
	}
}

func TestWatchPrintsNothingWhenTheLogDidNotChange(t *testing.T) {
	w := newWatchServer(t, []string{"running", "running", "completed"}, []*string{str("a\n")})
	var sleeps int
	got := command(t, watchVars(w), instantSleep(&sleeps), "watch", "run-7")
	if got.stdout != "a\n" {
		t.Errorf("stdout = %q, want the log once", got.stdout)
	}
}

func TestWatchIgnoresALogThatIsNotThereYet(t *testing.T) {
	w := newWatchServer(t,
		[]string{"queued", "running", "completed"},
		[]*string{nil, str("started\n"), str("started\ndone\n")})
	var sleeps int
	got := command(t, watchVars(w), instantSleep(&sleeps), "watch", "run-7")
	if got.code != ExitOK || got.stdout != "started\ndone\n" {
		t.Errorf("got %+v", got)
	}
}

func TestWatchExitsZeroWhenTheRunCompletedAndOneForAnyOtherEnd(t *testing.T) {
	for status, want := range map[string]int{
		"completed": ExitOK,
		"failed":    ExitFailed,
		"blocked":   ExitFailed,
		"stopped":   ExitFailed,
		"cancelled": ExitFailed,
		"zombie":    ExitFailed,
	} {
		w := newWatchServer(t, []string{"running", status}, nil)
		var sleeps int
		got := command(t, watchVars(w), instantSleep(&sleeps), "watch", "run-7")
		if got.code != want || !strings.HasSuffix(got.stderr, "Run run-7: "+status+"\n") {
			t.Errorf("ending %s: %+v, want status %d", status, got, want)
		}
	}
}

func TestWatchARunThatAlreadyEndedPrintsItsLogAndDoesNotWait(t *testing.T) {
	w := newWatchServer(t, []string{"failed"}, []*string{str("boom\n")})
	var sleeps int
	got := command(t, watchVars(w), instantSleep(&sleeps), "watch", "run-7")
	if got.code != ExitFailed || got.stdout != "boom\n" || sleeps != 0 {
		t.Errorf("got %+v after %d waits", got, sleeps)
	}
}

func TestWatchARunThatDoesNotExistExitsOneWithAClearMessage(t *testing.T) {
	w := newWatchServer(t, []string{"running"}, nil)
	w.runStatus = http.StatusNotFound
	var sleeps int
	got := command(t, watchVars(w), instantSleep(&sleeps), "watch", "run-7")
	if got.code != ExitFailed || got.stdout != "" || sleeps != 0 {
		t.Fatalf("got %+v after %d waits", got, sleeps)
	}
	if !strings.Contains(got.stderr, "Run run-7 was not found on this server.") {
		t.Errorf("stderr = %q", got.stderr)
	}
}

func TestWatchATimeoutExitsOneAndNamesTheRun(t *testing.T) {
	w := newWatchServer(t, []string{"running"}, []*string{str("working\n")})
	calls := 0
	got := command(t, watchVars(w), func(ctx context.Context, d time.Duration) error {
		// The clock runs out on the third wait, as the real timer would.
		if calls++; calls == 3 {
			return context.DeadlineExceeded
		}
		return nil
	}, "watch", "--timeout", "10m", "run-7")
	if got.code != ExitFailed {
		t.Errorf("status = %d", got.code)
	}
	if !strings.Contains(got.stderr, "Gave up waiting after 10m0s.") || !strings.Contains(got.stderr, "Run run-7 continues on the server.") {
		t.Errorf("stderr = %q, want the timeout and the run id", got.stderr)
	}
	if got.stdout != "working\n" {
		t.Errorf("stdout = %q: what was printed before the timeout stays, once", got.stdout)
	}
}

func TestWatchARealTimeoutEndsTheWaitWithoutTheFakeClock(t *testing.T) {
	w := newWatchServer(t, []string{"running"}, nil)
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"watch", "--timeout", "20ms", "run-7"}, Env{
		Stdout: &stdout, Stderr: &stderr, Runner: &execxtest.Fake{},
		Build:  buildinfo.Info{Version: "v0.0.0-test"},
		Getenv: func(name string) string { return watchVars(w)[name] },
	})
	if code != ExitFailed || !strings.Contains(stderr.String(), "Gave up waiting after 20ms.") || !strings.Contains(stderr.String(), "run-7") {
		t.Errorf("status = %d, stderr = %q", code, stderr.String())
	}
}

func TestWatchInterruptingSaysTheRunContinues(t *testing.T) {
	w := newWatchServer(t, []string{"running"}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr bytes.Buffer
	code := Run(ctx, []string{"watch", "run-7"}, Env{
		Stdout: &stdout, Stderr: &stderr, Runner: &execxtest.Fake{},
		Getenv: func(name string) string { return watchVars(w)[name] },
		Sleep: func(ctx context.Context, d time.Duration) error {
			cancel()
			return ctx.Err()
		},
	})
	if code != ExitFailed || !strings.Contains(stderr.String(), "Interrupted.") || !strings.Contains(stderr.String(), "Run run-7 continues on the server.") {
		t.Errorf("status = %d, stderr = %q", code, stderr.String())
	}
}

func TestWatchControlCharactersInTheLogNeverReachStdout(t *testing.T) {
	// A log is text of the server's choosing: an escape sequence could rewrite
	// the terminal. Newlines and tabs are the layout and stay.
	w := newWatchServer(t, []string{"completed"}, []*string{str("ok\t1\n\x1b]0;owned\x07\x1b[2Jline\r\x00two\n")})
	var sleeps int
	got := command(t, watchVars(w), instantSleep(&sleeps), "watch", "run-7")
	if want := "ok\t1\n]0;owned[2Jlinetwo\n"; got.stdout != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
	if strings.ContainsAny(got.stdout, "\x1b\x07\x00\r") {
		t.Errorf("a control character reached stdout: %q", got.stdout)
	}
}

func TestWatchNeverPrintsTheKeyWhateverTheServerEchoes(t *testing.T) {
	w := newWatchServer(t, []string{"running", "completed"},
		[]*string{str("Authorization: Bearer " + verifyKey + "\n"), str("Authorization: Bearer " + verifyKey + "\nsplit " + verifyKey[:6] + "\x00" + verifyKey[6:] + "\n")})
	var sleeps int
	got := command(t, watchVars(w), instantSleep(&sleeps), "watch", "run-7") // command fails the test if the key shows
	if got.code != ExitOK || !strings.Contains(got.stdout, "Bearer [redacted]") {
		t.Errorf("got %+v", got)
	}

	failing := newWatchServer(t, []string{"running"}, nil)
	failing.runStatus = http.StatusForbidden
	command(t, watchVars(failing), instantSleep(&sleeps), "watch", "run-7")
}

func TestNewLogTextHandlesTheEdgesOfTheWindow(t *testing.T) {
	for _, c := range []struct{ name, prev, cur, want string }{
		{"first chunk", "", "a\n", "a\n"},
		{"same chunk", "a\n", "a\n", ""},
		{"grew", "a\n", "a\nb\n", "b\n"},
		{"slid, longest overlap wins", "x\ny\nx\n", "x\ny\nx\nz\n"[2:], "z\n"},
		{"overlap of one line", "a\nb\n", "b\nc\n", "c\n"},
		{"no overlap", "a\n", "b\n", "b\n"},
		{"empty answer", "a\n", "", ""},
		{"multibyte text", "é\nü\n", "ü\nñ\n", "ñ\n"},
	} {
		if got := newLogText(c.prev, c.cur); got != c.want {
			t.Errorf("%s: newLogText(%q, %q) = %q, want %q", c.name, c.prev, c.cur, got, c.want)
		}
	}
}
