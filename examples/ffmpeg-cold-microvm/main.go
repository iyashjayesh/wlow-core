// Analyzer: go vet ./... and golangci-lint run.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/wlow/wlow/pkg/artifact"
	"github.com/wlow/wlow/pkg/workflow"
)

func main() {
	natsURL := flag.String("nats", "nats://localhost:4222", "NATS URL")
	workflowID := flag.String("workflow", "wf-cold-microvm-1", "workflow id")
	left := flag.String("left", "/app/left.png", "left image path inside the microVM rootfs")
	right := flag.String("right", "/app/right.png", "right image path inside the microVM rootfs")
	output0 := flag.String("output0", "/tmp/wlow-hstack.png", "p0 output path inside the microVM")
	output1 := flag.String("output1", "/tmp/wlow-vstack.png", "p1 output path inside the microVM")
	processorPrefix := flag.String("processor-prefix", "ffmpeg-snapshot", "processor id prefix; uses <prefix>-p0 and <prefix>-p1")
	repeat := flag.Int("repeat", 1, "number of workflow submissions for timing")
	workflowSubjectPrefix := flag.String("workflow-subject-prefix", envOr("WORKFLOW_SUBJECT_PREFIX", workflow.DefaultSubjectPrefix), "workflow control subject prefix")
	flag.Parse()
	if err := run(*natsURL, *workflowID, *workflowSubjectPrefix, *processorPrefix, *left, *right, *output0, *output1, *repeat); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(natsURL, workflowID, workflowSubjectPrefix, processorPrefix, left, right, output0, output1 string, repeat int) error {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	defer nc.Close()
	for idx := 0; idx < repeat; idx++ {
		id := workflowID
		if repeat > 1 {
			id = fmt.Sprintf("%s-%02d", workflowID, idx+1)
		}
		start := time.Now()
		if err := submitWorkflow(nc, id, workflowSubjectPrefix, processorPrefix, left, right, output0, output1); err != nil {
			return err
		}
		fmt.Printf("workflow %s elapsed=%s\n", id, time.Since(start))
	}
	return nil
}

func submitWorkflow(nc *nats.Conn, workflowID, workflowSubjectPrefix, processorPrefix, left, right, output0, output1 string) error {
	reply := "workflow.reply." + workflowID
	sub, err := nc.SubscribeSync(reply)
	if err != nil {
		return fmt.Errorf("subscribe reply: %w", err)
	}
	defer func() {
		if err := sub.Unsubscribe(); err != nil {
			log.Printf("unsubscribe reply: %v", err)
		}
	}()

	payload, err := workflowPayload(workflowID, reply, processorPrefix, left, right, output0, output1)
	if err != nil {
		return fmt.Errorf("marshal workflow: %w", err)
	}
	if err := nc.Publish(workflow.SubmitSubject(workflowSubjectPrefix), payload); err != nil {
		return fmt.Errorf("publish workflow: %w", err)
	}
	fmt.Printf("submitted workflow %s, waiting for %s\n", workflowID, reply)

	msg, err := sub.NextMsg(5 * time.Minute)
	if err != nil {
		return fmt.Errorf("wait workflow result: %w", err)
	}
	fmt.Println(string(msg.Data))
	var result workflow.WorkflowResult
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return fmt.Errorf("decode workflow result: %w", err)
	}
	if result.Status != workflow.StatusCompleted {
		return fmt.Errorf("workflow finished with status=%s error=%v", result.Status, result.Error)
	}
	return nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func workflowPayload(workflowID, reply, processorPrefix, left, right, output0, output1 string) ([]byte, error) {
	return json.Marshal(workflow.Workflow{
		ID:           workflowID,
		ReplySubject: reply,
		Tasks: map[string]workflow.Task{
			"p0": task(processorPrefix+"-p0", left, right, output0),
			"p1": task(processorPrefix+"-p1", left, right, output1),
		},
		Dependencies: map[string][]string{},
	})
}

func task(processorID, left, right, output string) workflow.Task {
	return workflow.Task{
		Subject:          "PROCESSOR." + processorID,
		ExecutionMode:    artifact.ExecutionSandboxed,
		Tenant:           artifact.DefaultTenant,
		ProcessorID:      processorID,
		ProcessorVersion: "latest",
		Input: map[string]any{
			"left":   left,
			"right":  right,
			"output": output,
		},
	}
}
