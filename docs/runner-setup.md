# MicroVM Runner Setup

The `wlow-runner` (Rust) executes tasks in Firecracker microVMs. This requires `/dev/kvm` to be accessible on the host. This document covers how to get KVM working across common deployment environments.

---

## What is nested virtualization and why it matters

When you run a cloud VM (e.g. a GCP or AWS EC2 instance), you are already inside a virtual machine managed by the cloud hypervisor. To run Firecracker inside that VM, the cloud hypervisor must expose virtualization CPU extensions (Intel VT-x or AMD SVM) to your VM. This is called **nested virtualization**.

Without nested virtualization, `/dev/kvm` either does not exist or returns permission errors, and Firecracker will fail to start.

On **bare-metal** (a physical workstation or a dedicated server), there is no nesting — KVM works directly as long as the CPU supports it.

---

## Linux workstation (bare metal)

Bare-metal Linux machines almost always work out of the box.

**Check if KVM is available:**
```sh
ls -la /dev/kvm
# should exist and be readable by your user

# More detailed check (Ubuntu/Debian: apt install cpu-checker)
kvm-ok
```

**If `/dev/kvm` does not exist**, check that CPU virtualization extensions are enabled in BIOS:
```sh
# Intel: look for vmx
grep -c vmx /proc/cpuinfo

# AMD: look for svm
grep -c svm /proc/cpuinfo
```

**If KVM is available but not accessible**, add your user to the `kvm` group:
```sh
sudo usermod -aG kvm $USER
newgrp kvm    # or log out and back in
ls -la /dev/kvm  # should now show kvm group ownership
```

**Install KVM tools if not present (Ubuntu/Debian):**
```sh
sudo apt update && sudo apt install -y qemu-kvm
```

---

## GCP — VM (Compute Engine)

By default, GCP VMs do not expose nested virtualization. You must enable it when creating the instance.

### Create a VM with nested virtualization

```sh
gcloud compute instances create wlow-runner \
  --machine-type=n2-standard-4 \
  --image-family=debian-12 \
  --image-project=debian-cloud \
  --enable-nested-virtualization \
  --zone=us-central1-a

# Verify inside the VM
ssh wlow-runner
ls /dev/kvm    # should exist
```

**Supported machine types for nested virtualization:** n2, n2d, c2, c2d, and t2d. The older n1 series does NOT support nested virtualization.

### Enable nested virtualization on an existing VM

You cannot enable nested virtualization on a running instance. You must stop it, modify it, and restart:

```sh
# Stop the instance
gcloud compute instances stop wlow-runner

# Export config
gcloud compute instances export wlow-runner --destination=/tmp/wlow-runner.yaml

# Edit the YAML: under advancedMachineFeatures add:
#   enableNestedVirtualization: true
# Then import
gcloud compute instances import wlow-runner --source=/tmp/wlow-runner.yaml

# Or use the REST API directly
gcloud compute instances update wlow-runner \
  --enable-nested-virtualization
```

---

## GCP — GKE (Kubernetes)

Node pools in GKE support nested virtualization per pool. The runner pod requires both the node label and the `/dev/kvm` device mounted.

### Create a node pool with nested virtualization

```sh
gcloud container node-pools create wlow-kvm-pool \
  --cluster=my-cluster \
  --zone=us-central1-a \
  --machine-type=n2-standard-4 \
  --num-nodes=2 \
  --enable-nested-virtualization \
  --node-labels=wlow.io/kvm=true \
  --node-taints=wlow.io/kvm=true:NoSchedule \
  --image-type=UBUNTU_CONTAINERD
```

> **Note:** Use `UBUNTU_CONTAINERD` or `UBUNTU` as the image type. The default Container-Optimized OS (COS) does not expose `/dev/kvm` to pods.

### Verify KVM is accessible in a pod

```sh
kubectl run kvm-check --image=debian:bookworm-slim --rm -it \
  --overrides='{"spec":{"nodeSelector":{"wlow.io/kvm":"true"},"containers":[{"name":"kvm-check","image":"debian:bookworm-slim","command":["ls","-la","/dev/kvm"],"volumeMounts":[{"name":"kvm","mountPath":"/dev/kvm"}],"securityContext":{"privileged":true}}],"volumes":[{"name":"kvm","hostPath":{"path":"/dev/kvm","type":"CharDevice"}}]}}' \
  -- sh
```

### Runner pod configuration

The runner deployment (`deploy/k8s/runner.yaml`) already includes the required configuration:
- `nodeSelector: wlow.io/kvm: "true"`
- `tolerations` for the `wlow.io/kvm` taint
- `securityContext.privileged: true`
- `volumes: kvm` hostPath device mount

Set the OCI registry env vars in the pod spec:

```yaml
env:
  - name: WLOW_REGISTRY
    value: REGION-docker.pkg.dev/YOUR_PROJECT/YOUR_REPO/wlow-artifacts
  - name: WLOW_SNAPSHOT_REGISTRY
    value: REGION-docker.pkg.dev/YOUR_PROJECT/YOUR_REPO/wlow-snapshots
  - name: IMAGE_PULL_AUTH
    valueFrom:
      secretKeyRef:
        name: gar-pull-auth
        key: IMAGE_PULL_AUTH
```

Create the pull auth secret:

```sh
TOKEN=$(gcloud auth print-access-token)
AUTH=$(printf "oauth2accesstoken:%s" "$TOKEN" | base64)
kubectl -n wlow create secret generic gar-pull-auth \
  --from-literal=IMAGE_PULL_AUTH="{\"auths\":{\"REGION-docker.pkg.dev\":{\"auth\":\"${AUTH}\"}}}"
```

