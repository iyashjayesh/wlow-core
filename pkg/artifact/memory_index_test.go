package artifact

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
)

func TestSnapshotMemoryIndexValidate(t *testing.T) {
	t.Parallel()
	idx := &SnapshotMemoryIndex{
		Version:     SnapshotMemoryIndexVersionV1,
		ChunkSize:   4096,
		MemoryBytes: 6 * 1024,
		Chunks: []SnapshotMemoryChunk{
			{
				Offset:           0,
				Ref:              "registry.example.com/demo/repo@sha256:1111111111111111111111111111111111111111111111111111111111111111",
				Digest:           "sha256:1111111111111111111111111111111111111111111111111111111111111111",
				CompressedSize:   4096,
				UncompressedSize: 4096,
				Compression:      "gzip",
				SHA256:           "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				BLAKE3:           "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			},
			{
				Offset:           4096,
				Ref:              "registry.example.com/demo/repo@sha256:2222222222222222222222222222222222222222222222222222222222222222",
				Digest:           "sha256:2222222222222222222222222222222222222222222222222222222222222222",
				CompressedSize:   2048,
				UncompressedSize: 2048,
				Compression:      "gzip",
				SHA256:           "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				BLAKE3:           "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
			},
		},
	}
	if err := idx.Validate(); err != nil {
		t.Fatalf("validate index: %v", err)
	}
	idx.Chunks[1].Offset = 5000
	if err := idx.Validate(); err == nil {
		t.Fatal("expected offset validation failure")
	}
}

func TestSnapshotMemoryLayers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	memoryPath := filepath.Join(dir, "vm-memory.bin")
	payload := bytes.Repeat([]byte{0x5A}, int(DefaultSnapshotChunkSize)+1234)
	if err := os.WriteFile(memoryPath, payload, 0o644); err != nil {
		t.Fatalf("write memory file: %v", err)
	}
	repo, err := name.NewRepository("registry.example.com/demo/snapshot")
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	layers, indexRef, total, err := snapshotMemoryLayers(repo, memoryPath)
	if err != nil {
		t.Fatalf("snapshot layers: %v", err)
	}
	if len(layers) != 3 {
		t.Fatalf("unexpected layer count: got %d want 3", len(layers))
	}
	if indexRef == nil || indexRef.Digest == "" {
		t.Fatal("index ref missing digest")
	}
	if total <= 0 {
		t.Fatalf("chunk compressed total invalid: %d", total)
	}
}
