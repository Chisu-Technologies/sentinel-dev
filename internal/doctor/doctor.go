// Package doctor checks whether a machine is ready for Sentinel. Every check
// that does not pass says how to fix it.
package doctor

import (
	"encoding/json"
	"fmt"
	"io"
)

// Status is the outcome of one check.
type Status string

const (
	OK   Status = "ok"
	Warn Status = "warn"
	Fail Status = "fail"
)

// Check is one thing doctor looked at.
type Check struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
	// Fix says what to do about it. It is empty when the check passed.
	Fix string `json:"fix,omitempty"`
}

// Report is the result of checking one target.
type Report struct {
	// Target is "This machine" or an ssh destination.
	Target string  `json:"target"`
	Checks []Check `json:"checks"`
	// Facts carries what was measured on the target, when there is any.
	Facts any `json:"facts,omitempty"`
}

// Failed reports whether any check failed. A warning is not a failure.
func (r Report) Failed() bool {
	return r.count(Fail) > 0
}

func (r Report) count(s Status) int {
	n := 0
	for _, c := range r.Checks {
		if c.Status == s {
			n++
		}
	}
	return n
}

// WriteJSON writes the report as one JSON document.
func (r Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteText writes the report for a person to read.
func (r Report) WriteText(w io.Writer) {
	fmt.Fprintf(w, "%s\n", r.Target)
	width := 0
	for _, c := range r.Checks {
		width = max(width, len(c.Title))
	}
	labels := map[Status]string{OK: "ok", Warn: "warn", Fail: "FAIL"}
	for _, c := range r.Checks {
		fmt.Fprintf(w, "  %-4s  %-*s  %s\n", labels[c.Status], width, c.Title, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(w, "        fix: %s\n", c.Fix)
		}
	}
	fmt.Fprintln(w)
	switch failed, warned := r.count(Fail), r.count(Warn); {
	case failed > 0:
		fmt.Fprintf(w, "%s failed.\n", plural(failed, "check"))
	case warned > 0:
		fmt.Fprintf(w, "All checks passed, with %s.\n", plural(warned, "warning"))
	default:
		fmt.Fprintln(w, "All checks passed.")
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
