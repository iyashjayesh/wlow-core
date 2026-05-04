package artifact

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/zeebo/blake3"
)

type SnapshotFiles struct {
	Config string
	State  string
	Memory string
	Rootfs string
}

const (
	SnapshotConfigFile = "vm-config.json"
	SnapshotStateFile  = "vm-state.bin"
	SnapshotMemoryFile = "vm-memory.bin"
	SnapshotRootfsFile = "rootfs.img"
)

func (s *Store) PutSnapshotArtifact(ctx context.Context, source *Manifest, version string, imageRef string, files SnapshotFiles, tags ...string) (*Manifest, error) {
	if s == nil {
		return nil, errors.New("artifact store required")
	}
	if source == nil {
		return nil, errors.New("source manifest required")
	}
	if version == "" {
		return nil, errors.New("snapshot version required")
	}
	if imageRef == "" {
		return nil, errors.New("snapshot image ref required")
	}
	objects, hash, size, err := PushSnapshotImage(ctx, imageRef, files)
	if err != nil {
		return nil, err
	}
	manifest := snapshotManifest(source, version, hash, size, objects)
	if err := s.PutArtifact(ctx, manifest, tags...); err != nil {
		return nil, err
	}
	return manifest, nil
}

func snapshotManifest(source *Manifest, version string, hash string, size int64, objects SnapshotObjects) *Manifest {
	cfg := copyRuntimeConfig(source.RuntimeConfig)
	cfg["snapshot_source_version"] = source.Version
	artifacts := snapshotArtifacts(objects)
	copySnapshotRuntimeArtifacts(artifacts, source.Artifacts)
	return &Manifest{
		Kind:          ManifestKind,
		Tenant:        source.Tenant,
		ProcessorID:   source.ProcessorID,
		Version:       version,
		Runtime:       RuntimeSnapshot,
		IOProtocol:    IOProtocolJSONVsockStream,
		RuntimeConfig: cfg,
		HashAlgorithm: HashAlgorithm,
		ArtifactHash:  hash,
		ArtifactSize:  size,
		Artifacts:     artifacts,
		Capabilities:  source.Capabilities,
		Deterministic: source.Deterministic,
		ResourceHints: source.ResourceHints,
		BuildProvenance: map[string]string{
			"source_runtime": string(source.RuntimeValue()),
		},
		Cache:     source.Cache,
		CreatedAt: time.Now().UTC(),
	}
}

func copySnapshotRuntimeArtifacts(dst map[string]Artifact, src map[string]Artifact) {
	for role, item := range src {
		if role == RoleOCIIndex {
			dst[role] = item
		}
	}
}

func snapshotArtifacts(objects SnapshotObjects) map[string]Artifact {
	return map[string]Artifact{
		RoleSnapshotConfig: {Kind: KindRemoteObject, Remote: objects.Config},
		RoleSnapshotState:  {Kind: KindRemoteObject, Remote: objects.State},
		RoleSnapshotMemory: {Kind: KindRemoteObject, Remote: objects.Memory},
		RoleSnapshotRootfs: {Kind: KindRemoteObject, Remote: objects.Rootfs},
	}
}

func SnapshotObjectFiles(objects SnapshotObjects) map[string]*RemoteRef {
	return map[string]*RemoteRef{
		SnapshotConfigFile: objects.Config,
		SnapshotStateFile:  objects.State,
		SnapshotMemoryFile: objects.Memory,
		SnapshotRootfsFile: objects.Rootfs,
	}
}

func assignSnapshotObject(objects *SnapshotObjects, role string, ref *RemoteRef) {
	switch role {
	case RoleSnapshotConfig:
		objects.Config = ref
	case RoleSnapshotState:
		objects.State = ref
	case RoleSnapshotMemory:
		objects.Memory = ref
	case RoleSnapshotRootfs:
		objects.Rootfs = ref
	}
}

func copyRuntimeConfig(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+1)
	for key, value := range in {
		out[key] = value
	}
	return out
}

func objectRoleName(role string, sum string) string {
	return strings.ReplaceAll(role, ".", "-") + "-" + sum
}

func hashSnapshotFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	h := blake3.New()
	n, err := io.Copy(h, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
