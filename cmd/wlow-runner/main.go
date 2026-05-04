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

// wlow-runner consumes sandboxed task work from NATS for one or more
// runtimes. Each runner advertises its NodeID + supported runtimes into the
// locality KV so the scheduler can place workloads near warm chunks.
//
// Typical deployments:
//   - linux-process only:   ./wlow-runner --runtimes process,wasm
//   - cold microvm runner:  ./wlow-runner --runtimes cold-microvm-rootfs --data-dir /var/lib/wlow
//   - mixed:                ./wlow-runner --runtimes process,cold-microvm-rootfs,snapshot-fork-microvm
func main() {
	natsURL := flag.String("nats", envOr("NATS_URL", "nats://localhost:4222"), "NATS URL")
	nodeID := flag.String("node-id", envOr("WLOW_NODE_ID", defaultNodeID()), "node identifier (heartbeats into locality KV)")
	runtimes := flag.String("runtimes", envOr("WLOW_RUNTIMES", "process"), "comma-separated runtimes this runner supports")
	dataDir := flag.String("data-dir", envOr("WLOW_DATA_DIR", "/var/lib/wlow"), "directory for rootfs/snapshot scratch")
	concurrency := flag.Int("concurrency", envOrInt("WLOW_CONCURRENCY", 4), "max in-flight tasks")
	heartbeat := flag.Duration("heartbeat", envOrDuration("WLOW_HEARTBEAT", 15*time.Second), "locality heartbeat interval")
	storeBucket := flag.String("store-bucket", envOr("STORE_BUCKET", "workflow-state"), "workflow store KV bucket")
	workflowSubjectPrefix := flag.String("workflow-subject-prefix", envOr("WORKFLOW_SUBJECT_PREFIX", workflow.DefaultSubjectPrefix), "workflow control subject prefix")
	processorStream := flag.String("processor-stream", envOr("PROCESSOR_STREAM", sandbox.DefaultStreamName), "processor work stream")
	processorSubjectPrefix := flag.String("processor-subject-prefix", envOr("PROCESSOR_SUBJECT_PREFIX", sandbox.DefaultSubjectPrefix), "processor work subject prefix")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	rts, err := parseRuntimes(*runtimes)
	if err != nil {
		log.Error("invalid --runtimes", "error", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Error("data dir not writable", "path", *dataDir, "error", err)
		os.Exit(1)
	}

	client, err := wlownats.NewClient(wlownats.Config{URL: *natsURL})
	if err != nil {
		log.Error("nats connect failed", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := wlownats.NewStore(ctx, client, wlownats.StoreConfig{Bucket: *storeBucket, TTL: 24 * time.Hour})
	if err != nil {
		log.Error("store init failed", "error", err)
		os.Exit(1)
	}

	artifactStore, err := artifact.NewStore(ctx, client.JetStream(), artifact.StoreConfig{})
	if err != nil {
		log.Error("artifact store init failed", "error", err)
		os.Exit(1)
	}

	outputCache, err := workflow.NewNATSOutputCache(ctx, client.JetStream(), 24*time.Hour)
	if err != nil {
		log.Error("output cache init failed", "error", err)
		os.Exit(1)
	}

	locality, err := workflow.NewLocalityScheduler(ctx, client.JetStream())
	if err != nil {
		log.Error("locality init failed", "error", err)
		os.Exit(1)
	}

	executors, err := sandbox.RegistryFor(*dataDir, rts...)
	if err != nil {
		log.Error("executor registry failed", "error", err)
		os.Exit(1)
	}

	runner, err := sandbox.NewRunner(sandbox.RunnerConfig{
		Client:                 client,
		Store:                  store,
		Artifacts:              artifactStore,
		Executors:              executors,
		OutputCache:            outputCache,
		Locality:               locality,
		NodeID:                 *nodeID,
		Runtimes:               rts,
		DataDir:                *dataDir,
		Concurrency:            *concurrency,
		StreamName:             *processorStream,
		ProcessorSubjectPrefix: *processorSubjectPrefix,
		HeartbeatInterval:      *heartbeat,
		WorkflowSubjectPrefix:  *workflowSubjectPrefix,
		Logger:                 log.With(slog.String("component", "runner"), slog.String("node", *nodeID)),
	})
	if err != nil {
		log.Error("runner init failed", "error", err)
		os.Exit(1)
	}

	consumeCtx, err := runner.Start(ctx)
	if err != nil {
		log.Error("runner start failed", "error", err)
		os.Exit(1)
	}
	defer consumeCtx.Stop()

	log.Info("wlow-runner started",
		"node", *nodeID,
		"runtimes", *runtimes,
		"data_dir", *dataDir,
		"concurrency", *concurrency,
	)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutting down")
}

func parseRuntimes(raw string) ([]artifact.Runtime, error) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 {
		return nil, errors.New("at least one runtime required")
	}
	const maxRuntimes = 16
	if len(parts) > maxRuntimes {
		return nil, fmt.Errorf("too many runtimes (max %d)", maxRuntimes)
	}
	out := make([]artifact.Runtime, 0, len(parts))
	for idx := 0; idx < len(parts); idx++ {
		name := strings.TrimSpace(parts[idx])
		if name == "" {
			continue
		}
		out = append(out, artifact.Runtime(name))
	}
	if len(out) == 0 {
		return nil, errors.New("at least one non-empty runtime required")
	}
	return out, nil
}

func defaultNodeID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "runner"
	}
	return host
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(v, "%d", &parsed); err != nil {
		return fallback
	}
	return parsed
}

func envOrDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
