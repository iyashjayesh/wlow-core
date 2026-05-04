package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/wlow/wlow/pkg/artifact"
	"github.com/wlow/wlow/pkg/workflow"
)

const script = `import json
import subprocess
import sys

req = json.load(sys.stdin)
left = req["left"]
right = req["right"]
output = req.get("output", "/tmp/wlow-ffmpeg-output.jpg")

subprocess.run([
    "ffmpeg", "-y",
    "-i", left,
    "-i", right,
    "-filter_complex", "hstack=inputs=2",
    output,
], check=True)

print(json.dumps({"output": output}))
`

func main() {
	natsURL := flag.String("nats", "nats://localhost:4222", "NATS server URL")
	workflowID := flag.String("workflow", "wf-ffmpeg-process-1", "workflow id")
	left := flag.String("left", "/tmp/left.jpg", "left image path visible to the processor")
	right := flag.String("right", "/tmp/right.jpg", "right image path visible to the processor")
	output := flag.String("output", "/tmp/ffmpeg-joined.jpg", "output image path")
	repeat := flag.Int("repeat", 1, "number of workflow submissions for timing")
	workflowSubjectPrefix := flag.String("workflow-subject-prefix", envOr("WORKFLOW_SUBJECT_PREFIX", workflow.DefaultSubjectPrefix), "workflow control subject prefix")
	flag.Parse()
	if err := run(*natsURL, *workflowID, *workflowSubjectPrefix, *left, *right, *output, *repeat); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(natsURL, workflowID, workflowSubjectPrefix, left, right, output string, repeat int) error {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	defer nc.Close()
	if err := pushProcessor(nc); err != nil {
		return err
	}
	for idx := 0; idx < repeat; idx++ {
		id := workflowID
		if repeat > 1 {
			id = fmt.Sprintf("%s-%02d", workflowID, idx+1)
		}
		start := time.Now()
		if err := submitWorkflow(nc, id, workflowSubjectPrefix, left, right, output); err != nil {
			return err
		}
		fmt.Printf("workflow %s elapsed=%s\n", id, time.Since(start))
	}
	return nil
}

func pushProcessor(nc *nats.Conn) error {
	client, err := artifact.NewClient(nc, 10*time.Second)
	if err != nil {
		return fmt.Errorf("artifact client: %w", err)
	}
	manifest, err := client.Push(context.Background(), []byte(script), artifact.PushOptions{
		Tenant:      artifact.DefaultTenant,
		ProcessorID: "ffmpeg-process",
		Version:     "v1",
		Tags:        []string{"latest"},
		Manifest: artifact.Manifest{
			Runtime:    artifact.RuntimeProcess,
			IOProtocol: artifact.IOProtocolJSONStdio,
			RuntimeConfig: map[string]any{
				"cmd":  "python3",
				"args": []string{"{artifact}"},
			},
			ResourceHints: artifact.ResourceHints{Timeout: 30 * time.Second},
		},
	})
	if err != nil {
		return fmt.Errorf("push processor: %w", err)
	}
	fmt.Printf("pushed %s:%s\n", manifest.ProcessorID, manifest.Version)
	return nil
}

func submitWorkflow(nc *nats.Conn, workflowID, workflowSubjectPrefix, left, right, output string) error {
	reply := "workflow.reply." + workflowID
	sub, err := nc.SubscribeSync(reply)
	if err != nil {
		return fmt.Errorf("subscribe reply: %w", err)
	}
	defer sub.Unsubscribe()
	payload, err := json.Marshal(workflow.Workflow{
		ID:           workflowID,
		ReplySubject: reply,
		Tasks: map[string]workflow.Task{
			"join": {
				Subject:          "PROCESSOR.ffmpeg-process",
				ExecutionMode:    artifact.ExecutionSandboxed,
				Tenant:           artifact.DefaultTenant,
				ProcessorID:      "ffmpeg-process",
				ProcessorVersion: "latest",
				Input: map[string]any{
					"left":   left,
					"right":  right,
					"output": output,
				},
			},
		},
		Dependencies: map[string][]string{},
	})
	if err != nil {
		return fmt.Errorf("marshal workflow: %w", err)
	}
	if err := nc.Publish(workflow.SubmitSubject(workflowSubjectPrefix), payload); err != nil {
		return fmt.Errorf("publish workflow: %w", err)
	}
	msg, err := sub.NextMsg(45 * time.Second)
	if err != nil {
		return fmt.Errorf("wait workflow result: %w", err)
	}
	fmt.Println(string(msg.Data))
	return nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
