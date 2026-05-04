package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const maxNameLen = 64

// newProcessor scaffolds a new wlow processor project.
func newProcessor(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	lang := fs.String("lang", "python", "language template: python, go")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("processor name required: wlow new <name>")
	}
	name := fs.Arg(0)
	if len(name) == 0 || len(name) > maxNameLen {
		return fmt.Errorf("processor name must be 1–%d characters", maxNameLen)
	}
	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("directory %q already exists", name)
	}
	if err := os.MkdirAll(name, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	var err error
	switch *lang {
	case "python":
		err = scaffoldPython(name)
	case "go":
		err = scaffoldGo(name)
	default:
		err = fmt.Errorf("unknown language %q — supported: python, go", *lang)
	}
	if err != nil {
		_ = os.RemoveAll(name)
		return err
	}
	fmt.Printf("created %s/\n", name)
	fmt.Printf("\n  next steps:\n")
	fmt.Printf("  1. edit %s/processor.py (or main.go)\n", name)
	fmt.Printf("  2. wlow push --id %s --runtime process --source binary \\\n", name)
	fmt.Printf("              --path %s/processor.py --entrypoint python3,{artifact}\n", name)
	fmt.Printf("  3. wlow start\n\n")
	return nil
}

func scaffoldPython(dir string) error {
	proc := `#!/usr/bin/env python3
"""
` + filepath.Base(dir) + ` — wlow process processor.

Reads JSON from stdin, writes JSON to stdout.

Test locally:
    echo '{"text": "hello world"}' | python3 processor.py

Register with wlow:
    wlow push --id ` + filepath.Base(dir) + ` --runtime process \
              --source binary --path processor.py \
              --entrypoint python3,{artifact} --tags latest
"""
import json
import sys


def process(input_data: dict) -> dict:
    # TODO: implement your processor logic here
    return {"result": input_data, "status": "ok"}


if __name__ == "__main__":
    req = json.load(sys.stdin)
    result = process(req)
    print(json.dumps(result))
`

	dockerfile := "# " + filepath.Base(dir) + ` processor container
#
# The processor script is stored in NATS via "wlow push" — this image only
# provides the runtime environment (python3 + dependencies) and the wlow binary.
#
# Build:   docker build -t ` + filepath.Base(dir) + `:latest .
# Run:     docker run -e NATS_URL=nats://your-nats:4222 ` + filepath.Base(dir) + `

FROM python:3.12-slim

# Install dependencies
# RUN pip install --no-cache-dir requests numpy

# Install wlow binary — replace with the correct version tag or use a local copy
ARG WLOW_VERSION=latest
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && curl -fL -o /usr/local/bin/wlow \
         "https://github.com/wlow/wlow/releases/download/${WLOW_VERSION}/wlow-linux-amd64" \
    && chmod +x /usr/local/bin/wlow

ENV NATS_URL=nats://nats:4222

ENTRYPOINT ["wlow", "start"]
`

	files := map[string]string{
		"processor.py": proc,
		"Dockerfile":   dockerfile,
	}
	for name, content := range files {
		if err := writeScaffoldFile(filepath.Join(dir, name), content); err != nil {
			return err
		}
	}
	return nil
}

func scaffoldGo(dir string) error {
	name := filepath.Base(dir)
	modFile := `module ` + name + `

go 1.23.0

require github.com/wlow/wlow v0.0.0
`

	mainFile := `// Analyzer: go vet ./... and golangci-lint run.
package main

import (
	"context"
	"log"

	"github.com/wlow/wlow/pkg/sdk"
)

// Input defines the task input schema.
type Input struct {
	Text string ` + "`json:\"text\"`" + `
}

// Output defines the task output schema.
type Output struct {
	Result string ` + "`json:\"result\"`" + `
	Status string ` + "`json:\"status\"`" + `
}

// ` + name + `Processor implements the processor logic.
type ` + name + `Processor struct{}

func (p *` + name + `Processor) Process(_ context.Context, in Input) (Output, error) {
	// TODO: implement your processor logic here
	return Output{Result: in.Text, Status: "ok"}, nil
}

func main() {
	runner, err := sdk.NewRunner(sdk.RunnerConfig{
		ProcessorID: "` + name + `",
		Subjects:    []string{"PROCESSOR.` + name + `.process"},
		Concurrency: 4,
	}, sdk.Wrap(&` + name + `Processor{}))
	if err != nil {
		log.Fatal(err)
	}
	runner.Run(context.Background())
}
`

	dockerfile := `# ` + name + ` processor — Go SDK processor
#
# This binary embeds the processor logic and the NATS consumer loop.
# No separate wlow start needed — just run this container.
#
# Build:   docker build -t ` + name + `:latest .
# Run:     docker run -e NATS_URL=nats://your-nats:4222 ` + name + `

FROM golang:1.23-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/processor .

FROM debian:bookworm-slim
COPY --from=builder /out/processor /usr/local/bin/processor
ENV NATS_URL=nats://nats:4222
ENTRYPOINT ["/usr/local/bin/processor"]
`

	files := map[string]string{
		"main.go":    mainFile,
		"go.mod":     modFile,
		"Dockerfile": dockerfile,
	}
	for fname, content := range files {
		if err := writeScaffoldFile(filepath.Join(dir, fname), content); err != nil {
			return err
		}
	}
	return nil
}

func writeScaffoldFile(path, content string) error {
	if path == "" {
		return errors.New("file path required")
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
