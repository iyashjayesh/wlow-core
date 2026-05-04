# Artifact Model

wlow separates **metadata** (NATS KV) from **payload** (OCI registry).

## Storage layers

```
┌────────────────────────────────────────────────────────────────────┐
│  NATS JetStream KV — metadata only                                 │
│                                                                    │
│  wlow-artifact-manifests    processor manifests (JSON)             │
│  wlow-artifact-refs         reference counts (for future GC)       │
│  wlow-tenant-keys           per-tenant encryption keys             │
│  wlow-tenant-quotas         per-tenant quota configs               │
│  wlow-output-cache          workflow task output dedup cache        │
│  wlow-node-inventory        runner heartbeats for locality sched.   │
└────────────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────────────┐
│  OCI Registry — large payloads                                     │
│                                                                    │
│  rootfs EROFS images        microvm runtime            │
│  snapshot state/memory      snapshot runtime          │
│  snapshot config JSON       per-snapshot metadata OCI layer        │
└────────────────────────────────────────────────────────────────────┘
```

## Manifest format

Every pushed processor has a manifest stored in NATS KV:

```json
{
  "kind": "wlow.processor.manifest.v1",
  "tenant": "default",
  "processor_id": "my-proc",
  "version": "cold-v1",
  "runtime": "microvm",
  "io_protocol": "json-vsock-v0",
  "artifacts": {
    "rootfs": {
      "kind": "oci-descriptor",
      "remote": {
        "ref": "ghcr.io/your-org/wlow-artifacts/my-proc:cold-v1",
        "digest": "sha256:abc123...",
        "size": 12345678,
        "media_type": "application/vnd.oci.image.manifest.v1+json"
      }
    }
  },
  "created_at": "2026-05-03T10:00:00Z"
}
```

## Tags

Tags are stored as separate KV entries pointing to a version string:

```
default.tag.my-proc.latest → cold-v1
default.tag.my-proc.cold   → cold-v1
```

Resolve a ref: `wlow push --tags latest,cold` writes both.

## Snapshot roles

A `snapshot` manifest bundles four artifact roles:

| Role | Content |
|------|---------|
| `snapshot.config` | VM config JSON |
| `snapshot.state` | Firecracker VM state binary |
| `snapshot.memory` | Guest memory snapshot |
| `snapshot.rootfs` | EROFS root filesystem |

All four are pushed as OCI layers under `WLOW_SNAPSHOT_REGISTRY/<processor-id>:<version>`.

## Runtimes

| Runtime constant | Description |
|-----------------|-------------|
| `process` | Subprocess via stdin/stdout JSON |
| `wasm` | Wasmtime WASM component |
| `microvm` | Firecracker, cold boot, EROFS rootfs via virtio-pmem |
| `snapshot` | Firecracker, restore from snapshot (fast path) |
| `attached` | In-process, no isolation |

## Tenancy

Manifests are namespaced by tenant: `{tenant}.processor.{id}.{version}`. The default tenant is `"default"`. Multi-tenant deployments use separate NATS KV key prefixes per tenant.
