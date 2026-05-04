package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/wlow/wlow/pkg/artifact"
)

type Workflow struct {
	ID           string              `json:"id"`
	Tasks        map[string]Task     `json:"tasks"`
	Dependencies map[string][]string `json:"dependencies"`
	ReplySubject string              `json:"reply_subject"`
	Metadata     map[string]any      `json:"metadata,omitempty"`
	StartedAt    time.Time           `json:"started_at"`
}

type Task struct {
	WorkflowID       string                 `json:"workflow_id"`
	ID               string                 `json:"id"`
	Subject          string                 `json:"subject"`
	Tenant           string                 `json:"tenant,omitempty"`
	ProcessorID      string                 `json:"processor_id,omitempty"`
	ProcessorVersion string                 `json:"processor_version,omitempty"`
	ExecutionMode    artifact.ExecutionMode `json:"execution_mode,omitempty"`
	Input            map[string]any         `json:"input"`
}

type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusQueued    TaskStatus = "queued"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
	StatusCancelled TaskStatus = "cancelled"
	StatusTimedOut  TaskStatus = "timed_out"
)

type TaskResult struct {
	WorkflowID   string         `json:"workflow_id"`
	TaskID       string         `json:"task_id"`
	ProcessorID  string         `json:"processor_id"`
	Status       TaskStatus     `json:"status"`
	Output       map[string]any `json:"output,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
}

type WorkflowResult struct {
	WorkflowID  string         `json:"workflow_id"`
	Status      TaskStatus     `json:"status"`
	TaskResults []TaskResult   `json:"task_results"`
	Error       *WorkflowError `json:"error,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type WorkflowError struct {
	ProcessorID string `json:"processor_id,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	Message     string `json:"message"`
}

type WorkflowCancel struct {
	WorkflowID string `json:"workflow_id"`
}

func ParseWorkflow(data []byte) (*Workflow, error) {
	var wf Workflow
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, err
	}
	if wf.ID == "" {
		return nil, errors.New("workflow id required")
	}
	if len(wf.Tasks) == 0 {
		return nil, errors.New("workflow must have tasks")
	}
	if err := validateDAG(&wf); err != nil {
		return nil, err
	}
	return &wf, nil
}

func validateDAG(wf *Workflow) error {
	visited := make(map[string]bool)
	stack := make(map[string]bool)

	for id := range wf.Tasks {
		if !visited[id] && hasCycle(id, wf.Dependencies, visited, stack) {
			return errors.New("cycle detected")
		}
	}

	for id, deps := range wf.Dependencies {
		if _, ok := wf.Tasks[id]; !ok {
			return errors.New("unknown task: " + id)
		}
		for _, dep := range deps {
			if _, ok := wf.Tasks[dep]; !ok {
				return errors.New("unknown dependency: " + dep)
			}
		}
	}
	return nil
}

func hasCycle(id string, deps map[string][]string, visited, stack map[string]bool) bool {
	visited[id] = true
	stack[id] = true
	for _, dep := range deps[id] {
		if !visited[dep] {
			if hasCycle(dep, deps, visited, stack) {
				return true
			}
		} else if stack[dep] {
			return true
		}
	}
	stack[id] = false
	return false
}

func (t Task) RouteSubject() string {
	if t.ExecutionMode != artifact.ExecutionSandboxed {
		return t.Subject
	}
	if t.ProcessorID == "" {
		return t.Subject
	}
	return fmt.Sprintf("PROCESSOR.sandbox.%s", t.ProcessorID)
}

func (t Task) ProcessorRef() (string, string) {
	id := t.ProcessorID
	if id == "" {
		id = t.Subject
	}
	version := t.ProcessorVersion
	if version == "" {
		version = "latest"
	}
	return id, version
}

func (t Task) TenantID() string {
	if t.Tenant == "" {
		return artifact.DefaultTenant
	}
	return t.Tenant
}

func (w *Workflow) RootTasks() []Task {
	var roots []Task
	for id, t := range w.Tasks {
		if len(w.Dependencies[id]) == 0 {
			t.ID = id
			t.WorkflowID = w.ID
			roots = append(roots, t)
		}
	}
	return roots
}

func (w *Workflow) Dependents(taskID string) []string {
	var out []string
	for id, deps := range w.Dependencies {
		for _, d := range deps {
			if d == taskID {
				out = append(out, id)
				break
			}
		}
	}
	return out
}
