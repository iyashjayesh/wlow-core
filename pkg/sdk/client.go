package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	gonats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/wlow/wlow/pkg/nats"
	"github.com/wlow/wlow/pkg/workflow"
)

type Client struct {
	nc *nats.Client
	js jetstream.JetStream
}

type ClientConfig struct {
	NATSUrl string
}

func NewClient(cfg ClientConfig) (*Client, error) {
	nc, err := nats.NewClient(nats.Config{URL: cfg.NATSUrl})
	if err != nil {
		return nil, err
	}
	return &Client{nc: nc, js: nc.JetStream()}, nil
}

func (c *Client) Close() { c.nc.Close() }

func (c *Client) Submit(ctx context.Context, wf *workflow.Workflow) error {
	data, _ := json.Marshal(wf)
	_, err := c.js.Publish(ctx, "workflow.submit", data)
	return err
}

func (c *Client) SubmitAndWait(ctx context.Context, wf *workflow.Workflow) (*workflow.WorkflowResult, error) {
	reply := fmt.Sprintf("workflow.reply.%s", wf.ID)
	wf.ReplySubject = reply

	sub, err := c.nc.SubscribeSync(reply)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := sub.Unsubscribe(); err != nil {
			slog.Default().Error("failed to unsubscribe", slog.String("error", err.Error()))
		}
	}()

	if err := c.Submit(ctx, wf); err != nil {
		return nil, err
	}

	msg, err := sub.NextMsgWithContext(ctx)
	if err != nil {
		return nil, err
	}

	var r workflow.WorkflowResult
	return &r, json.Unmarshal(msg.Data, &r)
}

func (c *Client) Cancel(ctx context.Context, wfID string) error {
	data, _ := json.Marshal(workflow.WorkflowCancel{WorkflowID: wfID})
	_, err := c.js.Publish(ctx, "workflow.cancel", data)
	return err
}

// WorkflowBuilder

type WorkflowBuilder struct {
	wf *workflow.Workflow
}

func NewWorkflow(id string) *WorkflowBuilder {
	return &WorkflowBuilder{wf: &workflow.Workflow{
		ID:           id,
		Tasks:        make(map[string]workflow.Task),
		Dependencies: make(map[string][]string),
		Metadata:     make(map[string]any),
	}}
}

func (b *WorkflowBuilder) Metadata(k string, v any) *WorkflowBuilder {
	b.wf.Metadata[k] = v
	return b
}

func (b *WorkflowBuilder) ReplyTo(subj string) *WorkflowBuilder {
	b.wf.ReplySubject = subj
	return b
}

func (b *WorkflowBuilder) Task(id, subject string, input map[string]any, deps ...string) *WorkflowBuilder {
	b.wf.Tasks[id] = workflow.Task{ID: id, Subject: subject, Input: input}
	if len(deps) > 0 {
		b.wf.Dependencies[id] = deps
	}
	return b
}

func (b *WorkflowBuilder) Build() (*workflow.Workflow, error) {
	data, _ := json.Marshal(b.wf)
	return workflow.ParseWorkflow(data)
}

func (b *WorkflowBuilder) MustBuild() *workflow.Workflow {
	wf, err := b.Build()
	if err != nil {
		panic(err)
	}
	return wf
}

// For testing/convenience
func (c *Client) Subscribe(subj string) (*gonats.Subscription, error) {
	return c.nc.Subscribe(subj, nil)
}
