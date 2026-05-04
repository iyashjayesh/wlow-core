# Example: Text Analysis Pipeline

One workflow, two processors, four runtimes. The workflow splits text into tokens, then counts word frequencies.

```
submit text
    │
    ▼
[tokenize] ── tokens ──► [analyze] ── word counts ──► result
```

---

## The processors

### tokenize

Takes `{"text": "..."}`, returns `{"tokens": [...], "count": N}`.

**Python script** (`tokenize.py`):

```python
import json, re, sys

req = json.load(sys.stdin)
tokens = re.findall(r"[a-z]+", req.get("text", "").lower())
print(json.dumps({"tokens": tokens, "count": len(tokens)}))
```

**Go SDK** (implement `pkg/sdk.Processor`):

```go
type TokenizeInput  struct { Text string `json:"text"` }
type TokenizeOutput struct { Tokens []string `json:"tokens"`; Count int `json:"count"` }

type Tokenizer struct{}

func (t *Tokenizer) Process(_ context.Context, in TokenizeInput) (TokenizeOutput, error) {
    var tokens []string
    for _, w := range strings.Fields(strings.ToLower(in.Text)) {
        tokens = append(tokens, strings.Trim(w, `.,!?;:`))
    }
    return TokenizeOutput{Tokens: tokens, Count: len(tokens)}, nil
}
```

### analyze

Takes `{"tokens": [...]}`, returns `{"top": [{"word": "...", "n": N}, ...]}`.

**Python script** (`analyze.py`):

```python
import json, sys
from collections import Counter

req = json.load(sys.stdin)
freq = Counter(req.get("tokens", []))
top = [{"word": w, "n": c} for w, c in freq.most_common(5)]
print(json.dumps({"frequencies": dict(freq), "top": top}))
```

**Go SDK**:

```go
type AnalyzeInput  struct { Tokens []string `json:"tokens"` }
type AnalyzeOutput struct { Top []WordCount `json:"top"` }
type WordCount     struct { Word string `json:"word"`; N int `json:"n"` }

type Analyzer struct{}

func (a *Analyzer) Process(_ context.Context, in AnalyzeInput) (AnalyzeOutput, error) {
    freq := make(map[string]int, len(in.Tokens))
    for _, t := range in.Tokens { freq[t]++ }
    // sort by frequency descending
    var sorted []WordCount
    for w, n := range freq { sorted = append(sorted, WordCount{w, n}) }
    sort.Slice(sorted, func(i, j int) bool { return sorted[i].N > sorted[j].N })
    top := sorted
    if len(top) > 5 { top = top[:5] }
    return AnalyzeOutput{Top: top}, nil
}
```

---

## The workflow submission

This code is the same regardless of which runtime the processors use. Only the `ProcessorID` prefix changes.

```go
func submitTextAnalysis(ctx context.Context, natsURL, prefix, text string) error {
    client, err := sdk.NewClient(sdk.ClientConfig{NATSUrl: natsURL})
    if err != nil { return err }
    defer client.Close()

    wf, err := workflow.NewBuilder("text-analysis-001").
        AddTask("tokenize", workflow.Task{
            ExecutionMode:    artifact.ExecutionSandboxed,
            ProcessorID:      prefix + "-tokenize",
            ProcessorVersion: "latest",
            Input:            map[string]any{"text": text},
        }).
        AddTask("analyze", workflow.Task{
            ExecutionMode:    artifact.ExecutionSandboxed,
            ProcessorID:      prefix + "-analyze",
            ProcessorVersion: "latest",
        }, "tokenize"). // depends on tokenize output
        Build()
    if err != nil { return err }

    result, err := client.SubmitAndWait(ctx, wf, 2*time.Minute)
    if err != nil { return err }
    fmt.Printf("%v\n", result.Tasks["analyze"].Output["top"])
    return nil
}
```

---

## Runtime 1 — process (OS subprocess)

**Prerequisites**: NATS, `wlow start --control-plane` running, `python3` on PATH. No push, no Docker, no KVM.

**Start the processors** — `--cmd` auto-registers the manifest and starts consuming:

```sh
# Each command in its own terminal (or use & for background)
./bin/wlow start --id proc-tokenize --cmd "python3 tokenize.py"
./bin/wlow start --id proc-analyze  --cmd "python3 analyze.py"
```

**Run:**

```sh
go run ./examples/text-analysis --prefix proc --text "the quick brown fox"
```

**As a container** (script is in the image — no push needed):

```dockerfile
FROM python:3.12-slim
COPY tokenize.py /app/tokenize.py
RUN curl -fL -o /usr/local/bin/wlow \
      https://github.com/wlow/wlow/releases/latest/download/wlow-linux-amd64 \
    && chmod +x /usr/local/bin/wlow
ENV NATS_URL=nats://nats:4222
ENTRYPOINT ["wlow", "start", "--id", "proc-tokenize", "--cmd", "python3 /app/tokenize.py"]
```

```sh
docker build -t proc-tokenize:latest .
docker run -e NATS_URL=nats://your-nats:4222 proc-tokenize:latest
```

Scale by running more containers. They share the same NATS task queue.

---

## Runtime 2 — WASM component

**Prerequisites**: NATS, `wlow start --control-plane` running, Rust toolchain with `wasm32-wasip2` target.

No KVM needed.

**Compile:**

