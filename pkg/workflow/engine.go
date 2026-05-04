package workflow

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
)

type Store interface {
	InitWorkflow(ctx context.Context, wf *Workflow) error
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)
	StoreTaskState(ctx context.Context, wfID, taskID string, state *TaskResult) error
	GetTaskState(ctx context.Context, wfID, taskID string) (*TaskResult, error)
	IsWorkflowCompleted(ctx context.Context, wfID string) (bool, error)
	CancelWorkflow(ctx context.Context, wfID string) error
	AggregateResult(ctx context.Context, wfID string) (*WorkflowResult, error)
	UpdateProgress(ctx context.Context, wfID string) error
}

type Publisher interface {
	Publish(ctx context.Context, subject string, data []byte) error
}

type PublisherFunc func(ctx context.Context, subject string, data []byte) error

func (f PublisherFunc) Publish(ctx context.Context, subject string, data []byte) error {
	return f(ctx, subject, data)
}

type Metrics interface {
	WorkflowStarted()
	WorkflowCompleted(id string, dur float64)
	WorkflowFailed(id string)
	TaskQueued(subject string)
	MessageError(typ string)
}

type noopMetrics struct{}

func (noopMetrics) WorkflowStarted()                  {}
func (noopMetrics) WorkflowCompleted(string, float64) {}
func (noopMetrics) WorkflowFailed(string)             {}
func (noopMetrics) TaskQueued(string)                 {}
func (noopMetrics) MessageError(string)               {}

type EngineConfig struct {
	Store                  Store
	Publisher              Publisher
	Resolver               Resolver
	Locality               *LocalityScheduler
	ProcessorSubjectPrefix string
	Logger                 *slog.Logger
	Metrics                Metrics
}

type Engine struct {
	store                  Store
	pub                    Publisher
	resolver               Resolver
	locality               *LocalityScheduler
	processorSubjectPrefix string
	log                    *slog.Logger
	metrics                Metrics
}

func NewEngine(cfg EngineConfig) *Engine {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Metrics == nil {
		cfg.Metrics = noopMetrics{}
	}
	return &Engine{
		store:                  cfg.Store,
		pub:                    cfg.Publisher,
		resolver:               cfg.Resolver,
		locality:               cfg.Locality,
		processorSubjectPrefix: cfg.ProcessorSubjectPrefix,
		log:                    cfg.Logger,
		metrics:                cfg.Metrics,
	}
}

func (e *Engine) HandleWorkflow(msg jetstream.Msg) {
	e.metrics.WorkflowStarted()

	wf, err := ParseWorkflow(msg.Data())
	if err != nil {
		e.metrics.MessageError("parse")
		e.log.Error("parse failed", "error", err)
		msg.Nak()
		return
	}

	log := e.log.With("workflow_id", wf.ID)

	if err := e.store.InitWorkflow(context.Background(), wf); err != nil {
		log.Error("init failed", "error", err)
		msg.Nak()
		return
	}

	log.Info("workflow started", "tasks", len(wf.Tasks))

	for _, t := range wf.RootTasks() {
		if err := e.publishTask(context.Background(), &t); err != nil {
			log.Error("publish failed", "task", t.ID, "error", err)
		}
	}

	msg.Ack()
}

func (e *Engine) publishTask(ctx context.Context, t *Task) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}

	subject, err := RouteTaskWithProcessorPrefix(ctx, e.resolver, e.locality, e.processorSubjectPrefix, t)
	if err != nil {
		return err
	}
	if err := e.pub.Publish(ctx, subject, data); err != nil {
		return err
	}

	e.metrics.TaskQueued(subject)

	return e.store.StoreTaskState(ctx, t.WorkflowID, t.ID, &TaskResult{
		WorkflowID: t.WorkflowID,
		TaskID:     t.ID,
		Status:     StatusQueued,
	})
}
