package artifact

import (
	"errors"
	"fmt"
	"strings"
)

const (
	SnapshotMemoryIndexVersionV1 = "v1"
	SnapshotMemoryPageSize       = int64(4 * 1024)
	DefaultSnapshotChunkSize     = int64(2 * 1024 * 1024)
	MaxSnapshotChunkSize         = int64(16 * 1024 * 1024)
	MaxSnapshotChunks            = 1 << 20
	MaxSnapshotMemoryBytes       = int64(1 << 40)
)

type SnapshotMemoryIndex struct {
	Version     string                `json:"version"`
	ChunkSize   int64                 `json:"chunk_size"`
	MemoryBytes int64                 `json:"memory_bytes"`
	Chunks      []SnapshotMemoryChunk `json:"chunks"`
	Layout      string                `json:"layout,omitempty"`
}

type SnapshotMemoryChunk struct {
	Offset           int64  `json:"offset"`
	Ref              string `json:"ref"`
	Digest           string `json:"digest"`
	CompressedSize   int64  `json:"compressed_size"`
	UncompressedSize int64  `json:"uncompressed_size"`
	Compression      string `json:"compression"`
	SHA256           string `json:"sha256"`
	BLAKE3           string `json:"blake3"`
}

func (idx *SnapshotMemoryIndex) Validate() error {
	if idx == nil {
		return errors.New("snapshot memory index required")
	}
	if idx.Version != SnapshotMemoryIndexVersionV1 {
		return fmt.Errorf("unsupported snapshot memory index version: %s", idx.Version)
	}
	if idx.ChunkSize <= 0 || idx.ChunkSize > MaxSnapshotChunkSize {
		return fmt.Errorf("snapshot chunk size out of bounds: %d", idx.ChunkSize)
	}
	if idx.ChunkSize%SnapshotMemoryPageSize != 0 {
		return fmt.Errorf("snapshot chunk size must align to %d", SnapshotMemoryPageSize)
	}
	if idx.MemoryBytes <= 0 || idx.MemoryBytes > MaxSnapshotMemoryBytes {
		return fmt.Errorf("snapshot memory bytes out of bounds: %d", idx.MemoryBytes)
	}
	if len(idx.Chunks) == 0 || len(idx.Chunks) > MaxSnapshotChunks {
		return fmt.Errorf("snapshot chunk count out of bounds: %d", len(idx.Chunks))
	}
	for i := 0; i < len(idx.Chunks); i++ {
		chunk := idx.Chunks[i]
		expectedOffset := int64(i) * idx.ChunkSize
		if chunk.Offset != expectedOffset {
			return fmt.Errorf("snapshot chunk offset mismatch at %d: got %d want %d", i, chunk.Offset, expectedOffset)
		}
		if chunk.Ref == "" {
			return fmt.Errorf("snapshot chunk ref missing at %d", i)
		}
		if !strings.HasPrefix(chunk.Digest, "sha256:") {
			return fmt.Errorf("snapshot chunk digest must be sha256 at %d", i)
		}
		if chunk.CompressedSize <= 0 {
			return fmt.Errorf("snapshot chunk compressed size invalid at %d", i)
		}
		if chunk.UncompressedSize <= 0 || chunk.UncompressedSize > idx.ChunkSize {
			return fmt.Errorf("snapshot chunk uncompressed size invalid at %d", i)
		}
		if chunk.Compression != "gzip" {
			return fmt.Errorf("snapshot chunk compression unsupported at %d: %s", i, chunk.Compression)
		}
		if len(chunk.SHA256) != 64 {
			return fmt.Errorf("snapshot chunk sha256 invalid at %d", i)
		}
		if len(chunk.BLAKE3) != 64 {
			return fmt.Errorf("snapshot chunk blake3 invalid at %d", i)
		}
	}
	last := idx.Chunks[len(idx.Chunks)-1]
	if last.Offset+last.UncompressedSize != idx.MemoryBytes {
		return fmt.Errorf("snapshot memory bytes mismatch: chunks end at %d but memory bytes=%d", last.Offset+last.UncompressedSize, idx.MemoryBytes)
	}
	return nil
}
