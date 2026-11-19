package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
)

// Plan states of the Architect, as the server names them.
const (
	PlanReadyForApproval      = "ready_for_approval"
	PlanAwaitingClarification = "awaiting_clarification"
	PlanApproved              = "approved"
	PlanDispatching           = "dispatching"
	PlanExecuting             = "executing"
	PlanBlocked               = "blocked"
	PlanCompleted             = "completed"
	PlanFailed                = "failed"
	PlanCancelled             = "cancelled"
)

// PlanEnded reports whether a plan in this state will not change again.
func PlanEnded(state string) bool {
	return state == PlanCompleted || state == PlanFailed || state == PlanCancelled
}

// Execution modes of an approved plan: the tasks named, or everything left.
const (
	ExecuteManual = "manual"
	ExecuteAuto   = "auto"
)

// Task states of a plan's task. Any other state means it is still going.
const (
	TaskPending      = "pending"
	TaskDone         = "done"
	TaskFailed       = "failed"
	TaskBlocked      = "blocked"
	TaskSkipped      = "skipped"
	TaskWaitingHuman = "waiting_human"
)

// TaskSettled reports whether a task needs nothing more from Sentinel: it
// ended, or it waits for a person.
func TaskSettled(state string) bool {
	switch state {
	case TaskDone, TaskFailed, TaskBlocked, TaskSkipped, TaskWaitingHuman:
		return true
	}
	return false
}

// A task id is sent back in a body only; it still has to look like one, so a
// hostile answer cannot smuggle anything in.
var taskID = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

// Message is one line of the conversation with the Architect.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Choice is one answer a question offers.
type Choice struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Tradeoff string `json:"tradeoff"`
}

// Question is one thing the Architect asks before it plans.
type Question struct {
	ID                  string   `json:"id"`
	Header              string   `json:"header"`
	Question            string   `json:"question"`
	AnswerType          string   `json:"answer_type"`
	Choices             []Choice `json:"choices"`
	RecommendedChoiceID string   `json:"recommended_choice_id"`
}

// Clarification is a set of questions waiting for answers.
type Clarification struct {
	ID                string     `json:"id"`
	Status            string     `json:"status"`
	Questions         []Question `json:"questions"`
	ExpectedRevision  int        `json:"expected_revision"`
	CanAnswer         bool       `json:"can_answer"`
	ContinuationToken *string    `json:"continuation_token"`
}

// Answer is the answer to one question: a string, or a list of choice ids.
type Answer struct {
	QuestionID string `json:"question_id"`
	Value      any    `json:"value"`
}

// Task is one task of a plan, with what Sentinel did with it so far.
type Task struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	State string `json:"_state"`
	PRURL string `json:"_pr_url"`
	Error string `json:"_error"`
}

// Blocker says why a plan stopped and waits.
type Blocker struct {
	Message string `json:"message"`
}

// Plan is the part of an Architect plan the client reads.
type Plan struct {
	ID                   string            `json:"id"`
	Repo                 string            `json:"repo"`
	State                string            `json:"state"`
	StateRevision        int               `json:"state_revision"`
	Revision             int               `json:"revision"`
	FinalizedRevision    *int              `json:"finalized_revision"`
	TaskSetSHA256        *string           `json:"task_set_sha256"`
	Messages             []Message         `json:"messages"`
	PendingClarification *Clarification    `json:"pending_clarification"`
	Blocker              *Blocker          `json:"blocker"`
	Failure              map[string]string `json:"failure"`
	Body                 *struct {
		Stages []struct {
			Sprints []struct {
				Tasks []Task `json:"tasks"`
			} `json:"sprints"`
		} `json:"stages"`
	} `json:"plan"`
}

// Tasks are the plan's tasks in their order, without ones whose id the client
// would not send back.
func (p Plan) Tasks() []Task {
	var tasks []Task
	if p.Body == nil {
		return nil
	}
	for _, stage := range p.Body.Stages {
		for _, sprint := range stage.Sprints {
			for _, task := range sprint.Tasks {
				if taskID.MatchString(task.ID) {
					if task.State == "" {
						task.State = TaskPending
					}
					tasks = append(tasks, task)
				}
			}
		}
	}
	return tasks
}

