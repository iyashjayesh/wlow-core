package build

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wlow/wlow/pkg/artifact"
)

type WasmAdapter struct{}

func (WasmAdapter) Build(_ context.Context, opts Options) (*Spec, error) {
	data, err := os.ReadFile(opts.Path)
	if err != nil {
		return nil, err
	}
	world := "wlow:core/deterministic-processor@0.1.0"
	if !opts.Deterministic {
		world = "wlow:core/effectful-processor@0.1.0"
	}
	return &Spec{Data: data, Tags: opts.Tags, Manifest: artifact.Manifest{
		Runtime:       artifact.RuntimeWasm,
		IOProtocol:    artifact.IOProtocolComponentWlowCore,
		WITWorld:      world,
		Deterministic: opts.Deterministic,
		Artifacts: map[string]artifact.Artifact{
			"component": {Kind: artifact.KindCompiledWasm},
		},
	}}, nil
}

type BinaryAdapter struct{}

func (BinaryAdapter) Build(_ context.Context, opts Options) (*Spec, error) {
	data, err := os.ReadFile(opts.Path)
	if err != nil {
		return nil, err
	}
	return &Spec{Data: data, Tags: opts.Tags, Manifest: artifact.Manifest{
		Runtime:    artifact.RuntimeProcess,
		IOProtocol: artifact.IOProtocolJSONStdio,
		RuntimeConfig: map[string]any{
			"cmd": "{artifact}",
		},
		Artifacts: map[string]artifact.Artifact{
			"program": {Kind: artifact.KindBlob},
		},
	}}, nil
}

type TarballAdapter struct{}

func (TarballAdapter) Build(_ context.Context, opts Options) (*Spec, error) {
	data, err := os.ReadFile(opts.Path)
	if err != nil {
		return nil, err
	}
	return microVMSpec(data, opts, map[string]string{"source": "tarball"}), nil
}

type DockerfileAdapter struct{}

func (DockerfileAdapter) Build(ctx context.Context, opts Options) (*Spec, error) {
	if opts.Path == "" {
		return nil, errors.New("dockerfile path required")
	}
	tarPath := opts.OutputPath
	if tarPath == "" {
		dir, err := os.MkdirTemp("", "wlow-build-*")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		tarPath = filepath.Join(dir, "rootfs.tar")
	}
	if err := runBuildctl(ctx, opts, tarPath); err != nil {
		return nil, err
	}
	if opts.OutputPath != "" {
		return microVMSpecFile(tarPath, opts, map[string]string{"source": "dockerfile"}), nil
	}
	data, err := os.ReadFile(tarPath)
	if err != nil {
		return nil, err
	}
	return microVMSpec(data, opts, map[string]string{"source": "dockerfile"}), nil
}

func runBuildctl(ctx context.Context, opts Options, dest string) error {
	contextDir := filepath.Dir(opts.Path)
	args := []string{
		"build",
		"--frontend", "dockerfile.v0",
		"--local", "context=" + contextDir,
		"--local", "dockerfile=" + contextDir,
		"--output", "type=tar,dest=" + dest,
	}
	if opts.Platform != "" {
		args = append(args, "--opt", "platform="+opts.Platform)
	}
	for _, secret := range opts.BuildSecrets {
		args = append(args, "--secret", secret)
	}
	cmd := buildctlCommand(ctx, args)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runBuildctlImage(ctx context.Context, opts Options, imageRef string) error {
	contextDir := filepath.Dir(opts.Path)
	args := []string{
		"build",
		"--frontend", "dockerfile.v0",
		"--local", "context=" + contextDir,
		"--local", "dockerfile=" + contextDir,
		"--output", "type=image,name=" + imageRef + ",push=true,oci-mediatypes=true",
	}
	if opts.Platform != "" {
		args = append(args, "--opt", "platform="+opts.Platform)
	}
	for _, secret := range opts.BuildSecrets {
		args = append(args, "--secret", secret)
	}
	cmd := buildctlCommand(ctx, args)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildctlCommand(ctx context.Context, args []string) *exec.Cmd {
	if addr := os.Getenv("BUILDKIT_HOST"); addr != "" {
		args = append([]string{"--addr", addr}, args...)
	}
	return exec.CommandContext(ctx, "buildctl", args...)
}

func extractOCIArchive(path, dest string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	tr := tar.NewReader(file)
	const maxEntries = 1 << 16
	for count := 0; count < maxEntries; count++ {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := writeOCIEntry(dest, hdr, tr); err != nil {
			return err
		}
	}
	return errors.New("OCI archive entry limit exceeded")
}

func writeOCIEntry(dest string, hdr *tar.Header, tr *tar.Reader) error {
	target, err := safeArchivePath(dest, hdr.Name)
	if err != nil {
		return err
	}
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, os.FileMode(hdr.Mode)&0o777)
	case tar.TypeReg, tar.TypeRegA:
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return writeOCIFile(target, hdr, tr)
	default:
		return nil
	}
}

func writeOCIFile(target string, hdr *tar.Header, tr *tar.Reader) error {
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, tr)
	return err
}

func safeArchivePath(base, rel string) (string, error) {
	clean := filepath.Clean("/" + rel)
	joined := filepath.Join(base, clean)
	out, err := filepath.Rel(base, joined)
	if err != nil {
		return "", err
	}
	if out == ".." || strings.HasPrefix(out, "../") {
		return "", fmt.Errorf("archive entry escapes output: %s", rel)
	}
	return joined, nil
}

func microVMSpec(data []byte, opts Options, provenance map[string]string) *Spec {
	spec := microVMSpecFile("", opts, provenance)
	spec.Data = data
	return spec
}

func microVMSpecFile(path string, opts Options, provenance map[string]string) *Spec {
	runtime := opts.Runtime
	if runtime == "" {
		runtime = artifact.RuntimeMicroVM
	}
	return &Spec{Path: path, Tags: opts.Tags, Manifest: artifact.Manifest{
		Runtime:         runtime,
		IOProtocol:      artifact.IOProtocolJSONVsock,
		RuntimeConfig:   rootfsConfig(opts),
		BuildProvenance: buildProvenance(opts, provenance),
		Artifacts: map[string]artifact.Artifact{
			"rootfs": {Kind: artifact.KindBlob},
		},
	}}
}

func buildProvenance(opts Options, base map[string]string) map[string]string {
	out := make(map[string]string, len(base)+1)
	for key, value := range base {
		out[key] = value
	}
	if opts.Platform != "" {
		out["platform"] = opts.Platform
	}
	return out
}

func rootfsConfig(opts Options) map[string]any {
	cfg := map[string]any{}
	if len(opts.Entrypoint) > 0 {
		cfg["entrypoint"] = opts.Entrypoint
	}
	if opts.WorkDir != "" {
		cfg["workdir"] = opts.WorkDir
	}
	if len(opts.Env) > 0 {
		cfg["env"] = opts.Env
	}
	return cfg
}
