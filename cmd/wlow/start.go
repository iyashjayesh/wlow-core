package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wlow/wlow/pkg/artifact"
	wlownats "github.com/wlow/wlow/pkg/nats"
	"github.com/wlow/wlow/pkg/sandbox"
	"github.com/wlow/wlow/pkg/workflow"
)

// startRunner is the handler for `wlow start [flags]`.
//
// With --control-plane: starts the wlow control plane (orchestrator).
// Without: connects to NATS and serves process/WASM processor tasks.
// MicroVM runtimes are not supported here — use the wlow runner image.
func startRunner(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	controlPlane := fs.Bool("control-plane", false, "start as control plane (orchestrator)")
	natsURL := fs.String("nats", envOr("NATS_URL", "nats://localhost:4222"), "NATS server URL")
	storeBucket := fs.String("store-bucket", envOr("STORE_BUCKET", "workflow-state"), "workflow state KV bucket")
	workflowPrefix := fs.String("workflow-subject-prefix", envOr("WORKFLOW_SUBJECT_PREFIX", workflow.DefaultSubjectPrefix), "workflow subject prefix")
	processorPrefix := fs.String("processor-subject-prefix", envOr("PROCESSOR_SUBJECT_PREFIX", sandbox.DefaultSubjectPrefix), "processor subject prefix")
	// Processor-runner-only flags (ignored when --control-plane is set)
	runtimesStr := fs.String("runtimes", envOr("WLOW_RUNTIMES", "process"), "runtimes: process, wasm")
	concurrency := fs.Int("concurrency", envOrInt("WLOW_CONCURRENCY", 4), "max in-flight tasks")
	processorStream := fs.String("processor-stream", envOr("PROCESSOR_STREAM", sandbox.DefaultStreamName), "processor work stream")
	processorID := fs.String("id", "", "processor ID to auto-register (use with --cmd)")
	processorCmd := fs.String("cmd", "", "command to run per task, e.g. \"python3 /app/processor.py\" (auto-registers manifest, no push required)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		slog.Info("shutting down")
		cancel()
	}()

	if *controlPlane {
		return startControlPlane(runCtx, controlPlaneFlags{
			natsURL:                *natsURL,
			storeBucket:            *storeBucket,
			workflowSubjectPrefix:  *workflowPrefix,
			processorSubjectPrefix: *processorPrefix,
		})
	}

	return runProcessorRunner(runCtx, processorRunnerConfig{
		natsURL:         *natsURL,
		storeBucket:     *storeBucket,
		workflowPrefix:  *workflowPrefix,
		processorPrefix: *processorPrefix,
		processorStream: *processorStream,
		runtimesStr:     *runtimesStr,
		concurrency:     *concurrency,
		processorID:     *processorID,
		processorCmd:    *processorCmd,
	})
}

type processorRunnerConfig struct {
	natsURL         string
	storeBucket     string
	workflowPrefix  string
	processorPrefix string
	processorStream string
	runtimesStr     string
	concurrency     int
	processorID     string
	processorCmd    string
}

