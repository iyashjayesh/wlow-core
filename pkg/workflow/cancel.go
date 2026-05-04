package workflow

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
)

type CancelHandlerConfig struct {
	Store                 Store
	Publisher             Publisher
	WorkflowSubjectPrefix string
	Logger                *slog.Logger
}

type CancelHandler struct {
	store                 Store
	pub                   Publisher
	workflowSubjectPrefix string
	log                   *slog.Logger
}

func NewCancelHandler(cfg CancelHandlerConfig) *CancelHandler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &CancelHandler{
		store:                 cfg.Store,
		pub:                   cfg.Publisher,
		workflowSubjectPrefix: cfg.WorkflowSubjectPrefix,
		log:                   cfg.Logger,
	}
}

func (h *CancelHandler) HandleCancel(msg jetstream.Msg) {
	var c WorkflowCancel
	if err := json.Unmarshal(msg.Data(), &c); err != nil {
		h.log.Error("unmarshal failed", "error", err)
		msg.Nak()
		return
	}

	log := h.log.With("workflow_id", c.WorkflowID)

	if err := h.store.CancelWorkflow(context.Background(), c.WorkflowID); err != nil {
		log.Error("cancel failed", "error", err)
		msg.Nak()
		return
	}

	h.pub.Publish(context.Background(), WorkflowCancelSubject(h.workflowSubjectPrefix, c.WorkflowID), nil)
	log.Info("cancelled")
	msg.Ack()
}
