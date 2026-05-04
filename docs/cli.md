# wlow CLI Reference

The `wlow` binary: `start`, `new`, `push`, `prepare-snapshot`, `benchmark`.

## wlow start

Starts the control plane or a process/WASM processor runner.

```
wlow start [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--control-plane` | `false` | Start the control plane (orchestrator) instead of a runner |
| `--nats` | `NATS_URL` / `nats://localhost:4222` | NATS server URL |
| `--id` | — | Processor ID to auto-register (use with `--cmd`) |
| `--cmd` | — | Command to run per task, e.g. `"python3 /app/processor.py"` — auto-registers manifest, no push needed |
| `--runtimes` | `WLOW_RUNTIMES` / `process` | Runtimes to serve: `process`, `wasm` |
| `--concurrency` | `WLOW_CONCURRENCY` / `4` | Max in-flight tasks |
| `--store-bucket` | `STORE_BUCKET` / `workflow-state` | Workflow state KV bucket |
| `--processor-subject-prefix` | `PROCESSOR_SUBJECT_PREFIX` / `wlow.processor` | Must match control plane |
| `--workflow-subject-prefix` | `WORKFLOW_SUBJECT_PREFIX` / `workflow` | Must match control plane |

### Examples

```sh
# Start the control plane
wlow start --control-plane

# Start a Python processor — no push needed
wlow start --id my-proc --cmd "python3 /app/processor.py"

# Start a WASM runner (after wlow push for WASM)
wlow start --runtimes wasm

# Start with multiple runtimes and higher concurrency
wlow start --runtimes process,wasm --concurrency 8
```

---

## wlow new

Scaffold a new processor project.

```
wlow new <name> [--lang python|go]
```

Creates `<name>/processor.py` (or `main.go`) and a `Dockerfile` ready for `wlow start`.

---

## wlow push

Build and register a processor artifact in NATS + OCI.

```
wlow push [flags]
```

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--nats` | `NATS_URL` | `nats://localhost:4222` | NATS server |
| `--id` | — | required | Processor ID |
| `--version` | — | `v1` | Processor version |
| `--runtime` | — | `wasm` | Runtime: `wasm`, `process`, `microvm`, `snapshot` |
| `--source` | — | inferred | Source kind: `wasm`, `dockerfile`, `tarball`, `binary` |
| `--path` | — | required | Path to source (Dockerfile, binary, etc.) |
| `--entrypoint` | — | — | Comma-separated entrypoint (e.g. `python,/app/processor.py`) |
| `--tags` | — | `latest` | Comma-separated tags to publish |
| `--registry` | `WLOW_REGISTRY` | — | OCI registry prefix for rootfs images |
| `--image-ref` | — | — | Full OCI ref override |
| `--tenant` | — | `default` | Multi-tenant namespace |
| `--platform` | — | host | Docker build platform |

### Snapshot flags (microvm only)

| Flag | Default | Description |
|------|---------|-------------|
| `--snapshot` | `false` | Prepare and push snapshot after cold push |
| `--snapshot-version` | `snapshot-v1` | Snapshot processor version |
| `--snapshot-tags` | `latest` | Tags for snapshot version |
| `--snapshot-ref` | — | OCI image ref for snapshot data |
| `--data-dir` | `WLOW_DATA_DIR` / `/var/lib/wlow` | Working dir for snapshot prep |

> **Important**: `--snapshot` invokes `wlow-runner` locally to prepare the Firecracker snapshot. In production, run `wlow prepare-snapshot` inside the runner pod instead. See [operations.md](operations.md#snapshot-preparation).

### Examples

```sh
# Push a WASM processor
wlow push --id my-wasm --runtime wasm --path ./processor.wasm --tags v1,latest

# Push a cold microVM processor from a Dockerfile
wlow push \
  --id my-proc --version cold-v1 --runtime microvm \
  --path ./Dockerfile --entrypoint python,/app/main.py \
  --registry ghcr.io/your-org/wlow-artifacts \
  --tags latest,cold
```

## wlow prepare-snapshot

Prepare and publish snapshot artifacts from inside a runner pod.

```
wlow prepare-snapshot [flags]
```

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--nats` | `NATS_URL` | `nats://localhost:4222` | NATS server |
| `--id` | — | required | Processor ID to snapshot |
| `--from` | — | `latest` | Source processor version/tag |
| `--version` | — | `snapshot-v1` | Snapshot version string |
| `--tags` | — | `latest` | Tags for snapshot version |
| `--snapshot-ref` | — | required | OCI image ref for snapshot data |
| `--data-dir` | `WLOW_DATA_DIR` | `/var/lib/wlow` | Working dir |
| `--tenant` | — | `default` | Tenant |

### Usage in runner pod

```sh
kubectl -n wlow exec deploy/wlow-runner -- \
  /usr/local/bin/wlow prepare-snapshot \
    --nats nats://nats.nats:4222 \
    --id my-proc \
    --from cold-v1 \
    --version snapshot-v1 \
    --snapshot-ref ghcr.io/your-org/wlow-snapshots/my-proc:snapshot-v1 \
    --tags latest
```

## wlow benchmark

Run sequential and concurrent workflow timing tests.

```
wlow benchmark [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--nats` | `nats://localhost:4222` | NATS server |
| `--workflow-subject-prefix` | `wlow.workflow` | Subject prefix |
| `--processor-prefix` | required | Processor ID prefix (uses `<prefix>-p0`, `<prefix>-p1`) |
| `--runtime` | `process` | Runtime under test |
| `--repeat` | `10` | Sequential runs |
| `--concurrent` | `0` | Concurrent workflows (0 = sequential only) |

## Runtime matrix

| Runtime | Binary | KVM required | OCI artifacts |
|---------|--------|-------------|---------------|
| `process` | `wlow-runner-go` | No | No |
| `wasm` | `wlow-runner-go` | No | No |
| `microvm` | `wlow-runner` (Rust) | Yes | EROFS rootfs |
| `snapshot` | `wlow-runner` (Rust) | Yes | EROFS rootfs + snapshot files |