func runProcessorRunner(ctx context.Context, cfg processorRunnerConfig) error {
	rts, err := parseStartRuntimes(cfg.runtimesStr)
	if err != nil {
		return err
	}

	natsCli, err := wlownats.NewClient(wlownats.Config{URL: cfg.natsURL})
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	defer natsCli.Close()

	store, err := wlownats.NewStore(ctx, natsCli, wlownats.StoreConfig{
		Bucket: cfg.storeBucket,
		TTL:    24 * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("init store: %w", err)
	}

	artStore, err := artifact.NewStore(ctx, natsCli.JetStream(), artifact.StoreConfig{})
	if err != nil {
		return fmt.Errorf("init artifact store: %w", err)
	}

	if cfg.processorID != "" && cfg.processorCmd != "" {
		if err := autoRegisterProcessor(ctx, artStore, cfg.processorID, cfg.processorCmd); err != nil {
			return fmt.Errorf("auto-register processor: %w", err)
		}
		slog.Info("processor registered", "id", cfg.processorID, "cmd", cfg.processorCmd)
	}

	outputCache, err := workflow.NewNATSOutputCache(ctx, natsCli.JetStream(), 24*time.Hour)
	if err != nil {
		return fmt.Errorf("init output cache: %w", err)
	}

	locality, err := workflow.NewLocalityScheduler(ctx, natsCli.JetStream())
	if err != nil {
		return fmt.Errorf("init locality: %w", err)
	}

	dataDir := envOr("WLOW_DATA_DIR", os.TempDir())
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("data dir: %w", err)
	}

	executors, err := sandbox.RegistryFor(dataDir, rts...)
	if err != nil {
		return fmt.Errorf("executor registry: %w", err)
	}

	nodeID, _ := os.Hostname()

	runner, err := sandbox.NewRunner(sandbox.RunnerConfig{
		Client:                 natsCli,
		Store:                  store,
		Artifacts:              artStore,
		Executors:              executors,
		OutputCache:            outputCache,
		Locality:               locality,
		NodeID:                 nodeID,
		Runtimes:               rts,
		DataDir:                dataDir,
		Concurrency:            cfg.concurrency,
		StreamName:             cfg.processorStream,
		ProcessorSubjectPrefix: cfg.processorPrefix,
		WorkflowSubjectPrefix:  cfg.workflowPrefix,
		HeartbeatInterval:      15 * time.Second,
		Logger:                 slog.Default(),
	})
	if err != nil {
		return fmt.Errorf("init runner: %w", err)
	}

	consumeCtx, err := runner.Start(ctx)
	if err != nil {
		return fmt.Errorf("start runner: %w", err)
	}
	defer consumeCtx.Stop()

	slog.Info("wlow processor started",
		"runtimes", cfg.runtimesStr,
		"nats", cfg.natsURL,
		"concurrency", cfg.concurrency,
		"node", nodeID,
	)

	<-ctx.Done()
	return nil
}

// autoRegisterProcessor writes a minimal process-runtime manifest to NATS KV
// so the orchestrator can route tasks to this processor without a prior push.
// No artifact bytes are stored — the command is the full entrypoint and lives
// entirely in runtime_config. The manifest is idempotent on re-run.
func autoRegisterProcessor(ctx context.Context, store *artifact.Store, processorID, cmd string) error {
	if processorID == "" || cmd == "" {
		return errors.New("processor id and cmd required")
	}
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return errors.New("cmd is empty")
	}
	command := parts[0]
	args := parts[1:]

	runtimeCfg := map[string]any{"cmd": command}
	if len(args) > 0 {
		runtimeCfg["args"] = args
	}

	m := &artifact.Manifest{
		Kind:          artifact.ManifestKind,
		Tenant:        artifact.DefaultTenant,
		ProcessorID:   processorID,
		Version:       "v1",
		Runtime:       artifact.RuntimeProcess,
		IOProtocol:    artifact.IOProtocolJSONStdio,
		RuntimeConfig: runtimeCfg,
		HashAlgorithm: artifact.HashAlgorithmOCI,
		ArtifactHash:  "direct-command",
		ArtifactSize:  0,
		Artifacts:     map[string]artifact.Artifact{},
		CreatedAt:     time.Now().UTC(),
	}
	return store.PutArtifact(ctx, m, "latest")
}

func parseStartRuntimes(raw string) ([]artifact.Runtime, error) {
	if raw == "" {
		return nil, errors.New("at least one runtime required")
	}
	parts := strings.Split(raw, ",")
	const maxRuntimes = 4
	rts := make([]artifact.Runtime, 0, len(parts))
	for idx := 0; idx < len(parts) && idx < maxRuntimes; idx++ {
		r := strings.TrimSpace(parts[idx])
		if r == "" {
			continue
		}
		rt := artifact.Runtime(r)
		if rt != artifact.RuntimeProcess && rt != artifact.RuntimeWasm {
			return nil, fmt.Errorf("runtime %q not supported by wlow start — microVM runtimes require the wlow runner image", r)
		}
		rts = append(rts, rt)
	}
	if len(rts) == 0 {
		return nil, errors.New("no valid runtimes specified")
	}
	return rts, nil
}

func envOrInt(key string, fallback int) int {
	v := envOr(key, "")
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return fallback
	}
	return n
}
