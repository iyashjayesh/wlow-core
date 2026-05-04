# Setup

## What you need

| Component | Requirement |
|-----------|------------|
| NATS JetStream | 2.10+ with `--js` flag |
| Go | 1.23+ (to build wlow binaries) |
| `/dev/kvm` | Only for microVM runtimes — see [runner-setup.md](runner-setup.md) |

Process and WASM processors run on any machine with the `wlow` binary and a NATS connection. No Docker, no KVM, no registry required.

---

## 1. Start NATS

```sh
nats-server --js
# or
docker run -p 4222:4222 nats:latest --js
```

---

## 2. Build and run the wlow server

The server (`orchestrator`) is the control plane. It tracks workflow state in NATS. It does not execute any processor code.

```sh
make wlow
./bin/wlow start --control-plane
```

The server creates all required NATS streams and KV buckets on startup. Restart it anytime — all state lives in NATS.

Defaults (override via env):

```
NATS_URL                  nats://localhost:4222
STORE_BUCKET              workflow-state
WORKFLOW_SUBJECT_PREFIX   workflow
PROCESSOR_SUBJECT_PREFIX  wlow.processor
PROCESSOR_STREAM          WLOW_PROCESSOR
```

---

## 3. Write a processor

### Scaffold

```sh
make wlow
./bin/wlow new my-proc          # Python (default)
./bin/wlow new my-proc --lang go
```

This creates `my-proc/processor.py` (or `main.go`) and a `Dockerfile` with everything wired up.

### Or write it manually

A process processor is any script or binary that reads JSON from stdin and writes JSON to stdout:

```python
# processor.py
import json, sys
req = json.load(sys.stdin)
print(json.dumps({"result": req, "status": "ok"}))
```

A Go SDK processor uses `pkg/sdk` directly:

```go
type MyProcessor struct{}

func (p *MyProcessor) Process(ctx context.Context, in map[string]any) (map[string]any, error) {
    return map[string]any{"result": in}, nil
}

func main() {
    runner, _ := sdk.NewRunner(sdk.RunnerConfig{
        ProcessorID: "my-proc",
        Subjects:    []string{"PROCESSOR.my-proc.process"},
    }, sdk.Wrap(&MyProcessor{}))
    runner.Run(context.Background())
}
```

Go SDK processors are their own binary — they don't use `wlow start`. Just build and run your binary.

---

## 4. Register and start a processor

### Do you need `wlow push`?

| Processor type | Need push? |
|---------------|-----------|
| Go SDK binary (`sdk.NewRunner`) | **No** — run your binary directly |
| Python/Node/script | **No** — use `wlow start --cmd` |
| WASM component | **Yes** — `.wasm` binary must be stored in NATS |
| MicroVM (Dockerfile) | **Yes** — rootfs image must be in OCI registry |

### Go SDK processor (no push, no wlow start)

Your binary uses `pkg/sdk` directly and is its own runner. Just build and run it:

```sh
go build -o bin/my-proc ./cmd/my-proc
./bin/my-proc   # connects to NATS via NATS_URL and starts serving
```

### Script processor — `wlow start --cmd` (no push)

`--cmd` auto-registers the manifest in NATS and starts consuming tasks. No push step.

```sh
# The script must exist on the machine where wlow start runs.
./bin/wlow start --id my-proc --cmd "python3 my-proc/processor.py"
```

As a container (script is in the image):

```dockerfile
FROM python:3.12-slim
COPY my-proc/processor.py /app/processor.py
# RUN pip install your-deps
RUN curl -fL -o /usr/local/bin/wlow \
      https://github.com/wlow/wlow/releases/latest/download/wlow-linux-amd64 \
    && chmod +x /usr/local/bin/wlow
ENV NATS_URL=nats://nats:4222
ENTRYPOINT ["wlow", "start", "--id", "my-proc", "--cmd", "python3 /app/processor.py"]
```

### WASM processor — push required (binary must be stored)

```sh
./bin/wlow push \
  --id my-wasm --version v1 --runtime wasm \
  --source wasm --path my-proc.wasm --tags latest
./bin/wlow start --runtimes wasm
```

### MicroVM processor — push required (rootfs image in OCI)

Requires BuildKit at `BUILDKIT_HOST` and an OCI registry.

```sh
./bin/wlow push \
  --id my-proc --version v1 --runtime microvm \
  --path my-proc/Dockerfile \
  --entrypoint python3,/app/processor.py \
  --registry ghcr.io/your-org/wlow-artifacts \
  --tags latest
```

Then deploy the **wlow runner image** on a KVM-capable host (we provide it):

```sh
docker run -e NATS_URL=nats://... \
  -e WLOW_REGISTRY=ghcr.io/your-org/wlow-artifacts \
  --device /dev/kvm:/dev/kvm --privileged \
  ghcr.io/wlow/wlow-runtime:latest \
  /usr/local/bin/wlow-runner --runtimes microvm
```

See [runner-setup.md](runner-setup.md) for KVM setup and [install.md](install.md) for Kubernetes.

---

## 6. Submit a workflow

```go
client, err := sdk.NewClient(sdk.ClientConfig{NATSUrl: "nats://localhost:4222"})
defer client.Close()

wf, err := workflow.NewBuilder("job-1").
    AddTask("step-a", workflow.Task{
        ProcessorID: "my-proc", ProcessorVersion: "latest",
        ExecutionMode: artifact.ExecutionSandboxed,
        Input: map[string]any{"text": "hello world"},
    }).
    Build()

result, err := client.SubmitAndWait(ctx, wf, 2*time.Minute)
```

Or from the command line (for testing):

```sh
go run ./examples/ffmpeg-process --nats nats://localhost:4222
```

---

## Verify the stack

```sh
# Check NATS streams
nats stream list

# Check a processor manifest registered
nats kv get wlow-artifact-manifests default.tag.my-proc.latest

# Check wlow help
./bin/wlow
```
