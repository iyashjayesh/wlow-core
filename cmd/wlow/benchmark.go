package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const maxBenchmarkSteps = 4

type benchmarkConfig struct {
	PushCmd      string
	PrepareCmd   string
	RunCmd       string
	E2ECmd       string
	PushLimit    time.Duration
	PrepareLimit time.Duration
	RunLimit     time.Duration
	E2ELimit     time.Duration
}

type benchmarkStep struct {
	Name  string
	Cmd   string
	Limit time.Duration
}

func benchmark(ctx context.Context, args []string) error {
	cfg, err := parseBenchmarkConfig(args)
	if err != nil {
		return err
	}
	steps := benchmarkSteps(cfg)
	if len(steps) == 0 {
		return errors.New("at least one benchmark command is required")
	}
	for idx := 0; idx < len(steps) && idx < maxBenchmarkSteps; idx++ {
		if err := runBenchmarkStep(ctx, steps[idx]); err != nil {
			return err
		}
	}
	return nil
}

func parseBenchmarkConfig(args []string) (*benchmarkConfig, error) {
	fs := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	pushCmd := fs.String("push-cmd", "", "command that builds and pushes a processor")
	prepareCmd := fs.String("prepare-cmd", "", "command that prepares a snapshot")
	runCmd := fs.String("run-cmd", "", "command that runs a snapshot-backed task")
	e2eCmd := fs.String("e2e-cmd", "", "command that runs the full workflow")
	pushLimit := fs.Duration("push-limit", 10*time.Second, "max allowed push duration")
	prepareLimit := fs.Duration("prepare-limit", 5*time.Second, "max allowed snapshot prepare duration")
	runLimit := fs.Duration("run-limit", time.Second, "max allowed steady-state run duration")
	e2eLimit := fs.Duration("e2e-limit", time.Second, "max allowed steady-state workflow duration")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return &benchmarkConfig{
		PushCmd:      *pushCmd,
		PrepareCmd:   *prepareCmd,
		RunCmd:       *runCmd,
		E2ECmd:       *e2eCmd,
		PushLimit:    *pushLimit,
		PrepareLimit: *prepareLimit,
		RunLimit:     *runLimit,
		E2ELimit:     *e2eLimit,
	}, nil
}

func benchmarkSteps(cfg *benchmarkConfig) []benchmarkStep {
	steps := make([]benchmarkStep, 0, maxBenchmarkSteps)
	if cfg.PushCmd != "" {
		steps = append(steps, benchmarkStep{Name: "push", Cmd: cfg.PushCmd, Limit: cfg.PushLimit})
	}
	if cfg.PrepareCmd != "" {
		steps = append(steps, benchmarkStep{Name: "prepare-snapshot", Cmd: cfg.PrepareCmd, Limit: cfg.PrepareLimit})
	}
	if cfg.RunCmd != "" {
		steps = append(steps, benchmarkStep{Name: "run", Cmd: cfg.RunCmd, Limit: cfg.RunLimit})
	}
	if cfg.E2ECmd != "" {
		steps = append(steps, benchmarkStep{Name: "e2e", Cmd: cfg.E2ECmd, Limit: cfg.E2ELimit})
	}
	return steps
}

func runBenchmarkStep(ctx context.Context, step benchmarkStep) error {
	if step.Cmd == "" {
		return errors.New("benchmark command required")
	}
	if step.Limit <= 0 {
		return errors.New("benchmark limit must be positive")
	}
	start := time.Now()
	cmd := exec.CommandContext(ctx, "sh", "-c", step.Cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", step.Name, err)
	}
	elapsed := time.Since(start)
	fmt.Printf("%s %s limit=%s\n", step.Name, elapsed.Round(time.Millisecond), step.Limit)
	if elapsed > step.Limit {
		return fmt.Errorf("%s exceeded limit: %s > %s", step.Name, elapsed, step.Limit)
	}
	return nil
}
