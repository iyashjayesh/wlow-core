package artifact

import "strings"

const maxOCIDescriptors = 1 << 14

func descriptorRole(desc OCIDescriptor, idx int) string {
	if idx == 0 {
		return RoleOCIIndex
	}
	return "oci.descriptor." + digestSuffix(desc.Digest)
}

func digestSuffix(digest string) string {
	_, value, ok := strings.Cut(digest, ":")
	if !ok || len(value) < 16 {
		return strings.ReplaceAll(digest, ":", ".")
	}
	return value[:16]
}
