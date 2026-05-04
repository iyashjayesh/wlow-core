.PHONY: all build clean test lint fmt vet orchestrator processor \
        wlow wlow-runner-go wlow-agent ffmpeg-snapshot-push ffmpeg-benchmark \
        ffmpeg-cold-push linux-amd64-bins image image-push nats-port-forward \
        buildkit-port-forward deploy-control-plane deploy-runner deploy-all help

# Build configuration
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     := -s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)
GO_TAGS     ?= no_wasm
BUILD       := CGO_ENABLED=0 go build -tags $(GO_TAGS) -trimpath -ldflags="$(LDFLAGS)"

BIN             := bin
LINUX_AMD64_BIN := $(BIN)/linux-amd64

# Kubernetes / NATS
KUBE_NAMESPACE     ?= wlow
NATS_NAMESPACE     ?= nats
NATS_URL           ?= nats://127.0.0.1:4222
BUILDKIT_NAMESPACE ?= buildkit
BUILDKIT_HOST      ?= tcp://127.0.0.1:1234
export BUILDKIT_HOST

# OCI registry
REGISTRY ?= ghcr.io/wlow
IMAGE    ?= wlow-runtime
TAG      ?= $(VERSION)
IMAGE_REF  := $(REGISTRY)/$(IMAGE):$(TAG)
LATEST_REF := $(REGISTRY)/$(IMAGE):latest

ARTIFACT_REGISTRY ?=
SNAPSHOT_REGISTRY ?=

all: build

# ── Core binaries ─────────────────────────────────────────────────────────────

$(BIN):
	@mkdir -p $(BIN)

build: $(BIN) orchestrator processor

orchestrator: $(BIN)
	$(BUILD) -o $(BIN)/orchestrator ./cmd/orchestrator

processor: $(BIN)
	$(BUILD) -o $(BIN)/processor ./cmd/processor

# ── wlow toolchain ────────────────────────────────────────────────────────────

wlow: $(BIN)
	$(BUILD) -o $(BIN)/wlow ./cmd/wlow

# Go process/WASM runner (for dev and process-only deployments)
wlow-runner-go: $(BIN)
	$(BUILD) -o $(BIN)/wlow-runner-go ./cmd/wlow-runner

# The production microVM runner is in the wlow-runner repo (Rust).
# Build it there and copy the binary to bin/linux-amd64/wlow-runner before
# running `make image`.
# See: https://github.com/wlow/wlow-runner

# In-VM agent (linux/amd64 only — embedded in microVM rootfs)
wlow-agent: $(BIN)
	GOOS=linux GOARCH=amd64 $(BUILD) -o $(BIN)/wlow-agent.linux-amd64 ./cmd/wlow-agent

# ── Code quality ──────────────────────────────────────────────────────────────

fmt:
	@go fmt ./...

vet:
	@go vet ./...

lint: fmt vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed, skipping"

# ── Testing ───────────────────────────────────────────────────────────────────

test:
	go test -v -race -coverprofile=coverage.out ./...

test-short:
	go test -v -short ./...

cover: test
	go tool cover -html=coverage.out -o coverage.html

# ── Dependencies ──────────────────────────────────────────────────────────────

deps:
	go mod tidy
	go mod verify

# ── Cleanup ───────────────────────────────────────────────────────────────────

clean:
	rm -rf $(BIN) coverage.out coverage.html

# ── Example workflows ─────────────────────────────────────────────────────────

ffmpeg-cold-push: wlow
	@test -n "$(ARTIFACT_REGISTRY)" || (echo "ARTIFACT_REGISTRY is required" && exit 1)
	./$(BIN)/wlow push --nats $(NATS_URL) --registry $(ARTIFACT_REGISTRY) --id ffmpeg-cold-p0 --version cold-v1 --runtime microvm --source dockerfile --path examples/ffmpeg-cold-microvm/p0/Dockerfile --entrypoint python,/app/processor.py --tags latest,cold
	./$(BIN)/wlow push --nats $(NATS_URL) --registry $(ARTIFACT_REGISTRY) --id ffmpeg-cold-p1 --version cold-v1 --runtime microvm --source dockerfile --path examples/ffmpeg-cold-microvm/p1/Dockerfile --entrypoint python,/app/processor.py --tags latest,cold

ffmpeg-snapshot-push: wlow
	@test -n "$(ARTIFACT_REGISTRY)" || (echo "ARTIFACT_REGISTRY is required" && exit 1)
	@test -n "$(SNAPSHOT_REGISTRY)" || (echo "SNAPSHOT_REGISTRY is required" && exit 1)
	./$(BIN)/wlow push --nats $(NATS_URL) --registry $(ARTIFACT_REGISTRY) --snapshot-ref $(SNAPSHOT_REGISTRY)/ffmpeg-snapshot-p0:snapshot-v1 --id ffmpeg-snapshot-p0 --version cold-v1 --runtime microvm --path examples/ffmpeg-cold-microvm/p0/Dockerfile --entrypoint python,/app/processor.py --tags cold --snapshot --snapshot-version snapshot-v1 --snapshot-tags latest
	./$(BIN)/wlow push --nats $(NATS_URL) --registry $(ARTIFACT_REGISTRY) --snapshot-ref $(SNAPSHOT_REGISTRY)/ffmpeg-snapshot-p1:snapshot-v1 --id ffmpeg-snapshot-p1 --version cold-v1 --runtime microvm --path examples/ffmpeg-cold-microvm/p1/Dockerfile --entrypoint python,/app/processor.py --tags cold --snapshot --snapshot-version snapshot-v1 --snapshot-tags latest

