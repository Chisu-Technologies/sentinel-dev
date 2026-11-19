package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Chisu-Technologies/sentinel-dev/internal/checkout"
	"github.com/Chisu-Technologies/sentinel-dev/internal/server"
)

// The modes of a session. They say how far Sentinel goes on its own once the
// Architect has a plan.
const (
	// modePlan talks the plan through and runs nothing.
	modePlan = "plan"
	// modeEdit runs the plan one task at a time, asking before each.
	modeEdit = "edit"
	// modeAuto approves the plan and runs every task. It is never the default:
	// it is the only mode in which Sentinel works without asking.
	modeAuto = "auto"
)

const sessionUsage = `session [--mode plan|edit|auto] [--plan <id>] [--language <code>] ["<what to do>"]`

const sessionHelp = `Type a message to change the plan, or:
  /plan   talk the plan through; nothing runs (the default)
  /edit   run the plan one task at a time, asking before each
  /auto   approve the plan and run every task without asking
  /quit   leave; the plan stays on the server and in the panel`

// errQuit means the person asked to leave.
var errQuit = errors.New("quit")

// runSession is an interactive session with the Architect of a Sentinel
// server, in the terminal: it plans a change to the repository of this folder
// with the person, then runs it as far as the mode allows.
func runSession(ctx context.Context, args []string, env Env) int {
	fs := flag.NewFlagSet("session", flag.ContinueOnError)
	mode := fs.String("mode", modePlan, "plan (run nothing), edit (ask before each task) or auto (run every task)")
	planID := fs.String("plan", "", "continue a plan that already exists, by its id")
	language := fs.String("language", "en", "the language the Architect answers in, such as en or es")
	if code := parse(fs, args, env, sessionUsage); code >= 0 {
		return code
	}
	if *mode != modePlan && *mode != modeEdit && *mode != modeAuto {
		fmt.Fprintln(env.Stderr, "sentinel session: --mode is plan, edit or auto")
		return ExitUsage
	}
	if fs.NArg() > 1 || (*planID != "" && fs.NArg() > 0) {
		fmt.Fprintf(env.Stderr, "usage: sentinel %s\n", sessionUsage)
		return ExitUsage
	}
	if *planID != "" && !server.ValidRunID(*planID) {
		fmt.Fprintln(env.Stderr, "sentinel session: --plan needs the id of a plan")
		return ExitUsage
	}
	if !isTerminal(env) {
		fmt.Fprintln(env.Stderr, `sentinel session needs a terminal to ask you things; to send a task from a script, use sentinel run "<task>"`)
		return ExitUsage
	}
	client, code := newServerClient(env, "session")
	if code >= 0 {
		return code
	}
	s := &session{env: env, client: client, mode: *mode, language: *language, in: newLineReader(env.Stdin)}

	var plan server.Plan
	var err error
	if *planID != "" {
		plan, err = client.GetPlan(ctx, *planID)
	} else {
		repo, repoErr := checkout.Repository(ctx, env.Runner, env.Dir)
		if repoErr != nil {
			fmt.Fprintf(env.Stderr, "sentinel session: %v\n", repoErr)
			return ExitFailed
		}
		intent := strings.TrimSpace(fs.Arg(0))
		if intent == "" {
			fmt.Fprintf(env.Stdout, "Sentinel session on %s, mode %s. What should Sentinel do?\n", repo, s.mode)
			if intent, err = s.ask(ctx, "> "); err != nil {
				return s.leave(err, plan)
			}
		}
		plan, err = client.CreatePlan(ctx, intent, repo, s.language)
	}
	if err != nil {
		fmt.Fprintf(env.Stderr, "sentinel session: %v\n", err)
		if apiErr := (*server.APIError)(nil); errors.As(err, &apiErr) && apiErr.Status == 401 {
			fmt.Fprintln(env.Stderr, "A session needs the sign-in of sentinel login: an API key made in the panel cannot talk to the Architect.")
		}
		return ExitFailed
	}
	fmt.Fprintf(env.Stdout, "Plan %s, mode %s. %s\n", plan.ID, s.mode, "Type /help for the commands.")
	return s.leave(s.loop(ctx, plan), s.last)
}

