package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// logFormatter turns the run log, one JSON event per line, into one short line of
// text per event. A line that is not a JSON object with a "type" is printed as it
// came: the log of an older server, or a line the formatter does not know.
//
// The text arrives in pieces that need not end at a line break, so the part
// after the last one waits for the next piece (or for flush).
type logFormatter struct {
	// clean is applied to every value taken out of the JSON: decoding turns an
	// escaped control character back into a real one.
	clean   func(string) string
	pending string
}

func (f *logFormatter) write(text string) string {
	text = f.pending + text
	end := strings.LastIndexByte(text, '\n')
	if end < 0 {
		f.pending = text
		return ""
	}
	f.pending = text[end+1:]
	var out strings.Builder
	for _, line := range strings.Split(text[:end], "\n") {
		out.WriteString(f.line(line))
		out.WriteByte('\n')
	}
	return out.String()
}

// flush returns what is left once no more text will come.
func (f *logFormatter) flush() string {
	if f.pending == "" {
		return ""
	}
	line := f.line(f.pending)
	f.pending = ""
	return line + "\n"
}

func (f *logFormatter) line(raw string) string {
	var event map[string]any
	if json.Unmarshal([]byte(raw), &event) != nil {
		return raw
	}
	kind, ok := event["type"].(string)
	if !ok || kind == "" {
		return raw
	}
	get := func(name string) string {
		value, ok := event[name]
		if !ok || value == nil {
			return ""
		}
		var s string
		switch v := value.(type) {
		case string:
			s = v
		case float64:
			s = strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
		case bool:
			s = fmt.Sprint(v)
		default:
			return ""
		}
		return f.clean(s)
	}
	used := map[string]bool{"type": true, "ts": true, "role": true}
	use := func(names ...string) {
		for _, name := range names {
			used[name] = true
		}
	}

	var what string
	switch kind {
	case "run.preparing":
		what = "preparing " + get("repo")
		use("repo")
	case "run.finalizing":
		what = "finalizing"
	case "worker.started":
		what = "started"
	case "worker.session_started":
		what = fmt.Sprintf("attempt %s of %s", get("attempt"), get("task"))
		use("attempt", "task", "session_id", "phase_kind")
	case "worker.task_started":
		what = "task " + get("task")
		use("task")
	case "worker.phase":
		what = strings.ReplaceAll(get("phase"), "_", " ")
		use("phase", "task")
	case "worker.tool":
		what = strings.TrimSpace(get("tool") + " " + get("target"))
		use("tool", "target")
	case "worker.provider_waiting":
		what = "waiting for the model, turn " + get("turn")
		use("turn")
	case "worker.error":
		what = "ERROR " + get("detail")
		use("detail")
	case "worker.done":
		what = "done"
		if get("success") == "false" {
			what = "done, without success"
		}
		use("success")
	case "worker.session_finished":
		what = fmt.Sprintf("attempt %s %s", get("attempt"), get("status"))
		if cost := get("cost_usd"); cost != "" {
			what += ", $" + money(event["cost_usd"])
		}
		if model := get("model"); model != "" {
			what += ", " + model
		}
		use("attempt", "status", "cost_usd", "model", "session_id", "task", "phase_kind")
	default:
		what = f.clean(kind)
	}

	// Whatever the event carries that the line above did not show, in a stable
	// order, so no field of an event the client does not know is lost.
	var rest []string
	for name := range event {
		if !used[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	for _, name := range rest {
		if value := get(name); value != "" {
			what += " " + f.clean(name) + "=" + value
		}
	}

	ts := get("ts")
	if ts == "" {
		ts = "--:--:--"
	}
	return strings.TrimRight(fmt.Sprintf("%-8s  %-11s  %s", ts, get("role"), what), " ")
}

func money(value any) string {
	if v, ok := value.(float64); ok {
		return fmt.Sprintf("%.2f", v)
	}
	return ""
}
