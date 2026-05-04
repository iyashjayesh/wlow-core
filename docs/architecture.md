# Architecture

## What wlow is

wlow is a workflow orchestrator. You define a DAG (directed acyclic graph) of tasks, submit it to the wlow server, and wlow schedules and executes each task on a worker — respecting dependencies, retrying failures, and returning a final aggregated result.

What makes wlow different from a typical workflow system is the **execution model for workers**: tasks run inside sandboxed processors, and the sandbox level can be tuned from a plain OS subprocess all the way to a hardware-isolated Firecracker microVM — with the same workflow API regardless.

---

## The three moving parts

### 1. The wlow server (control plane)

The control plane is a single binary (`orchestrator`). It does one job: schedule and track task execution. It does not execute any task payload itself.

When you submit a workflow:
1. The server validates the DAG structure.
2. It writes all task states to a NATS KV store (state: `pending`).
3. It publishes the root tasks (those with no unmet dependencies) to a NATS stream.
4. Runners consume tasks from that stream.
5. When a task result arrives, the server checks which downstream tasks are now unblocked and publishes them.
6. When all tasks finish, the server returns the final result to the caller.

The server never touches processor binaries or OCI images. It only talks to NATS.

**You always need this.** It has no dependency on KVM, Docker, BuildKit, or a registry. All it needs is a NATS server.

### 2. The runner (execution plane)

The runner is what actually executes task payloads. There are two different things called "runner" in wlow, and it's important to understand the distinction:

**`wlow start` (your container)** handles `process` and `wasm` runtimes.

For **process** (scripts, binaries): write your processor, run `wlow start --id my-proc --cmd "python3 /app/processor.py"`. No push step. The `--cmd` flag auto-registers the manifest in NATS on startup. Your container provides the runtime environment (python3, dependencies, the script). `wlow start` provides the NATS consumer loop and JSON I/O. Scale by running more instances of your container.

For **WASM**: compile to a `.wasm` binary, `wlow push` it once (the binary is the artifact that gets stored in NATS), then `wlow start --runtimes wasm` anywhere. Push is required here because the WASM binary must be stored somewhere the runner can pull it from.

For **Go SDK processors**: they don't use `wlow start` at all. The binary embeds the processor logic and the NATS consumer loop via `pkg/sdk`. Just build and run it.

**`wlow-runner` (our image)** handles `microvm` and `snapshot` runtimes. We provide this image. It contains Firecracker, the kernel, the in-VM agent, and all microVM infrastructure. You push a Dockerfile and the runner image boots it in a Firecracker VM for each task. You deploy our image on a KVM-capable host.

Both types:
- Subscribe to the same NATS processor stream.
- Resolve processor manifests from NATS KV.
- Report results back via NATS.
- Can run in parallel for horizontal scale.

Runners are the only component with runtime-specific requirements:
- `wlow start`: just needs the host OS and NATS. No Docker, no KVM.
- `wlow-runner`: needs `/dev/kvm`, Firecracker, a kernel, and an OCI registry.

### 3. Processors

A processor is the unit of work — the function a task executes. Processors are registered with wlow via `wlow push`, which stores a manifest in NATS KV and (for microVM runtimes) pushes a payload to an OCI registry.

The same processor interface (input JSON → output JSON) works across all runtimes. The orchestrator does not need to know which runtime a processor uses; it just routes tasks to runners that support it.

---

## Execution tiers

```
Runtime                   Isolation      Startup      Requirements
────────────────────────────────────────────────────────────────────
process                   OS process     ~10ms        None
wasm                      Wasmtime VM    ~20ms        None
microvm       Firecracker    ~2s          KVM + OCI registry
snapshot     Firecracker    ~860ms       KVM + OCI registry + snapshot prep
```

### process

A subprocess on the runner host. The runner spawns a command (e.g. `python3 script.py`), writes JSON to stdin, and reads JSON from stdout. Fast and simple; no isolation between tasks or from the host.

Use this for: trusted workloads, development, lightweight transforms.

### wasm

