# syntax=docker/dockerfile:1.7
#
# wlow runtime image.
#
# Prerequisites before building:
#   1. Build Go binaries:        make linux-amd64-bins
#   2. Get the Rust microVM runner from https://github.com/wlow/wlow-runner
#      and place it at:          bin/linux-amd64/wlow-runner
#   3. (Optional) Pre-built kernel at bin/linux-amd64/vmlinux.bin
#
# Then: make image   (or docker build -f deploy/Dockerfile .)

ARG GO_VERSION=1.23
ARG FIRECRACKER_VERSION=v1.14.4
ARG KERNEL_URL=
ARG KERNEL_SHA256=
ARG KERNEL_SOURCE_REF=microvm-kernel-6.1.102-1.182.amzn2023

FROM golang:${GO_VERSION}-bookworm AS go-builder
WORKDIR /src

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential pkg-config ca-certificates \
    && rm -rf /var/lib/apt/lists/*

ENV GOMAXPROCS=1 GOFLAGS=-p=1

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=1 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/orchestrator     ./cmd/orchestrator
RUN go build -trimpath -ldflags="-s -w" -o /out/wlow-runner-go   ./cmd/wlow-runner
RUN go build -trimpath -ldflags="-s -w" -o /out/wlow             ./cmd/wlow
RUN go build -trimpath -ldflags="-s -w" -o /out/wlow-mcp         ./cmd/wlow-mcp
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/wlow-agent ./cmd/wlow-agent

FROM debian:bookworm AS kernel-builder
ARG FIRECRACKER_VERSION
ARG KERNEL_URL
ARG KERNEL_SHA256
ARG KERNEL_SOURCE_REF
ARG TARGETARCH

RUN apt-get update && apt-get install -y --no-install-recommends \
        bc bison build-essential ca-certificates curl flex git libelf-dev libssl-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /kernel
RUN set -eux; \
    mkdir -p /out; \
    if [ -n "${KERNEL_URL}" ]; then \
      curl -fL -o /out/vmlinux "${KERNEL_URL}"; \
      if [ -n "${KERNEL_SHA256}" ]; then \
        echo "${KERNEL_SHA256}  /out/vmlinux" | sha256sum -c -; \
      fi; \
      exit 0; \
    fi; \
    case "${TARGETARCH}" in \
      amd64) config_arch=x86_64; make_arch=x86 ;; \
      arm64) config_arch=aarch64; make_arch=arm64 ;; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    git clone --depth 1 --branch "${KERNEL_SOURCE_REF}" https://github.com/amazonlinux/linux.git /kernel/linux; \
    cd /kernel/linux; \
    curl -fL -o .config \
      "https://raw.githubusercontent.com/firecracker-microvm/firecracker/${FIRECRACKER_VERSION}/resources/guest_configs/microvm-kernel-ci-${config_arch}-6.1.config"; \
    scripts/config \
      --enable ACPI --enable PCI --enable PCI_MSI --enable VIRTIO_PCI \
      --enable VIRTIO_BLK --enable VSOCKETS --enable VIRTIO_VSOCKETS \
      --enable VIRTIO_MMIO --enable DEVICE_MIGRATION --enable ZONE_DEVICE \
      --enable VIRTIO_PMEM --enable LIBNVDIMM --enable BLK_DEV_PMEM \
      --enable ND_CLAIM --enable ND_BTT --enable BTT --enable ND_PFN \
      --enable NVDIMM_PFN --enable NVDIMM_DAX --enable OF_PMEM \
      --enable NVDIMM_KEYS --enable DAX --enable DEV_DAX \
      --enable DEV_DAX_PMEM --enable DEV_DAX_KMEM --enable FS_DAX \
      --enable FS_DAX_PMD --enable EROFS_FS --enable EROFS_FS_XATTR \
      --enable EROFS_FS_POSIX_ACL; \
    make ARCH="${make_arch}" olddefconfig; \
    make ARCH="${make_arch}" -j"$(nproc)" vmlinux; \
    cp vmlinux /out/vmlinux

FROM debian:bookworm-slim AS firecracker
ARG FIRECRACKER_VERSION
ARG TARGETARCH

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tar \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /tools
RUN set -eux; \
    case "${TARGETARCH}" in \
      amd64) fc_arch=x86_64 ;; \
      arm64) fc_arch=aarch64 ;; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    curl -fL -o /tools/firecracker.tgz \
      "https://github.com/firecracker-microvm/firecracker/releases/download/${FIRECRACKER_VERSION}/firecracker-${FIRECRACKER_VERSION}-${fc_arch}.tgz"; \
    tar -xzf /tools/firecracker.tgz -C /tools; \
    find /tools -type f -name "firecracker-${FIRECRACKER_VERSION}-${fc_arch}" -exec cp {} /tools/firecracker \; ; \
    chmod +x /tools/firecracker

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates erofs-utils tini \
    && rm -rf /var/lib/apt/lists/*

COPY --from=go-builder /out/orchestrator     /usr/local/bin/orchestrator
COPY --from=go-builder /out/wlow-runner-go   /usr/local/bin/wlow-runner-go
COPY --from=go-builder /out/wlow             /usr/local/bin/wlow
COPY --from=go-builder /out/wlow-mcp         /usr/local/bin/wlow-mcp
COPY --from=go-builder /out/wlow-agent       /usr/local/bin/wlow-agent
COPY --from=firecracker /tools/firecracker   /usr/local/bin/firecracker
COPY --from=kernel-builder /out/vmlinux      /var/lib/wlow/kernel/vmlinux

# wlow-runner (Rust microVM runner) — build from https://github.com/wlow/wlow-runner
# and supply via the build context: bin/linux-amd64/wlow-runner
COPY bin/linux-amd64/wlow-runner /usr/local/bin/wlow-runner

RUN mkdir -p /var/lib/wlow/rootfs /var/lib/wlow/snapshots

ENV NATS_URL=nats://nats:4222 \
    STORE_BUCKET=workflow-state \
    WLOW_DATA_DIR=/var/lib/wlow \
    WLOW_AGENT_BINARY=/usr/local/bin/wlow-agent \
    WLOW_KERNEL_PATH=/var/lib/wlow/kernel/vmlinux \
    WLOW_FIRECRACKER_BINARY=/usr/local/bin/firecracker \
    WLOW_FIRECRACKER_ENABLE_PCI=true \
    WLOW_MKFS_EROFS=/usr/bin/mkfs.erofs

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["/usr/local/bin/orchestrator"]
