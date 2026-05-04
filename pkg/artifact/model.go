package artifact

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

)

const (
	HashAlgorithm    = "blake3-keyed"
	HashAlgorithmOCI = "oci-sha256"
	ManifestKind     = "wlow.processor.manifest.v1"

	DefaultTenant       = "default"
	DefaultChunkTarget  = 64 * 1024
	DefaultChunkMin     = 16 * 1024
	DefaultChunkMax     = 256 * 1024
	DefaultMaxChunkSize = DefaultChunkMax

	BlobBucket      = "wlow-artifact-blobs"    // inline binary artifacts (process scripts, WASM)
	ChunkBucket     = "wlow-artifact-chunks"   // legacy; unused, kept for migration
	ManifestBucket  = "wlow-artifact-manifests"
	RefBucket       = "wlow-artifact-refs"
	TenantKeyBucket = "wlow-tenant-keys"
	QuotaBucket     = "wlow-tenant-quotas"
)

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._=-]*$`)

type ExecutionMode string

const (
	ExecutionAttached  ExecutionMode = "attached"
	ExecutionSandboxed ExecutionMode = "sandboxed"
)

type Runtime string

const (
	RuntimeProcess  Runtime = "process"
	RuntimeWasm     Runtime = "wasm"
	RuntimeMicroVM  Runtime = "microvm"
	RuntimeSnapshot Runtime = "snapshot"
)

type IOProtocol string

const (
	IOProtocolExternRefJSON     IOProtocol = "externref-json-v0"
	IOProtocolJSONStdio         IOProtocol = "json-stdin-stdout-v0"
	IOProtocolJSONVsock         IOProtocol = "json-vsock-v0"
	IOProtocolJSONVsockStream   IOProtocol = "json-vsock-stream-v0"
	IOProtocolComponentWlowCore IOProtocol = "component-wlow-core-v0"
)

type Capability string

const (
	CapabilityContext   Capability = "wlow:core/context"
	CapabilityLog       Capability = "wlow:core/log"
	CapabilityHeartbeat Capability = "wlow:core/heartbeat"
	CapabilityState     Capability = "wlow:core/state"
	CapabilityHTTP      Capability = "wlow:net/http"
	CapabilityBlob      Capability = "wlow:storage/blob"
	CapabilityMCP       Capability = "wlow:mcp/client"
)

type ArtifactKind string

const (
	KindBlob           ArtifactKind = "blob"
	KindObject         ArtifactKind = "object"
	KindFileTree       ArtifactKind = "file-tree"
	KindMemorySnapshot ArtifactKind = "memory-snapshot"
	KindVMState        ArtifactKind = "vm-state"
	KindVMConfig       ArtifactKind = "vm-config"
	KindCompiledWasm   ArtifactKind = "compiled-wasm"
	KindComposite      ArtifactKind = "composite"
	KindOCIDescriptor  ArtifactKind = "oci-descriptor"
	KindRemoteObject   ArtifactKind = "remote-object"
)

const (
	RoleOCIIndex        = "oci.index"
	RoleRootfs          = "rootfs"
	RoleSnapshotConfig  = "snapshot.config"
	RoleSnapshotState   = "snapshot.state"
	RoleSnapshotMemory  = "snapshot.memory"
	RoleSnapshotRootfs  = "snapshot.rootfs"
	RoleSnapshotRuntime = "snapshot.runtime"
)

type ChunkRef struct {
	Hash string `json:"hash"`
	Size int    `json:"size"`
}

type BlobRef struct {
	Size   int64      `json:"size"`
	Chunks []ChunkRef `json:"chunks"`
}

type ObjectRef struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest,omitempty"`
}

type RemoteRef struct {
	Ref       string `json:"ref,omitempty"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	MediaType string `json:"media_type,omitempty"`
}

type OCIDescriptor struct {
	Digest      string            `json:"digest"`
	MediaType   string            `json:"media_type"`
	Size        int64             `json:"size"`
	Object      *ObjectRef        `json:"object,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type FileNode struct {
	Path   string     `json:"path"`
	Mode   uint32     `json:"mode"`
	UID    uint32     `json:"uid,omitempty"`
	GID    uint32     `json:"gid,omitempty"`
	Size   int64      `json:"size,omitempty"`
	Chunks []ChunkRef `json:"chunks,omitempty"`
	Link   string     `json:"link,omitempty"`
}

type FileTree struct {
	Files []FileNode `json:"files"`
}

type CompositeRef struct {
	Role string       `json:"role"`
	Hash string       `json:"hash"`
	Kind ArtifactKind `json:"kind,omitempty"`
}

