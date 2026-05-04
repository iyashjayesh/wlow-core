package artifact

import "strings"

const (
	SubjectManifestPut = "wlow.manifests.put"
	SubjectTenantKey   = "wlow.tenants.key"
)

// DescriptorRefKeyPart converts an OCI digest to a NATS KV key segment.
func DescriptorRefKeyPart(digest string) string {
	return "oci." + strings.ReplaceAll(digest, ":", ".")
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type ManifestPutRequest struct {
	Manifest Manifest `json:"manifest"`
	Tags     []string `json:"tags,omitempty"`
}

type ManifestPutResponse struct {
	ProcessorID  string `json:"processor_id"`
	Version      string `json:"version"`
	ArtifactHash string `json:"artifact_hash"`
}

type TenantKeyRequest struct {
	Tenant string `json:"tenant,omitempty"`
}

type TenantKeyResponse struct {
	Tenant string `json:"tenant"`
	Key    []byte `json:"key"`
}