type session struct {
	env      Env
	client   *server.Client
	mode     string
	language string
	in       *lineReader
	// seen is how many messages of the conversation were already shown.
	seen int
	// tasks is the state of each task as last shown, so only changes print.
	tasks map[string]string
	// last is the plan as last read, for the message when the session ends.
	last server.Plan
	// auto is the plan this session already asked to run every task of.
	auto string
}

// loop follows the plan and asks the person at each point where Sentinel
// needs them, until the plan ends or the person leaves.
func (s *session) loop(ctx context.Context, plan server.Plan) error {
	sleep := s.env.Sleep
	if sleep == nil {
		sleep = sleepContext
	}
	for {
		s.show(plan)
		// acted is true when the plan or the mode changed here, so the loop
		// looks at it again at once instead of waiting for the server.
		acted := false
		var err error
		switch {
		case server.PlanEnded(plan.State):
			return nil
		case plan.State == server.PlanBlocked:
			reason := "it needs a decision"
			if plan.Blocker != nil && plan.Blocker.Message != "" {
				reason = s.client.Clean(plan.Blocker.Message, server.MaxDetail)
			}
			return fmt.Errorf("the plan is blocked: %s; continue it in the panel", reason)
		case plan.State == server.PlanAwaitingClarification && plan.PendingClarification != nil && plan.PendingClarification.CanAnswer:
			err = s.clarify(ctx, *plan.PendingClarification)
		case plan.State == server.PlanReadyForApproval && plan.Approvable():
			plan, acted, err = s.decide(ctx, plan)
		case plan.State == server.PlanApproved || (plan.State == server.PlanExecuting && !s.anyRunning(plan)):
			plan, acted, err = s.run(ctx, plan)
		}
		if err != nil {
			return err
		}
		if acted {
			continue
		}
		if err := sleep(ctx, pollInterval); err != nil {
			return err
		}
		if plan, err = s.client.GetPlan(ctx, plan.ID); err != nil {
			return err
		}
	}
}

// decide handles a plan ready to approve, as the mode says: plan talks it
// through, edit approves it once the person says yes to its first task, and
// auto approves it. It reports whether it acted.
func (s *session) decide(ctx context.Context, plan server.Plan) (server.Plan, bool, error) {
	switch s.mode {
	case modeAuto:
		fmt.Fprintln(s.env.Stdout, "Mode auto: approving the plan.")
		next, err := s.client.Approve(ctx, plan)
		return next, err == nil, err
	case modeEdit:
		task, ok := nextPending(plan)
		if !ok {
			return plan, false, errors.New("the plan has no task to run")
		}
		yes, err := s.confirm(ctx, task)
		if err != nil || !yes {
			return plan, !yes && err == nil, err
		}
		next, err := s.client.Approve(ctx, plan)
		if err != nil {
			return plan, false, err
		}
		next, err = s.client.Execute(ctx, next, server.ExecuteManual, []string{task.ID})
		return next, err == nil, err
	}
	for {
		line, err := s.ask(ctx, "plan> ")
		if err != nil {
			return plan, false, err
		}
		switch {
		case line == "":
			continue
		case s.command(line):
			if s.mode != modePlan {
				return plan, true, nil // looked at again, in the new mode
			}
			continue
		}
		next, err := s.client.SendMessage(ctx, plan, line, s.language)
		return next, false, err
	}
}

// run starts the tasks of an approved plan as the mode says: in auto all that
// are left; in edit the next one, if the person says yes. It reports whether
// it acted.
func (s *session) run(ctx context.Context, plan server.Plan) (server.Plan, bool, error) {
	switch s.mode {
	case modeAuto:
		if s.auto == plan.ID {
			return plan, false, nil // already running every task
		}
		fmt.Fprintln(s.env.Stdout, "Running every task that is left.")
		next, err := s.client.Execute(ctx, plan, server.ExecuteAuto, nil)
		if err == nil {
			s.auto = plan.ID
		}
		return next, err == nil, err
	case modeEdit:
		task, ok := nextPending(plan)
		if !ok {
			return plan, false, nil
		}
		yes, err := s.confirm(ctx, task)
		if err != nil || !yes {
			return plan, !yes && err == nil, err
		}
		next, err := s.client.Execute(ctx, plan, server.ExecuteManual, []string{task.ID})
		return next, err == nil, err
	}
	for {
		line, err := s.ask(ctx, "The plan is approved. /edit or /auto to run it, /quit to leave: ")
		if err != nil {
			return plan, false, err
		}
		if s.command(line) && s.mode != modePlan {
			return plan, true, nil
		}
	}
}

