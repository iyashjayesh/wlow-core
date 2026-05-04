# Operations Reference

## Full environment variable reference

### Orchestrator (`cmd/orchestrator`)

| Variable | Default | Description |
|----------|---------|-------------|
| `NATS_URL` | `nats://localhost:4222` | NATS server URL |
| `STORE_BUCKET` | `workflow-state` | Workflow state KV bucket name |
| `WORKFLOW_STREAM` | `WLOW_WORKFLOW` | JetStream stream name for workflow subjects |
| `WORKFLOW_SUBJECT_PREFIX` | `workflow` | Subject prefix for workflow messages |
| `PROCESSOR_STREAM` | `WLOW_PROCESSOR` | JetStream stream name for processor subjects |
| `PROCESSOR_SUBJECT_PREFIX` | `wlow.processor` | Subject prefix for sandboxed processor tasks |
| `METRICS_PORT` | `2112` | Prometheus metrics port |
| `DASHBOARD_PORT` | `8085` | HTTP dashboard port |
| `SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |
| `MAX_RETRIES` | `3` | Task retry attempts |
| `ACK_TIMEOUT` | `5m` | Task ack/execution timeout |
| `STREAM_MAX_BYTES` | `1073741824` | Max bytes for JetStream streams |

### wlow-runner (`cmd/wlow-runner`)

| Variable | Default | Description |
|----------|---------|-------------|
| `NATS_URL` | `nats://localhost:4222` | NATS server URL |
| `WLOW_NODE_ID` | hostname | Unique runner identity (locality heartbeats) |
| `WLOW_RUNTIMES` | `process` | Comma-separated runtimes (e.g. `process,microvm`) |
| `WLOW_DATA_DIR` | `/var/lib/wlow` | Scratch dir for rootfs, snapshots, VM sockets |
| `WLOW_CONCURRENCY` | `4` | Max in-flight tasks |
| `WLOW_HEARTBEAT` | `15s` | Locality heartbeat interval |
| `WLOW_REGISTRY` | — | OCI registry prefix for artifact images |
| `WLOW_SNAPSHOT_REGISTRY` | `WLOW_REGISTRY` | OCI registry prefix for snapshot images |
| `WLOW_AGENT_BINARY` | `/usr/local/bin/wlow-agent` | Path to in-VM agent binary |
| `WLOW_FIRECRACKER_BINARY` | `/usr/local/bin/firecracker` | Path to Firecracker binary |
| `WLOW_KERNEL_PATH` | `/var/lib/wlow/kernel/vmlinux` | Path to microVM kernel |
| `WLOW_FIRECRACKER_ENABLE_PCI` | `true` | Enable PCI bus for virtio-pmem DAX |
| `WLOW_MKFS_EROFS` | `/usr/bin/mkfs.erofs` | Path to mkfs.erofs |
| `IMAGE_PULL_AUTH` | — | Base64-encoded Docker config for private OCI registries |
| `STORE_BUCKET` | `workflow-state` | Workflow state KV bucket name |
| `WORKFLOW_SUBJECT_PREFIX` | `workflow` | Must match orchestrator |
| `PROCESSOR_STREAM` | `WLOW_PROCESSOR` | Must match orchestrator |
| `PROCESSOR_SUBJECT_PREFIX` | `wlow.processor` | Must match orchestrator |

### wlow-agent (`cmd/wlow-agent`, in-VM)

These are typically injected via kernel command line in the Firecracker boot config:

| Variable | Description |
|----------|-------------|
| `WLOW_VSOCK_PORT` | Vsock port to connect to host (default: 1024) |
| `WLOW_VSOCK_CID` | Host CID for vsock (default: 2) |
| `WLOW_VSOCK_PATH` | Unix socket path (test/dev fallback) |
| `WLOW_CHROOT` | Optional chroot path |
| `WLOW_AFTER_RESTORE` | Command to run on VM-gen-ID change (snapshot restore) |

## NATS subject layout

```
{WORKFLOW_SUBJECT_PREFIX}.submit            → orchestrator: new workflow
{WORKFLOW_SUBJECT_PREFIX}.result.*          → orchestrator: task results
{WORKFLOW_SUBJECT_PREFIX}.cancel            → orchestrator: cancel workflow
{WORKFLOW_SUBJECT_PREFIX}.cancel.<id>       → runner: cancel running task
{WORKFLOW_SUBJECT_PREFIX}.reply.<id>        → client: final workflow result

{PROCESSOR_SUBJECT_PREFIX}.sandbox.*       → runner: sandboxed task dispatch
```

Both the orchestrator and all runners must use the same subject prefixes.

## Snapshot preparation

Because snapshot creation requires Firecracker to run inside the runner pod (with `/dev/kvm`), `wlow prepare-snapshot` must be executed from within the runner pod:

```sh
kubectl -n wlow exec deploy/wlow-runner -- \
  /usr/local/bin/wlow prepare-snapshot \
    --nats nats://nats.nats:4222 \
    --id <processor-id> \
    --from <cold-version-or-tag> \
    --version snapshot-v1 \
    --snapshot-ref <your-oci-registry>/<processor-id>:snapshot-v1 \
    --tags latest
```

`wlow push --snapshot` attempts this locally and requires a local `wlow-runner` binary — only useful in dev environments where the runner binary is co-located with the CLI.

## OCI registry auth

Set `IMAGE_PULL_AUTH` to a base64-encoded Docker config JSON:

```sh
# GCP Artifact Registry example
TOKEN=$(gcloud auth print-access-token)
AUTH=$(echo -n "oauth2accesstoken:${TOKEN}" | base64)
export IMAGE_PULL_AUTH="{\"auths\":{\"REGION-docker.pkg.dev\":{\"auth\":\"${AUTH}\"}}}"
```

Or mount a Kubernetes secret referencing `IMAGE_PULL_AUTH`.

## Monitoring

Prometheus metrics are exposed on `:2112/metrics`. Key metrics:

- `workflow_submitted_total` — workflows submitted
- `workflow_completed_total` — workflows completed
- `task_duration_seconds` — per-runtime task execution time
- `task_retries_total` — retry events by reason

## Troubleshooting

**Orchestrator can't find processor**: verify `PROCESSOR_SUBJECT_PREFIX` matches on both orchestrator and runner. Default is `wlow.processor`.

**Runner fails to pull OCI artifact**: check `IMAGE_PULL_AUTH`, `WLOW_REGISTRY`, and that the registry is reachable from the runner pod.

**Snapshot workflow times out**: snapshot preparation must have run successfully first. Check NATS for `{tenant}.tag.<processor-id>.latest` pointing to a `snapshot-v1` manifest with all four snapshot roles (`snapshot.config`, `snapshot.state`, `snapshot.memory`, `snapshot.rootfs`).