---

## AWS — EC2

AWS Nitro-based instances expose `/dev/kvm` natively on most modern instance types. Metal instances expose bare-metal KVM.

### Which instance types have KVM

| Category | Examples | KVM |
|----------|---------|-----|
| Metal (bare-metal) | `c5.metal`, `m5.metal`, `r5.metal`, `c6i.metal` | Always |
| Modern Nitro (non-metal) | `m5`, `c5`, `r5`, `m6i`, `c6i` (non-metal) | Usually — check `/dev/kvm` after launch |
| Older generation | `m4`, `c4` and older | Not supported |

The safest choice is `.metal` instances for guaranteed KVM availability. For regular Nitro instances, availability depends on the hypervisor configuration — test by checking `/dev/kvm` on a launched instance.

### Launch a KVM-capable instance

```sh
# Example: c6i.metal for guaranteed KVM
aws ec2 run-instances \
  --image-id ami-xxxxxxxxxxxxxxxxx \    # Amazon Linux 2023 or Ubuntu 22.04
  --instance-type c6i.metal \
  --key-name my-key \
  --security-group-ids sg-xxxxxxxxx \
  --subnet-id subnet-xxxxxxxxx

# SSH in and verify
ssh ec2-user@INSTANCE_IP
ls /dev/kvm
```

### Install KVM tools and Firecracker on Amazon Linux 2023

```sh
# KVM is exposed by the hypervisor; no kernel module needed
# Verify
ls /dev/kvm

# Install erofs tools (required for wlow push)
sudo dnf install -y e2fsprogs erofs-utils

# Download Firecracker
VERSION=v1.14.4
curl -fL -o firecracker.tgz \
  "https://github.com/firecracker-microvm/firecracker/releases/download/${VERSION}/firecracker-${VERSION}-x86_64.tgz"
tar -xzf firecracker.tgz
sudo cp firecracker-${VERSION}-x86_64/firecracker-${VERSION}-x86_64 /usr/local/bin/firecracker
sudo chmod +x /usr/local/bin/firecracker
```

---

## AWS — EKS

Use a managed node group with a `.metal` instance type or a KVM-enabled Nitro instance.

### Create a node group with KVM-capable instances

```sh
# Using eksctl
eksctl create nodegroup \
  --cluster=my-cluster \
  --name=wlow-kvm-nodes \
  --instance-types=m5.metal \
  --nodes=2 \
  --node-labels="wlow.io/kvm=true" \
  --node-taints="wlow.io/kvm=true:NoSchedule" \
  --asg-access

# Or with the AWS Console: create a managed node group with m5.metal
# and set node labels/taints via the launch template user data:
# --node-labels wlow.io/kvm=true
```

**AMI recommendation**: Use Amazon Linux 2 or Amazon Linux 2023 (not Bottlerocket for KVM workloads, as Bottlerocket restricts device access differently).

### Verify KVM in a pod

```sh
kubectl run kvm-check \
  --image=amazonlinux:2023 \
  --rm -it \
  --overrides='{"spec":{"nodeSelector":{"wlow.io/kvm":"true"},"containers":[{"name":"check","image":"amazonlinux:2023","command":["ls","-la","/dev/kvm"],"volumeMounts":[{"name":"kvm","mountPath":"/dev/kvm"}],"securityContext":{"privileged":true}}],"volumes":[{"name":"kvm","hostPath":{"path":"/dev/kvm","type":"CharDevice"}}]}}' \
  -- bash
```

---

## Installing Firecracker and kernel

The `deploy/Dockerfile` handles this in the container build. For a standalone runner outside Kubernetes:

### Firecracker binary

```sh
VERSION=v1.14.4
ARCH=x86_64   # or aarch64
curl -fL -o firecracker.tgz \
  "https://github.com/firecracker-microvm/firecracker/releases/download/${VERSION}/firecracker-${VERSION}-${ARCH}.tgz"
tar -xzf firecracker.tgz
sudo install -m 755 firecracker-${VERSION}-${ARCH}/firecracker-${VERSION}-${ARCH} /usr/local/bin/firecracker
```

### Kernel (vmlinux)

wlow requires a custom kernel built with EROFS, virtio-pmem, DAX, and vsock support. The Dockerfile kernel-builder stage builds one from Amazon Linux sources. To use a pre-built kernel:

1. Build the runtime image: `make image` — the resulting container includes `/var/lib/wlow/kernel/vmlinux`.
2. Copy it out: `docker create wlow-runtime:latest | docker cp <id>:/var/lib/wlow/kernel/vmlinux ./vmlinux`.
3. Set `WLOW_KERNEL_PATH=/path/to/vmlinux` in the runner environment.

A pre-built kernel tarball for x86_64 will be available as a GitHub release artifact.

---

## Runner environment variables (microVM)

```sh
export WLOW_FIRECRACKER_BINARY=/usr/local/bin/firecracker
export WLOW_KERNEL_PATH=/var/lib/wlow/kernel/vmlinux
export WLOW_DATA_DIR=/var/lib/wlow/data
export WLOW_REGISTRY=ghcr.io/your-org/wlow-artifacts
export WLOW_SNAPSHOT_REGISTRY=ghcr.io/your-org/wlow-snapshots
export WLOW_CONCURRENCY=4
export NATS_URL=nats://your-nats-server:4222

./bin/wlow-runner \
  --runtimes microvm,snapshot
```

See [operations.md](operations.md) for the complete variable reference.