// confirm asks whether to run task. A /command that changes the mode answers
// no, so the plan is looked at again in the new mode; n ends the session.
func (s *session) confirm(ctx context.Context, task server.Task) (bool, error) {
	for {
		line, err := s.ask(ctx, fmt.Sprintf("Run %s «%s»? [y/n] ", task.ID, s.client.Clean(task.Title, 120)))
		if err != nil {
			return false, err
		}
		switch strings.ToLower(line) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, errQuit
		}
		if s.command(line) && s.mode != modeEdit {
			return false, nil
		}
	}
}

// command applies a /command and reports whether line was one. /quit is
// handled by ask.
func (s *session) command(line string) bool {
	switch line {
	case "/plan", "/edit", "/auto":
		s.mode = strings.TrimPrefix(line, "/")
		fmt.Fprintf(s.env.Stdout, "Mode %s.\n", s.mode)
		return true
	case "/help":
		fmt.Fprintln(s.env.Stdout, sessionHelp)
		return true
	}
	if strings.HasPrefix(line, "/") {
		fmt.Fprintf(s.env.Stdout, "Unknown command %s.\n%s\n", s.client.Clean(line, 40), sessionHelp)
		return true
	}
	return false
}

// clarify asks the questions of the Architect one by one and sends the answers.
func (s *session) clarify(ctx context.Context, c server.Clarification) error {
	answers := make([]server.Answer, 0, len(c.Questions))
	for _, q := range c.Questions {
		fmt.Fprintf(s.env.Stdout, "\n%s\n%s\n", s.client.Clean(q.Header, 64), s.client.Clean(q.Question, 1000))
		for i, choice := range q.Choices {
			mark := ""
			if choice.ID == q.RecommendedChoiceID {
				mark = " (recommended)"
			}
			fmt.Fprintf(s.env.Stdout, "  %d. %s%s: %s\n", i+1, s.client.Clean(choice.Label, 120), mark, s.client.Clean(choice.Tradeoff, 300))
		}
		for {
			line, err := s.ask(ctx, questionPrompt(q))
			if err != nil {
				return err
			}
			if value, ok := answerFor(q, line); ok {
				answers = append(answers, server.Answer{QuestionID: q.ID, Value: value})
				break
			}
			fmt.Fprintln(s.env.Stdout, "That is not an answer to this question.")
		}
	}
	return s.client.AnswerClarification(ctx, c, answers)
}

func questionPrompt(q server.Question) string {
	switch q.AnswerType {
	case "single_select":
		return "Number, or Enter for the recommended one: "
	case "multiple_select":
		return "Numbers separated by commas: "
	}
	return "> "
}