A [WASM component](https://component-model.bytecodealliance.org/) executed inside Wasmtime. The component is compiled against the `wlow:core` WIT world (`wit/wlow/core/world.wit`). It is memory-isolated and can only access the outside world through explicitly imported capabilities (state, HTTP, blob, logging). No filesystem access, no network by default.

Use this for: untrusted code that should not touch the host, deterministic computation.

### microvm

A fresh Firecracker microVM is booted for each task. The root filesystem is an EROFS image served over virtio-pmem with DAX (direct-access, no page cache copy). The in-VM agent (`wlow-agent`) connects to the host via vsock, receives a task envelope, runs the processor command, and returns the result. The VM is discarded after the task completes.

Startup time is ~2s (kernel boot + agent ready). Use this for: untrusted code that needs a full OS environment, strict isolation, or large dependency footprints.

### snapshot

Same as cold boot but the VM is restored from a pre-captured Firecracker snapshot instead of booting from scratch. The snapshot captures VM memory after the processor's environment is fully initialized (imports loaded, runtime warm). On restore, the VM resumes in ~860ms total (memory load 2–5ms, agent ready 43ms, task 640ms).

Snapshot preparation is a one-time step: you boot a cold VM, run the processor's "before snapshot" initialization, pause the VM, and save the memory + VM state to an OCI image. From then on, every task restore takes the same time regardless of how heavy the initialization was.

Use this for: production microVM workloads where cold-boot latency is unacceptable.

---

## Data storage

```
NATS JetStream KV                      OCI Registry
──────────────────────────────         ──────────────────────────────
wlow-artifact-manifests                rootfs EROFS images
  → processor manifests (JSON)           microvm payloads
wlow-artifact-refs                     snapshot OCI images
  → reference counts for GC               snapshot.state  (VM state)
wlow-tenant-keys                           snapshot.memory (guest RAM)
  → per-tenant encryption keys             snapshot.rootfs (EROFS)
wlow-tenant-quotas                         snapshot.config (metadata)
wlow-output-cache
  → task output dedup (idempotency)
wlow-node-inventory
  → runner heartbeats (locality)
workflow-state KV
  → per-workflow task states
  → per-workflow input/output data
```

**NATS carries only metadata.** Manifests, state, and references are small JSON documents. Binary payloads (rootfs images, snapshot memory) are stored in OCI and pulled by runners on demand.

---

## Request flow: end to end

```
1. Client submits workflow JSON
   → NATS: {prefix}.submit
   → Orchestrator validates DAG

2. Orchestrator writes task states
   → NATS KV: workflow-state

3. Orchestrator publishes ready tasks
   → NATS stream: wlow.processor.sandbox.{runtime}.{processorID}

4. Runner picks up a task
   → Resolves manifest: NATS KV wlow-artifact-manifests
   → Pulls payload if needed: OCI registry (cached locally)
   → Executes sandbox
   → Writes result to NATS KV (state = completed/failed)
   → Publishes result: {prefix}.result.{taskID}

5. Orchestrator handles result
   → Checks downstream tasks in DAG
   → Publishes newly unblocked tasks (back to step 3)
   → When all tasks done: replies to {prefix}.reply.{workflowID}

6. Client receives final aggregated result
```

---

## Processor manifest

Every pushed processor has a manifest in NATS KV. The manifest tells runners where the payload is and how to execute it:

```json
{
  "kind": "wlow.processor.manifest.v1",
  "tenant": "default",
  "processor_id": "my-proc",
  "version": "v1",
  "runtime": "microvm",
  "io_protocol": "json-vsock-v0",
  "entrypoint": "python3,/app/processor.py",
  "artifacts": {
    "rootfs": {
      "kind": "oci-descriptor",
      "remote": {
        "ref": "ghcr.io/your-org/wlow-artifacts/my-proc:v1",
        "digest": "sha256:abc123...",
        "size": 45678901
      }
    }
  }
}
```

For `process` runtime, the payload is inline (a script blob stored in NATS directly). For `wasm`, `microvm`, and `snapshot`, the payload is an OCI reference pulled by the runner.

---

## Subject layout

The server and runners must agree on subject prefixes. Defaults:

```
Control plane:
  {WORKFLOW_SUBJECT_PREFIX}.submit          inbound workflow submissions
  {WORKFLOW_SUBJECT_PREFIX}.result.*        task results from runners
  {WORKFLOW_SUBJECT_PREFIX}.cancel          cancel workflow request
  {WORKFLOW_SUBJECT_PREFIX}.reply.*         outbound final results to clients

Runner:
  {PROCESSOR_SUBJECT_PREFIX}.sandbox.*     task dispatch to runners
```

Override both with `WORKFLOW_SUBJECT_PREFIX` and `PROCESSOR_SUBJECT_PREFIX` env vars. The defaults are `workflow` and `wlow.processor`. All subjects must be covered by a JetStream stream the server creates on startup.

---

## Multi-runner deployment

Runners self-register by writing a heartbeat into the `wlow-node-inventory` NATS KV bucket every 15 seconds. The heartbeat records the node ID and which runtimes it supports. The orchestrator uses this for **locality-aware scheduling**: when a cold-boot task is dispatched, the server preferentially routes to the runner that already has the rootfs image locally cached, reducing OCI pull overhead.

If no locality hint exists (first execution), the task goes to any runner that supports the runtime.
