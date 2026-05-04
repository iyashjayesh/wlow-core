# Architecture

> The narrative architecture docs have moved to the `docs/` folder. This file is a concise reference for contributors.

## Component map

```
┌──────────┐   workflow.submit      ┌──────────────────────────────────────────────────┐
│  Client  │ ─────────────────────► │  Orchestrator  (cmd/orchestrator)                │
│          │ ◄───────────────────── │                                                  │
└──────────┘  workflow.reply.<id>   │  Engine     pkg/workflow/engine.go               │
                                    │  Result     pkg/workflow/result.go               │
                                    │  Cancel     pkg/workflow/cancel.go               │
                                    │  Locality   pkg/workflow/locality.go             │
                                    └────────────────────┬─────────────────────────────┘
                                                         │  wlow.processor.sandbox.>
                                                         ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│  NATS JetStream                                                                     │
│                                                                                     │
│  WLOW_WORKFLOW stream         WLOW_PROCESSOR stream      workflow-state KV          │
│  ├─ workflow.submit           ├─ wlow.processor.sandbox.>  ├─ workflow.<id>.*       │
│  ├─ workflow.result.*         └─ ...                       └─ ...                   │
│  └─ workflow.cancel                                                                 │
│                                                                                     │
│  wlow-artifact-manifests KV   wlow-output-cache KV         wlow-node-inventory KV  │
└────────────────────────────────────────┬────────────────────────────────────────────┘
                                         │
                    ┌────────────────────┴────────────────────┐
                    ▼                                         ▼
         ┌───────────────────┐                    ┌───────────────────┐
         │   wlow-runner-go  │                    │   wlow-runner     │
         │   (Go)            │                    │   (Rust)          │
         │                   │                    │                   │
         │   process         │                    │   cold-microvm-   │
         │   wasm            │                    │   rootfs          │
         └───────────────────┘                    │   snapshot-fork-  │
                                                  │   microvm         │
                                                  └────────┬──────────┘
                                                           │
                                              ┌────────────┴────────────┐
                                              ▼                         ▼
                                       OCI Registry              Firecracker VM
                                    (rootfs, snapshots)       + wlow-agent (PID-1)
```

## Key design decisions

**NATS is control-plane only.** Workflow state, manifests, and locality metadata live in NATS KV. No large binary blobs. OCI registry holds rootfs images and snapshot data.

**Runners are stateless.** Each runner advertises the runtimes it supports via a locality heartbeat. The orchestrator uses this to route tasks to compatible runners (work queue — first available wins; locality aware routing is additive).

**Snapshot fork model.** For `snapshot`, a single Firecracker snapshot is materialized once per runner node. On each task dispatch, the runner restores from the frozen snapshot, services the task, then discards the VM. No persistent VM state between tasks.

**No cross-component sync on manifest writes.** `wlow push` writes manifests to NATS KV atomically. Runners resolve manifests on every task dispatch, so a new version is live as soon as the push completes and the tag is updated.

## Data flows

### Workflow submission

```
Client → workflow.submit → Engine.HandleWorkflow
  → validate DAG
  → write task states (pending) to workflow-state KV
  → publish root tasks to wlow.processor.sandbox.*
```

### Task execution

```
wlow.processor.sandbox.* → Runner.execute
  → resolve manifest from wlow-artifact-manifests KV
  → pull OCI artifact (rootfs or snapshot) if not cached
  → execute (process | WASM | microVM)
  → write task state (completed/failed)
  → publish to workflow.result.<task_id>
```

### Result handling

```
workflow.result.<task_id> → ResultHandler.HandleResult
  → update task state in KV
  → check if downstream tasks are now unblocked
  → publish newly ready tasks to wlow.processor.sandbox.*
  → if all tasks done: publish final result to workflow.reply.<wf_id>
```

## Package responsibilities

| Package | Responsibility |
|---------|---------------|
| `pkg/workflow` | DAG engine, state machine, dependency resolution, locality |
| `pkg/artifact` | Manifest CRUD on NATS KV, OCI descriptor client, snapshot model |
| `pkg/sandbox` | Executor registry, process/WASM/microVM dispatch, snapshot prep |
| `pkg/sdk` | Client-side workflow builder and typed processor runner |
| `pkg/build` | Dockerfile → EROFS rootfs builder, WASM builder |
| `pkg/nats` | NATS client wrapper, KV store, stream/consumer management |
| `pkg/config` | Environment-based config for the orchestrator |
| `pkg/capability` | WASM capability bus — state, HTTP, blob, log, MCP |
| `pkg/microvm` | Firecracker REST API, vsock, UFFD CoW page cache, vmgenid |

## See also

- [docs/artifacts.md](docs/artifacts.md) — manifest format, OCI payload layout, snapshot roles
- [docs/operations.md](docs/operations.md) — NATS subjects, env vars, snapshot prep workflow
- [docs/cli.md](docs/cli.md) — `wlow push`, `prepare-snapshot` reference