// Approvable reports whether the plan can be approved as it is: the server
// binds the approval to the exact revision and task set the person saw.
func (p Plan) Approvable() bool {
	return p.State == PlanReadyForApproval && p.FinalizedRevision != nil &&
		*p.FinalizedRevision == p.Revision && p.TaskSetSHA256 != nil && sha256Hex.MatchString(*p.TaskSetSHA256)
}

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ErrNotApprovable means the plan changed or is not ready to approve.
var ErrNotApprovable = errors.New("the plan is not ready to approve")

const architectPath = "/api/org/architect"

// CreatePlan starts a conversation with the Architect about intent, in repo.
func (c *Client) CreatePlan(ctx context.Context, intent, repo, language string) (Plan, error) {
	body := map[string]any{"intent": intent, "repo": repo, "language": language}
	return c.plan(ctx, http.MethodPost, architectPath+"/plans", body)
}

// GetPlan reads a plan as it is now.
func (c *Client) GetPlan(ctx context.Context, id string) (Plan, error) {
	if !ValidRunID(id) {
		return Plan{}, errors.New("not a valid plan id")
	}
	return c.plan(ctx, http.MethodGet, architectPath+"/plans/"+id, nil)
}

// SendMessage tells the Architect something more about the plan.
func (c *Client) SendMessage(ctx context.Context, p Plan, content, language string) (Plan, error) {
	if !ValidRunID(p.ID) {
		return Plan{}, errors.New("not a valid plan id")
	}
	body := map[string]any{"content": content, "expected_state_revision": p.StateRevision, "language": language}
	return c.plan(ctx, http.MethodPost, architectPath+"/plans/"+p.ID+"/message", body)
}

// AnswerClarification answers the questions of a clarification. The server
// takes the answer and the plan moves on; the caller reads the plan again.
func (c *Client) AnswerClarification(ctx context.Context, clarification Clarification, answers []Answer) error {
	if !ValidRunID(clarification.ID) {
		return errors.New("not a valid clarification id")
	}
	token := ""
	if clarification.ContinuationToken != nil {
		token = *clarification.ContinuationToken
	}
	body := map[string]any{"expected_revision": clarification.ExpectedRevision, "continuation_token": token, "answers": answers}
	status, data, tooLarge, err := c.fetch(ctx, http.MethodPost, architectPath+"/clarifications/"+clarification.ID+"/answer", body, "application/json")
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusAccepted {
		return c.apiError(status, data, tooLarge)
	}
	return nil
}

// Approve approves the plan exactly as p shows it. Nothing runs yet.
func (c *Client) Approve(ctx context.Context, p Plan) (Plan, error) {
	if !p.Approvable() || !ValidRunID(p.ID) {
		return Plan{}, ErrNotApprovable
	}
	body := map[string]any{
		"expected_revision":        *p.FinalizedRevision,
		"expected_state_revision":  p.StateRevision,
		"expected_task_set_sha256": *p.TaskSetSHA256,
	}
	return c.plan(ctx, http.MethodPost, architectPath+"/plans/"+p.ID+"/approve", body)
}

// Execute starts the approved plan: the tasks named with ExecuteManual, every
// task left with ExecuteAuto.
func (c *Client) Execute(ctx context.Context, p Plan, mode string, taskIDs []string) (Plan, error) {
	if !ValidRunID(p.ID) {
		return Plan{}, errors.New("not a valid plan id")
	}
	if mode != ExecuteManual && mode != ExecuteAuto {
		return Plan{}, errors.New("unknown execution mode")
	}
	for _, id := range taskIDs {
		if !taskID.MatchString(id) {
			return Plan{}, errors.New("not a valid task id")
		}
	}
	if taskIDs == nil {
		taskIDs = []string{}
	}
	body := map[string]any{"mode": mode, "task_ids": taskIDs, "expected_state_revision": p.StateRevision}
	return c.plan(ctx, http.MethodPost, architectPath+"/plans/"+p.ID+"/execute", body)
}

func (c *Client) plan(ctx context.Context, method, path string, body any) (Plan, error) {
	status, data, tooLarge, err := c.fetch(ctx, method, path, body, "application/json")
	if err != nil {
		return Plan{}, err
	}
	// 201 for a new plan; 202 when the server hands the change to the machine
	// that runs the plans and answers before it is applied.
	if status != http.StatusOK && status != http.StatusCreated && status != http.StatusAccepted {
		return Plan{}, c.apiError(status, data, tooLarge)
	}
	var p Plan
	if err := decode(data, tooLarge, &p); err != nil {
		return Plan{}, err
	}
	if !ValidRunID(p.ID) {
		return Plan{}, errors.New("the server returned a plan id that is not valid")
	}
	return p, nil
}
