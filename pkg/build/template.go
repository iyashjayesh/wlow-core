package build

import (
	"context"
	"errors"
	"time"

	"github.com/wlow/wlow/pkg/artifact"
)

type WarmTemplateConfig struct {
	RootfsHash     string
	Warmup         []string
	ReadyText      string
	Timeout        time.Duration
	BeforeSnapshot []string
	AfterRestore   []string
}

func BuildWarmTemplate(ctx context.Context, _ any, dir string, cfg WarmTemplateConfig) (*artifact.Manifest, error) {
	if ctx == nil || dir == "" || cfg.RootfsHash == "" {
		return nil, errors.New("context, dir and rootfs hash required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	return nil, errors.New("go cloud hypervisor warm template build removed; use runner-rs")
}

func snapshotManifest(dir string, cfg WarmTemplateConfig) *artifact.Manifest {
	linked := []artifact.CompositeRef{
		{Role: "rootfs", Hash: cfg.RootfsHash, Kind: artifact.KindBlob},
		{Role: artifact.RoleSnapshotMemory, Hash: artifact.RoleSnapshotMemory, Kind: artifact.KindMemorySnapshot},
		{Role: artifact.RoleSnapshotState, Hash: artifact.RoleSnapshotState, Kind: artifact.KindVMState},
		{Role: artifact.RoleSnapshotConfig, Hash: artifact.RoleSnapshotConfig, Kind: artifact.KindVMConfig},
	}
	return &artifact.Manifest{
		Runtime:    artifact.RuntimeSnapshot,
		IOProtocol: artifact.IOProtocolJSONVsockStream,
		Artifacts: map[string]artifact.Artifact{
			"snapshot": {Kind: artifact.KindComposite, Linked: linked},
		},
		RuntimeConfig: map[string]any{
			"after_restore": cfg.AfterRestore,
		},
	}
}