ffmpeg-benchmark:
	go run ./examples/ffmpeg-process --nats $(NATS_URL) --workflow wf-ffmpeg-local --repeat 10
	go run ./examples/ffmpeg-cold-microvm --nats $(NATS_URL) --workflow wf-ffmpeg-cold --processor-prefix ffmpeg-cold --repeat 10
	go run ./examples/ffmpeg-cold-microvm --nats $(NATS_URL) --workflow wf-ffmpeg-snapshot --processor-prefix ffmpeg-snapshot --repeat 10

# ── Container image ───────────────────────────────────────────────────────────
# Prerequisite: copy the wlow-runner binary (from wlow-runner repo) to
# bin/linux-amd64/wlow-runner before building the runtime image.

image:
	buildctl --addr $(BUILDKIT_HOST) build \
	  --frontend dockerfile.v0 \
	  --local context=. --local dockerfile=. \
	  --opt filename=deploy/Dockerfile --opt platform=linux/amd64 \
	  --output "type=image,name=$(IMAGE_REF),push=true"

image-push: image
	@echo "Pushed $(IMAGE_REF)."

# ── Cross-compiled Linux/amd64 binaries ───────────────────────────────────────
# Note: does not build wlow-runner (Rust). Build that separately from
# https://github.com/wlow/wlow-runner and place at bin/linux-amd64/wlow-runner.

linux-amd64-bins:
	@mkdir -p $(LINUX_AMD64_BIN)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags no_wasm -trimpath -ldflags="$(LDFLAGS)" -o $(LINUX_AMD64_BIN)/orchestrator ./cmd/orchestrator
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags no_wasm -trimpath -ldflags="$(LDFLAGS)" -o $(LINUX_AMD64_BIN)/wlow-runner-go ./cmd/wlow-runner
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags no_wasm -trimpath -ldflags="$(LDFLAGS)" -o $(LINUX_AMD64_BIN)/wlow ./cmd/wlow
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags no_wasm -trimpath -ldflags="$(LDFLAGS)" -o $(LINUX_AMD64_BIN)/wlow-mcp ./cmd/wlow-mcp
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags no_wasm -trimpath -ldflags="$(LDFLAGS)" -o $(LINUX_AMD64_BIN)/wlow-agent ./cmd/wlow-agent

# ── Kubernetes ────────────────────────────────────────────────────────────────

buildkit-port-forward:
	kubectl port-forward -n $(BUILDKIT_NAMESPACE) svc/buildkitd 1234:1234

nats-port-forward:
	kubectl port-forward -n $(NATS_NAMESPACE) svc/nats 4222:4222

deploy-control-plane:
	kubectl apply -n $(KUBE_NAMESPACE) -f deploy/k8s/control-plane.yaml
	kubectl rollout status -n $(KUBE_NAMESPACE) deploy/wlow-control-plane

deploy-runner:
	kubectl apply -n $(KUBE_NAMESPACE) -f deploy/k8s/runner.yaml
	kubectl rollout status -n $(KUBE_NAMESPACE) deploy/wlow-runner

deploy-all:
	kubectl apply -k deploy/k8s
	kubectl rollout status -n $(KUBE_NAMESPACE) deploy/wlow-control-plane
	kubectl rollout status -n $(KUBE_NAMESPACE) deploy/wlow-runner

# ── Help ──────────────────────────────────────────────────────────────────────

help:
	@echo ""
	@echo "wlow-core — build targets"
	@echo ""
	@echo "Quick start:"
	@echo "  make wlow                                        build the CLI"
	@echo "  ./bin/wlow start --control-plane                 start the control plane"
	@echo "  ./bin/wlow start --id P --cmd 'python3 /app/p.py'  run a processor"
	@echo ""
	@echo "Build:"
	@echo "  wlow               CLI (start / new / push / prepare-snapshot / benchmark)"
	@echo "  build              orchestrator + processor"
	@echo "  wlow-runner-go     Go process/WASM runner"
	@echo "  wlow-agent         In-VM agent (linux/amd64)"
	@echo "  linux-amd64-bins   All Go binaries cross-compiled for linux/amd64"
	@echo ""
	@echo "  Note: the Rust microVM runner is in https://github.com/wlow/wlow-runner"
	@echo "  Build it there and copy to bin/linux-amd64/wlow-runner before make image."
	@echo ""
	@echo "Quality:"
	@echo "  test / test-short  Run tests"
	@echo "  lint / fmt / vet   Code quality"
	@echo "  deps               go mod tidy + verify"
	@echo ""
	@echo "Image:  make image  (requires BuildKit at BUILDKIT_HOST)"
	@echo "Deploy: deploy-control-plane | deploy-runner | deploy-all"
	@echo ""
