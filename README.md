# wlow

**wlow** is a DAG-based workflow orchestrator. You define a graph of tasks, submit it, and wlow runs each task in the right sandbox — with dependency tracking, retries, and a final result.

Processors run in three execution tiers:

| Runtime | How to run it | KVM |
|---------|--------------|-----|
| **process** — OS subprocess | `wlow start` on any machine with NATS | No |
| **wasm** — Wasmtime component | `wlow start --runtimes wasm` on any machine | No |
| **microvm** — Firecracker VM | wlow runner image on a KVM host | Yes |
| **snapshot** — Firecracker from snapshot | wlow runner image on a KVM host | Yes |

> **Status**: pre-1.0, actively developed.

---

## How it works

You write a processor, push it to wlow, and run it anywhere — the processor just needs NATS connectivity and the `wlow` binary (for process/WASM) or the wlow runner image (for microVMs).

```
                     wlow.processor.sandbox.>
Orchestrator ──────────────────────────────────► wlow start (your container)
     ▲                                                  └─ spawns your script/binary per task
     │
     │  workflow.reply.<id>
     └──────────────────────────────────────────── result
```

For microVM runtimes the runner is our image (with Firecracker inside), not yours.

---

## Install

```sh
# Linux / macOS — one-liner
curl -fsSL https://raw.githubusercontent.com/wlow/wlow-core/main/install.sh | sh

# Homebrew
brew install wlow/tap/wlow

# Go install (requires Go 1.23+)
go install github.com/wlow/wlow-core/cmd/wlow@latest

# Direct download — https://github.com/wlow/wlow-core/releases
```

Verify: `wlow version`

---

## Quickstart

### 1. Start NATS and the control plane

```sh
nats-server --js &
make wlow
./bin/wlow start --control-plane
```

### 2. Scaffold a processor

```sh
./bin/wlow new my-proc
# creates my-proc/processor.py and a Dockerfile
```

### 3. Start it — no push needed

```sh
# Any machine with wlow + python3 + NATS connectivity:
./bin/wlow start --id my-proc --cmd "python3 my-proc/processor.py"
```

`--cmd` auto-registers the processor manifest in NATS and starts consuming tasks immediately. No `wlow push` required for process or Go SDK processors.

### 4. Submit a workflow

```go
client, _ := sdk.NewClient(sdk.ClientConfig{NATSUrl: "nats://localhost:4222"})
wf, _ := workflow.NewBuilder("job-1").
    AddTask("step", workflow.Task{
        ProcessorID: "my-proc", ProcessorVersion: "latest",
        Input: map[string]any{"text": "hello world"},
    }).Build()
result, _ := client.SubmitAndWait(ctx, wf, time.Minute)
```

---

## The wlow CLI

```
wlow start --control-plane   Start the control plane (orchestrator)
wlow start --id P --cmd CMD  Start a process processor — no push needed
wlow start --runtimes wasm   Start a WASM processor runner
wlow new <name>              Scaffold a new processor project
wlow push                    Register a WASM or microVM processor artifact
wlow prepare-snapshot        Prepare snapshot artifacts (run from a KVM host)
wlow benchmark               Timing tests
```

### When do you need `wlow push`?

| Processor type | Need push? | How to run |
|---------------|-----------|-----------|
| Go SDK (`sdk.NewRunner`) | **No** | build your binary, run it |
| Python/Node script | **No** | `wlow start --id P --cmd "python3 /app/p.py"` |
| WASM component | **Yes** — binary stored in NATS | `wlow push`, then `wlow start --runtimes wasm` |
| MicroVM (Dockerfile) | **Yes** — rootfs image in OCI | `wlow push --runtime microvm`, then deploy runner image |

---

## Deploying a process processor as a container

The container owns its runtime (python3, dependencies, the script). `wlow start --cmd` registers the processor manifest on startup — no prior push step.

```dockerfile
# my-proc/Dockerfile
FROM python:3.12-slim

COPY my-proc/processor.py /app/processor.py
# RUN pip install your-dependencies

RUN curl -fL -o /usr/local/bin/wlow \
      https://github.com/wlow/wlow/releases/latest/download/wlow-linux-amd64 \
    && chmod +x /usr/local/bin/wlow

ENV NATS_URL=nats://nats:4222
ENTRYPOINT ["wlow", "start", "--id", "my-proc", "--cmd", "python3 /app/processor.py"]
```

Build and run:

```sh
docker build -t my-proc:latest .
docker run -e NATS_URL=nats://your-nats:4222 my-proc:latest
```

Scale by running more instances. They all share the same NATS task queue.

---

## MicroVM processors

For `microvm` or `snapshot`, you push a Dockerfile:

```sh
wlow push --id my-proc --runtime microvm \
  --path my-proc/Dockerfile --entrypoint python3,/app/processor.py \
  --registry ghcr.io/your-org/wlow-artifacts
```

Then deploy the wlow runner image on a KVM-capable host and it handles execution. See [docs/runner-setup.md](docs/runner-setup.md) for KVM setup on GCP, AWS, and Linux workstations.

---

## Build

```sh
make wlow               # the CLI — start, new, push, prepare-snapshot, benchmark
make linux-amd64-bins   # all binaries cross-compiled for linux/amd64
```

---

## Documentation

| Doc | What it covers |
|-----|---------------|
| [docs/architecture.md](docs/architecture.md) | How the system works end-to-end |
| [docs/setup.md](docs/setup.md) | Running the server and processors |
| [docs/runner-setup.md](docs/runner-setup.md) | KVM setup for microVM runners (GCP, AWS, Linux) |
| [docs/examples.md](docs/examples.md) | One pipeline, two processors, all four runtimes |
| [docs/cli.md](docs/cli.md) | Full CLI reference |
| [docs/sdk.md](docs/sdk.md) | Go SDK: typed processors and workflow submission |
| [docs/artifacts.md](docs/artifacts.md) | Manifest model and OCI storage |
| [docs/install.md](docs/install.md) | Container image and Kubernetes deployment |
| [docs/operations.md](docs/operations.md) | Env vars, NATS subjects, monitoring |
| [docs/mcp.md](docs/mcp.md) | MCP server — AI agent and IDE integration |

---

## Performance (single runner, KVM)

| Runtime | p50 | p99 |
|---------|-----|-----|
| process | ~50ms | ~90ms |
| wasm | ~25ms | ~45ms |
| microvm | ~2.06s | ~2.12s |
| snapshot | ~862ms | ~924ms |

---

## Repositories

| Repo | Contents |
|------|----------|
| **wlow-core** (this repo) | CLI, control plane, Go SDK, process/WASM runner, examples |
| [wlow-runner](https://github.com/wlow/wlow-runner) | Rust microVM runner — Firecracker, vsock, snapshot |
| [wlow-charts](https://github.com/wlow/wlow-charts) | Helm charts for Kubernetes deployment |

The runtime container image bundles binaries from both wlow-core and wlow-runner.

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) · [SECURITY.md](SECURITY.md) · Apache-2.0