// answerFor reads one answer: free text as typed; one choice by its number, or
// the recommended one for an empty line; several choices by their numbers.
func answerFor(q server.Question, line string) (any, bool) {
	pick := func(field string) (string, bool) {
		n, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || n < 1 || n > len(q.Choices) {
			return "", false
		}
		return q.Choices[n-1].ID, true
	}
	switch q.AnswerType {
	case "single_select":
		if line == "" && q.RecommendedChoiceID != "" {
			return q.RecommendedChoiceID, true
		}
		return pick(line)
	case "multiple_select":
		var ids []string
		seen := map[string]bool{}
		for _, field := range strings.Split(line, ",") {
			id, ok := pick(field)
			if !ok {
				return nil, false
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		return ids, len(ids) > 0
	}
	return line, line != ""
}

// show prints what changed since the last time: new words of the Architect,
// the tasks once a plan has them, and each task whose state moved.
func (s *session) show(plan server.Plan) {
	s.last = plan
	if s.seen > len(plan.Messages) {
		s.seen = 0
	}
	for _, m := range plan.Messages[s.seen:] {
		if m.Role != "user" {
			fmt.Fprintf(s.env.Stdout, "\nArchitect: %s\n", s.client.CleanText(m.Content))
		}
	}
	s.seen = len(plan.Messages)
	tasks := plan.Tasks()
	if len(tasks) == 0 {
		return
	}
	if s.tasks == nil {
		s.tasks = map[string]string{}
		fmt.Fprintln(s.env.Stdout, "\nTasks:")
		for i, t := range tasks {
			fmt.Fprintf(s.env.Stdout, "  %d. %s  %s\n", i+1, t.ID, s.client.Clean(t.Title, 120))
		}
	}
	for _, t := range tasks {
		before, known := s.tasks[t.ID]
		s.tasks[t.ID] = t.State
		if !known && t.State == server.TaskPending || before == t.State {
			continue
		}
		line := fmt.Sprintf("%s: %s", t.ID, s.client.Clean(t.State, 32))
		if strings.HasPrefix(t.PRURL, "https://github.com/") {
			line += "  " + s.client.Clean(t.PRURL, server.MaxDetail)
		}
		if t.Error != "" && t.State != server.TaskDone {
			line += "  (" + s.client.Clean(t.Error, server.MaxDetail) + ")"
		}
		fmt.Fprintln(s.env.Stdout, line)
	}
}

// anyRunning reports whether a task of the plan is still in Sentinel's hands.
func (s *session) anyRunning(plan server.Plan) bool {
	for _, t := range plan.Tasks() {
		if t.State != server.TaskPending && !server.TaskSettled(t.State) {
			return true
		}
	}
	return false
}

func nextPending(plan server.Plan) (server.Task, bool) {
	for _, t := range plan.Tasks() {
		if t.State == server.TaskPending {
			return t, true
		}
	}
	return server.Task{}, false
}

// ask prints prompt and reads one line. /quit and the end of the input both
// end the session.
func (s *session) ask(ctx context.Context, prompt string) (string, error) {
	fmt.Fprint(s.env.Stdout, prompt)
	line, err := s.in.read(ctx)
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "/quit" {
		return "", errQuit
	}
	return line, nil
}

// leave ends the session and says where the plan is.
func (s *session) leave(err error, plan server.Plan) int {
	where := ""
	if plan.ID != "" {
		where = fmt.Sprintf(" Plan %s stays on the server; sentinel session --plan %s continues it.", plan.ID, plan.ID)
	}
	switch {
	case err == nil:
		state := s.client.Clean(plan.State, 32)
		fmt.Fprintf(s.env.Stdout, "\nThe plan is %s.\n", state)
		if plan.State == server.PlanCompleted {
			return ExitOK
		}
		return ExitFailed
	case errors.Is(err, errQuit) || errors.Is(err, io.EOF):
		fmt.Fprintf(s.env.Stdout, "\nLeft the session.%s\n", where)
		return ExitOK
	case errors.Is(err, context.Canceled):
		fmt.Fprintf(s.env.Stderr, "\nInterrupted.%s\n", where)
		return ExitFailed
	}
	fmt.Fprintf(s.env.Stderr, "sentinel session: %s.%s\n", s.client.Clean(err.Error(), server.MaxDetail), where)
	return ExitFailed
}

// isTerminal reports whether the input is a person at a terminal.
func isTerminal(env Env) bool {
	if env.Terminal != nil {
		return env.Terminal()
	}
	f, ok := env.Stdin.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// lineReader reads lines in the background, so waiting for a line can stop
// when the context ends: Ctrl-C must end a session that waits for an answer.
type lineReader struct {
	lines chan string
	err   error
}

func newLineReader(r io.Reader) *lineReader {
	l := &lineReader{lines: make(chan string)}
	if r == nil {
		r = strings.NewReader("")
	}
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			l.lines <- scanner.Text()
		}
		l.err = scanner.Err()
		close(l.lines)
	}()
	return l
}

func (l *lineReader) read(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case line, ok := <-l.lines:
		if !ok {
			if l.err != nil {
				return "", l.err
			}
			return "", io.EOF
		}
		return line, nil
	}
}