```sh
rustup target add wasm32-wasip2
cargo build --target wasm32-wasip2 --release
```

**Push:**

```sh
./bin/wlow push \
  --id wasm-tokenize --version v1 --runtime wasm \
  --source wasm --path target/wasm32-wasip2/release/tokenize.wasm \
  --tags latest

./bin/wlow push \
  --id wasm-analyze --version v1 --runtime wasm \
  --source wasm --path target/wasm32-wasip2/release/analyze.wasm \
  --tags latest
```

**Start:**

```sh
./bin/wlow start --runtimes wasm
```

**Run:**

```sh
go run ./examples/text-analysis --prefix wasm --text "the quick brown fox"
```

WASM components run in a Wasmtime sandbox. Memory-isolated, no host syscalls except through explicitly imported capabilities (`wlow:core/*`). The same `wlow start` command serves WASM tasks.

---

## Runtime 3 — cold microVM

**Prerequisites**: KVM host, Firecracker, OCI registry, BuildKit.

See [runner-setup.md](runner-setup.md) for KVM setup on GCP, AWS, or a Linux workstation.

**Dockerfiles** (`Dockerfile.tokenize`, `Dockerfile.analyze`):

```dockerfile
# Dockerfile.tokenize
FROM python:3.12-slim
WORKDIR /app
COPY tokenize.py .
CMD ["python3", "tokenize.py"]
```

**Push** (builds EROFS rootfs from Dockerfile):

```sh
export WLOW_REGISTRY=ghcr.io/your-org/wlow-artifacts
export BUILDKIT_HOST=tcp://127.0.0.1:1234

./bin/wlow push \
  --id cold-tokenize --version v1 --runtime microvm \
  --path Dockerfile.tokenize --entrypoint python3,/app/tokenize.py \
  --registry $WLOW_REGISTRY --tags latest

./bin/wlow push \
  --id cold-analyze --version v1 --runtime microvm \
  --path Dockerfile.analyze --entrypoint python3,/app/analyze.py \
  --registry $WLOW_REGISTRY --tags latest
```

**Start the wlow runner image** (on your KVM host — this is our image, not yours):

```sh
docker run \
  -e NATS_URL=nats://your-nats:4222 \
  -e WLOW_REGISTRY=$WLOW_REGISTRY \
  --device /dev/kvm:/dev/kvm \
  --privileged \
  -v /var/lib/wlow:/var/lib/wlow \
  ghcr.io/wlow/wlow-runtime:latest \
  /usr/local/bin/wlow-runner \
  --runtimes microvm
```

Or use the Kubernetes manifests in `deploy/k8s/`. See [install.md](install.md).

**Run:**

```sh
go run ./examples/text-analysis --prefix cold --text "the quick brown fox"
# each task boots a Firecracker VM (~2s), runs the script, shuts down
```

---

## Runtime 4 — snapshot microVM

Same processor images as cold, but VMs restore from a snapshot instead of booting. Snapshot prep is a one-time step per processor version.

**Prepare snapshots** (run from inside the runner, where KVM is available):

```sh
# From the wlow runner container / pod:
/usr/local/bin/wlow prepare-snapshot \
  --nats nats://your-nats:4222 \
  --id snap-tokenize --from cold-v1 --version snapshot-v1 \
  --snapshot-ref ghcr.io/your-org/wlow-snapshots/snap-tokenize:snapshot-v1 \
  --tags latest

/usr/local/bin/wlow prepare-snapshot \
  --nats nats://your-nats:4222 \
  --id snap-analyze --from cold-v1 --version snapshot-v1 \
  --snapshot-ref ghcr.io/your-org/wlow-snapshots/snap-analyze:snapshot-v1 \
  --tags latest
```

On Kubernetes:

```sh
kubectl -n wlow exec deploy/wlow-runner -- \
  /usr/local/bin/wlow prepare-snapshot \
    --nats nats://nats.nats:4222 \
    --id snap-tokenize --from cold-v1 --version snapshot-v1 \
    --snapshot-ref ghcr.io/your-org/wlow-snapshots/snap-tokenize:snapshot-v1 \
    --tags latest
```

**Start the runner** with snapshot runtimes:

```sh
docker run ... ghcr.io/wlow/wlow-runtime:latest \
  /usr/local/bin/wlow-runner \
  --runtimes snapshot
```

**Run:**

```sh
go run ./examples/text-analysis --prefix snap --text "the quick brown fox"
# restores from snapshot (~860ms per task)
```

---

## Comparing runtimes

Same workflow, same processors, different runtimes:

```
process:    ~50ms   (subprocess, no isolation)
wasm:       ~25ms   (memory isolated, no filesystem)
cold-vm:    ~2.1s   (full VM, cold boot each task)
snap-vm:    ~862ms  (full VM, restore from snapshot)
```

Choose based on your isolation and latency requirements. The workflow API and processor interface are identical across all four.

---

## Existing examples

The `examples/` directory has real runnable examples:

| Example | Runtime | What it shows |
|---------|---------|--------------|
| `examples/ffmpeg-process/` | process | Inline script push + submit |
| `examples/ffmpeg-cold-microvm/` | microvm | 2-task parallel DAG |
| `examples/ffmpeg-snapshot-microvm/` | snapshot | Snapshot prepare + restore cycle |
| `examples/wasm-component/` | wasm | WIT component push + submit |
