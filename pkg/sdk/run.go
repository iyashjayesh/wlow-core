package sdk

import (
	"context"
	"log/slog"
	"os"
)

// Run starts a processor with minimal configuration.
//
//	sdk.Run("echo", "PROCESSOR.echo", func(ctx context.Context, in MyInput) (MyOutput, error) {
//	    return MyOutput{Result: in.Text}, nil
//	})
func Run[In, Out any](id, subject string, fn Func[In, Out]) error {
	return RunWithConfig(RunConfig{
		ID:       id,
		Subjects: []string{subject},
	}, fn)
}

// RunConfig provides optional configuration.
type RunConfig struct {
	ID          string
	Subjects    []string
	NATSUrl     string // default: NATS_URL env or nats://localhost:4222
	Concurrency int    // default: 2
	Logger      *slog.Logger
}

// RunWithConfig starts a processor with explicit configuration.
func RunWithConfig[In, Out any](cfg RunConfig, p Processor[In, Out]) error {
	if cfg.NATSUrl == "" {
		cfg.NATSUrl = os.Getenv("NATS_URL")
		if cfg.NATSUrl == "" {
			cfg.NATSUrl = "nats://localhost:4222"
		}
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	runner, err := NewRunnerFor(RunnerConfig{
		NATSUrl:     cfg.NATSUrl,
		ProcessorID: cfg.ID,
		Subjects:    cfg.Subjects,
		Concurrency: cfg.Concurrency,
		Logger:      cfg.Logger,
	}, p)
	if err != nil {
		return err
	}

	return runner.Run(context.Background())
}

// RunHandler runs a pre-wrapped Handler (for external/dynamic processors).
func RunHandler(id, subject string, h Handler) error {
	return RunHandlerWithConfig(RunConfig{
		ID:       id,
		Subjects: []string{subject},
	}, h)
}

// RunHandlerWithConfig runs a Handler with explicit configuration.
func RunHandlerWithConfig(cfg RunConfig, h Handler) error {
	if cfg.NATSUrl == "" {
		cfg.NATSUrl = os.Getenv("NATS_URL")
		if cfg.NATSUrl == "" {
			cfg.NATSUrl = "nats://localhost:4222"
		}
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	runner, err := NewRunner(RunnerConfig{
		NATSUrl:     cfg.NATSUrl,
		ProcessorID: cfg.ID,
		Subjects:    cfg.Subjects,
		Concurrency: cfg.Concurrency,
		Logger:      cfg.Logger,
	}, h)
	if err != nil {
		return err
	}

	return runner.Run(context.Background())
}