type Artifact struct {
	Kind     ArtifactKind      `json:"kind"`
	Tree     *FileTree         `json:"tree,omitempty"`
	Blob     *BlobRef          `json:"blob,omitempty"`
	Object   *ObjectRef        `json:"object,omitempty"`
	OCI      *OCIDescriptor    `json:"oci,omitempty"`
	Remote   *RemoteRef        `json:"remote,omitempty"`
	Linked   []CompositeRef    `json:"linked,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type ResourceHints struct {
	MemoryBytes int64         `json:"memory_bytes,omitempty"`
	Fuel        uint64        `json:"fuel,omitempty"`
	Timeout     time.Duration `json:"timeout,omitempty"`
}

type Manifest struct {
	Kind             string              `json:"kind"`
	Tenant           string              `json:"tenant"`
	ProcessorID      string              `json:"processor_id"`
	Version          string              `json:"version"`
	Runtime          Runtime             `json:"runtime"`
	IOProtocol       IOProtocol          `json:"io_protocol"`
	RuntimeConfig    map[string]any      `json:"runtime_config,omitempty"`
	Entrypoint       string              `json:"entrypoint,omitempty"`
	HashAlgorithm    string              `json:"hash_algorithm"`
	ArtifactHash     string              `json:"artifact_hash"`
	ArtifactSize     int64               `json:"artifact_size"`
	Chunks           []ChunkRef          `json:"chunks"`
	Artifacts        map[string]Artifact `json:"artifacts,omitempty"`
	WITWorld         string              `json:"wit_world,omitempty"`
	Capabilities     []Capability        `json:"capabilities_required,omitempty"`
	Deterministic    bool                `json:"deterministic"`
	ResourceHints    ResourceHints       `json:"resource_hints,omitempty"`
	CompiledArtifact string              `json:"compiled_artifact,omitempty"`
	BuildProvenance  map[string]string   `json:"build_provenance,omitempty"`
	Cache            CachePolicy         `json:"cache,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
}

type CachePolicy struct {
	Enabled bool          `json:"enabled"`
	TTL     time.Duration `json:"ttl,omitempty"`
}


func (m *Manifest) Validate() error {
	if m == nil {
		return errors.New("manifest required")
	}
	m.applyDefaults()
	if m.Kind != ManifestKind {
		return fmt.Errorf("unsupported manifest kind: %s", m.Kind)
	}
	if !safeName.MatchString(m.Tenant) {
		return fmt.Errorf("invalid tenant: %s", m.Tenant)
	}
	if !safeName.MatchString(m.ProcessorID) {
		return fmt.Errorf("invalid processor id: %s", m.ProcessorID)
	}
	if !safeName.MatchString(m.Version) {
		return fmt.Errorf("invalid version: %s", m.Version)
	}
	if len(m.Chunks) == 0 && len(m.Artifacts) == 0 {
		// Direct-command process processors store their entrypoint in runtime_config
		// and carry no payload artifact — no file to upload, no OCI image to pull.
		if m.Runtime != RuntimeProcess || !hasDirectCommand(m.RuntimeConfig) {
			return errors.New("manifest artifact required")
		}
	}
	if err := ValidateArtifacts(m.Artifacts); err != nil {
		return err
	}
	if m.HashAlgorithm != HashAlgorithm && m.HashAlgorithm != HashAlgorithmOCI {
		return fmt.Errorf("unsupported hash algorithm: %s", m.HashAlgorithm)
	}
	if m.Runtime == "" {
		return errors.New("runtime required")
	}
	if m.IOProtocol == "" {
		return errors.New("io protocol required")
	}
	return nil
}

func (m *Manifest) RuntimeValue() Runtime {
	if m == nil || m.Runtime == "" {
		return RuntimeWasm
	}
	return m.Runtime
}

func (m *Manifest) IOProtocolValue() IOProtocol {
	if m == nil || m.IOProtocol == "" {
		return IOProtocolExternRefJSON
	}
	return m.IOProtocol
}

func (m *Manifest) applyDefaults() {
	if m.Kind == "" {
		m.Kind = ManifestKind
	}
	if m.Tenant == "" {
		m.Tenant = DefaultTenant
	}
	if m.HashAlgorithm == "" {
		m.HashAlgorithm = HashAlgorithm
	}
	if m.Runtime == "" {
		m.Runtime = RuntimeWasm
	}
	if m.IOProtocol == "" {
		m.IOProtocol = IOProtocolExternRefJSON
	}
}

func (m *Manifest) Marshal() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

func DecodeManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, m.Validate()
}

func ManifestKey(tenant, processorID, version string) string {
	return fmt.Sprintf("%s.processor.%s.%s", normTenant(tenant), processorID, version)
}

func TagKey(tenant, processorID, tag string) string {
	return fmt.Sprintf("%s.tag.%s.%s", normTenant(tenant), processorID, tag)
}

func ChunkKey(tenant, hash string) string {
	return fmt.Sprintf("%s.%s", normTenant(tenant), hash)
}

func RefKey(tenant, hash string) string {
	return fmt.Sprintf("%s.ref.%s", normTenant(tenant), hash)
}

func normTenant(tenant string) string {
	if tenant == "" {
		return DefaultTenant
	}
	return tenant
}

// HasDirectCommand returns true when the runtime_config specifies a concrete
// command that is not the {artifact} placeholder. Such processors carry no
// stored payload — the command itself is the full entrypoint.
func HasDirectCommand(cfg map[string]any) bool {
	cmd, _ := cfg["cmd"].(string)
	return cmd != "" && cmd != "{artifact}"
}

// hasDirectCommand is the unexported alias used internally by Validate.
func hasDirectCommand(cfg map[string]any) bool { return HasDirectCommand(cfg) }
