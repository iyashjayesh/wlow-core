package build

import (
	"context"
	"errors"

	"github.com/wlow/wlow/pkg/artifact"
)

type SourceKind string

const (
	SourceWasm       SourceKind = "wasm"
	SourceDockerfile SourceKind = "dockerfile"
	SourceTarball    SourceKind = "tarball"
	SourceBinary     SourceKind = "binary"
)

type Options struct {
	Kind          SourceKind
	Path          string
	Runtime       artifact.Runtime
	ProcessorID   string
	Version       string
	Tags          []string
	Entrypoint    []string
	WorkDir       string
	Env           map[string]string
	Deterministic bool
	BuildSecrets  []string
	Platform      string
	OutputPath    string
	ImageRef      string
}

type Spec struct {
	Data     []byte
	Path     string
	Manifest artifact.Manifest
	Tags     []string
}

type Adapter interface {
	Build(ctx context.Context, opts Options) (*Spec, error)
}

func Build(ctx context.Context, opts Options) (*Spec, error) {
	if opts.ProcessorID == "" || opts.Version == "" {
		return nil, errors.New("processor id and version required")
	}
	switch opts.Kind {
	case SourceWasm:
		return WasmAdapter{}.Build(ctx, opts)
	case SourceDockerfile:
		return DockerfileAdapter{}.Build(ctx, opts)
	case SourceTarball:
		return TarballAdapter{}.Build(ctx, opts)
	case SourceBinary:
		return BinaryAdapter{}.Build(ctx, opts)
	default:
		return nil, errors.New("unsupported build source")
	}
}
