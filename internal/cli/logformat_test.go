package cli

import (
	"strings"
	"testing"
)

func plainFormatter() *logFormatter {
	return &logFormatter{clean: func(s string) string {
		return strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, s)
	}}
}

// Events as a real server wrote them in the log of a completed run.
const realEvents = `{"ts": "", "type": "run.preparing", "repo": "acme/widgets@main"}
{"ts": "08:17:43", "type": "worker.session_started", "role": "implementer", "session_id": "anthropic-session-1", "task": "STR-MVXK4FRK", "attempt": 1, "phase_kind": "task"}
{"ts": "08:17:43", "type": "worker.phase", "role": "implementer", "phase": "implementing_fix_for", "task": "STR-MVXK4FRK"}
{"ts": "08:19:42", "type": "worker.tool", "role": "implementer", "tool": "edit_file", "target": "docs/design/tokens.json"}
{"ts": "08:24:33", "type": "worker.tool", "role": "implementer", "tool": "workspace_exec"}
{"ts": "08:19:01", "type": "worker.provider_waiting", "role": "implementer", "turn": 6}
{"ts": "08:29:29", "type": "worker.error", "role": "qa", "detail": "inference gateway request failed (WorkspaceCoordinatorError)"}
{"ts": "08:35:11", "type": "worker.done", "role": "qa", "success": true}
{"ts": "08:35:21", "type": "worker.session_finished", "role": "qa", "session_id": "anthropic-session-1", "task": "STR-MVXK4FRK", "attempt": 4, "phase_kind": "task", "status": "completed", "cost_usd": 0.726715, "model": "claude-opus-5-5"}
{"ts": "", "type": "run.finalizing"}
`

func TestLogFormatterTurnsEachEventIntoOneReadableLine(t *testing.T) {
	got := plainFormatter().write(realEvents)
	want := strings.Join([]string{
		"--:--:--               preparing acme/widgets@main",
		"08:17:43  implementer  attempt 1 of STR-MVXK4FRK",
		"08:17:43  implementer  implementing fix for",
		"08:19:42  implementer  edit_file docs/design/tokens.json",
		"08:24:33  implementer  workspace_exec",
		"08:19:01  implementer  waiting for the model, turn 6",
		"08:29:29  qa           ERROR inference gateway request failed (WorkspaceCoordinatorError)",
		"08:35:11  qa           done",
		"08:35:21  qa           attempt 4 completed, $0.73, claude-opus-5-5",
		"--:--:--               finalizing",
	}, "\n") + "\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLogFormatterLeavesLinesThatAreNotEventsAsTheyCame(t *testing.T) {
	in := "plain text from an older server\n[1, 2]\n{\"no\": \"type\"}\n{broken\n"
	if got := plainFormatter().write(in); got != in {
		t.Errorf("got %q, want the input unchanged", got)
	}
}

func TestLogFormatterKeepsTheFieldsOfAnEventItDoesNotKnow(t *testing.T) {
	got := plainFormatter().write(`{"ts":"09:00:00","type":"worker.new_thing","role":"qa","zeta":"z","alpha":3,"nested":{"x":1}}` + "\n")
	if want := "09:00:00  qa           worker.new_thing alpha=3 zeta=z\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The server's window of the log can end in the middle of a line; that part
// waits for the next piece, and flush gives it when nothing more will come.
func TestLogFormatterWaitsForTheEndOfALine(t *testing.T) {
	f := plainFormatter()
	line := `{"ts":"08:00:00","type":"worker.started","role":"qa"}`
	if got := f.write(line[:20]); got != "" {
		t.Errorf("half a line printed %q", got)
	}
	if got := f.write(line[20:] + "\n" + `{"ts":"08:00:01"`); got != "08:00:00  qa           started\n" {
		t.Errorf("got %q", got)
	}
	if got := f.flush(); got != `{"ts":"08:00:01"`+"\n" {
		t.Errorf("flush = %q, want the unfinished line as it came", got)
	}
	if got := f.flush(); got != "" {
		t.Errorf("second flush = %q", got)
	}
}

// Decoding JSON turns an escaped control character into a real one; it must
// not reach the terminal.
func TestLogFormatterRemovesControlCharactersFromDecodedValues(t *testing.T) {
	got := plainFormatter().write(`{"ts":"08:00:00","type":"worker.tool","role":"qa","tool":"\u001b[2Jbrowser","target":"x\u0007"}` + "\n")
	if strings.ContainsAny(got, "\x1b\x07") {
		t.Errorf("a control character reached the output: %q", got)
	}
}

func TestWatchPrintsTheLogReadableAndRawOnRequest(t *testing.T) {
	event := `{"ts":"08:19:42","type":"worker.tool","role":"implementer","tool":"edit_file","target":"a.go"}` + "\n"
	w := newWatchServer(t, []string{"completed"}, []*string{str(event)})

	got := command(t, watchVars(w), nil, "watch", "run-7")
	if want := "08:19:42  implementer  edit_file a.go\n"; got.stdout != want || got.code != ExitOK {
		t.Errorf("readable: %+v, want stdout %q", got, want)
	}

	got = command(t, watchVars(w), nil, "watch", "--raw", "run-7")
	if got.stdout != event || got.code != ExitOK {
		t.Errorf("--raw: %+v, want the log as it came", got)
	}
}
