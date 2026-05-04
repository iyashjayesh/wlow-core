package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// ExecProcessor runs an external executable as a processor.
// Input is passed as JSON via stdin, output expected as JSON on stdout.
// Implements Processor[Dynamic, Dynamic].
type ExecProcessor struct {
	Cmd     string
	Args    []string
	Env     []string
	Dir     string
	Timeout time.Duration
}

func NewExecProcessor(cmd string, args ...string) *ExecProcessor {
	return &ExecProcessor{Cmd: cmd, Args: args, Timeout: 5 * time.Minute}
}

func (p *ExecProcessor) WithTimeout(d time.Duration) *ExecProcessor { p.Timeout = d; return p }
func (p *ExecProcessor) WithDir(d string) *ExecProcessor            { p.Dir = d; return p }
func (p *ExecProcessor) WithEnv(e ...string) *ExecProcessor         { p.Env = append(p.Env, e...); return p }

func (p *ExecProcessor) Process(ctx context.Context, in Dynamic) (Dynamic, error) {
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}

	input, _ := json.Marshal(in)
	cmd := exec.CommandContext(ctx, p.Cmd, p.Args...)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = append(os.Environ(), p.Env...)
	if p.Dir != "" {
		cmd.Dir = p.Dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("timeout after %v", p.Timeout)
		}
		if ctx.Err() == context.Canceled {
			return nil, fmt.Errorf("cancelled")
		}
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}

	var out Dynamic
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("invalid output JSON: %w", err)
	}
	return out, nil
}

// Script processors

func NewPythonProcessor(path string, args ...string) *ExecProcessor {
	return NewExecProcessor("python3", append([]string{path}, args...)...)
}

func NewNodeProcessor(path string, args ...string) *ExecProcessor {
	return NewExecProcessor("node", append([]string{path}, args...)...)
}

func NewBashProcessor(path string, args ...string) *ExecProcessor {
	return NewExecProcessor("bash", append([]string{path}, args...)...)
}

// HTTPProcessor calls an HTTP endpoint as a processor.
// Implements Processor[Dynamic, Dynamic].
type HTTPProcessor struct {
	URL     string
	Headers map[string]string
	Timeout time.Duration
}

func NewHTTPProcessor(url string) *HTTPProcessor {
	return &HTTPProcessor{URL: url, Headers: make(map[string]string), Timeout: 30 * time.Second}
}

func (p *HTTPProcessor) WithHeader(k, v string) *HTTPProcessor      { p.Headers[k] = v; return p }
func (p *HTTPProcessor) WithTimeout(d time.Duration) *HTTPProcessor { p.Timeout = d; return p }

func (p *HTTPProcessor) Process(ctx context.Context, in Dynamic) (Dynamic, error) {
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}

	body, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, "POST", p.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}

	var out Dynamic
	return out, json.NewDecoder(resp.Body).Decode(&out)
}
