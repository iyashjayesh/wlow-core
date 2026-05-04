# Install and Deploy

## Build binaries

```sh
# Core binaries
make wlow

# wlow CLI (push/snapshot/benchmark)
make wlow

# Go runner (process + WASM runtimes)
make wlow-runner-go

# Rust runner (Firecracker microVM runtimes)
make rust-runner
```

## Container image

The `deploy/Dockerfile` multi-stage build produces a single image containing all binaries plus the Firecracker binary and kernel (optional — see ARGs).

```sh
# Using BuildKit (recommended)
BUILDKIT_HOST=tcp://127.0.0.1:1234 REGISTRY=ghcr.io/your-org TAG=v0.1.0 make image

# Or plain docker
docker build -f deploy/Dockerfile -t your-org/wlow-runtime:latest .
```

### Build arguments

| ARG | Default | Description |
|-----|---------|-------------|
| `GO_VERSION` | `1.23` | Go toolchain version |
| `RUST_VERSION` | `1` | Rust toolchain version |
| `FIRECRACKER_VERSION` | `v1.14.4` | Firecracker release tag |
| `KERNEL_URL` | — | Pre-built vmlinux URL (skips kernel build) |
| `KERNEL_SHA256` | — | Expected SHA256 of pre-built vmlinux |
| `KERNEL_SOURCE_REF` | amazon linux 6.1 branch | Branch for kernel source build |

## Kubernetes deployment

### Prerequisites

- Kubernetes cluster with a NATS JetStream instance reachable at `nats://nats.nats:4222`
- Nodes for microVM runners must have `/dev/kvm` and be labeled `wlow.io/kvm=true`
- Namespace `wlow` created (`kubectl create namespace wlow`)

### Apply

```sh
# Override the image in kustomization.yaml first, then:
make deploy-all

# Or individually:
make deploy-control-plane
make deploy-runner
```

### Image override

Edit `deploy/k8s/kustomization.yaml` or pass an overlay:

```yaml
images:
  - name: ghcr.io/wlow/wlow-runtime
    newTag: v0.1.0
```

### Process-only runner (no KVM)

Remove the `nodeSelector` and `tolerations` blocks from `deploy/k8s/runner.yaml` and set:

```yaml
- name: WLOW_RUNTIMES
  value: process,wasm
```

### Private OCI registry

Set `WLOW_REGISTRY` (and optionally `WLOW_SNAPSHOT_REGISTRY`) in the runner deployment, plus `IMAGE_PULL_AUTH` if the registry requires auth. See the commented-out section in `deploy/k8s/runner.yaml`.
