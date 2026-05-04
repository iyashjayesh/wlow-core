package artifact

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

const (
	MediaTypeSnapshotConfig = "application/vnd.wlow.snapshot.config.v1+json"
	MediaTypeSnapshotState  = "application/vnd.wlow.snapshot.state.v1+json"
	MediaTypeSnapshotMemory = "application/vnd.wlow.snapshot.memory.v1"
	MediaTypeSnapshotRootfs = "application/vnd.wlow.snapshot.rootfs.v1"
	MediaTypeRootfsEROFS    = "application/vnd.wlow.rootfs.erofs.v1"
)

type RemoteImageDescriptor struct {
	Descriptor OCIDescriptor
	Role       string
}

func InspectRemoteOCI(ctx context.Context, imageRef string, opts PushOptions) (*Manifest, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, err
	}
	desc, err := remote.Get(ref, remoteOptions(ctx)...)
	if err != nil {
		return nil, err
	}
	descs, err := remoteDescriptors(ctx, ref, desc)
	if err != nil {
		return nil, err
	}
	hash := desc.Digest.String()
	size := int64(0)
	for idx := 0; idx < len(descs) && idx < maxOCIDescriptors; idx++ {
		size += descs[idx].Descriptor.Size
	}
	manifest := remoteOCIManifest(opts, imageRef, hash, size, descs)
	return manifest, manifest.Validate()
}

func remoteDescriptors(ctx context.Context, ref name.Reference, desc *remote.Descriptor) ([]RemoteImageDescriptor, error) {
	out := []RemoteImageDescriptor{{
		Role: RoleOCIIndex,
		Descriptor: OCIDescriptor{
			Digest:    desc.Digest.String(),
			MediaType: string(desc.MediaType),
			Size:      desc.Size,
		},
	}}
	if desc.MediaType.IsIndex() {
		var index v1.IndexManifest
		if err := json.Unmarshal(desc.Manifest, &index); err != nil {
			return nil, err
		}
		for idx := 0; idx < len(index.Manifests) && idx < maxOCIDescriptors; idx++ {
			child, err := remote.Get(ref.Context().Digest(index.Manifests[idx].Digest.String()), remoteOptions(ctx)...)
			if err != nil {
				return nil, err
			}
			items, err := remoteManifestDescriptors(child)
			if err != nil {
				return nil, err
			}
			out = append(out, items...)
		}
		return dedupeRemoteDescriptors(out), nil
	}
	items, err := remoteManifestDescriptors(desc)
	if err != nil {
		return nil, err
	}
	out = append(out, items...)
	return dedupeRemoteDescriptors(out), nil
}

func remoteManifestDescriptors(desc *remote.Descriptor) ([]RemoteImageDescriptor, error) {
	var manifest v1.Manifest
	if err := json.Unmarshal(desc.Manifest, &manifest); err != nil {
		return nil, err
	}
	out := []RemoteImageDescriptor{{
		Role: descriptorRole(OCIDescriptor{MediaType: string(desc.MediaType), Digest: desc.Digest.String()}, 1),
		Descriptor: OCIDescriptor{
			Digest:    desc.Digest.String(),
			MediaType: string(desc.MediaType),
			Size:      desc.Size,
		},
	}, {
		Role: descriptorRole(OCIDescriptor{MediaType: string(manifest.Config.MediaType), Digest: manifest.Config.Digest.String()}, 2),
		Descriptor: OCIDescriptor{
			Digest:    manifest.Config.Digest.String(),
			MediaType: string(manifest.Config.MediaType),
			Size:      manifest.Config.Size,
		},
	}}
	for idx := 0; idx < len(manifest.Layers) && idx < maxOCIDescriptors; idx++ {
		layer := manifest.Layers[idx]
		desc := OCIDescriptor{
			Digest:      layer.Digest.String(),
			MediaType:   string(layer.MediaType),
			Size:        layer.Size,
			Annotations: layer.Annotations,
		}
		out = append(out, RemoteImageDescriptor{Role: descriptorRole(desc, idx+3), Descriptor: desc})
	}
	return out, nil
}

func dedupeRemoteDescriptors(in []RemoteImageDescriptor) []RemoteImageDescriptor {
	seen := make(map[string]struct{}, len(in))
	out := make([]RemoteImageDescriptor, 0, len(in))
	for idx := 0; idx < len(in) && idx < maxOCIDescriptors; idx++ {
		digest := in[idx].Descriptor.Digest
		if _, ok := seen[digest]; ok {
			continue
		}
		seen[digest] = struct{}{}
		out = append(out, in[idx])
	}
	return out
}

func remoteOCIManifest(opts PushOptions, imageRef, hash string, size int64, descs []RemoteImageDescriptor) *Manifest {
	tenant := opts.Tenant
	if tenant == "" {
		tenant = DefaultTenant
	}
	cfg := copyRuntimeConfig(opts.Manifest.RuntimeConfig)
	cfg["image_ref"] = imageRef
	m := &Manifest{
		Kind:             ManifestKind,
		Tenant:           tenant,
		ProcessorID:      opts.ProcessorID,
		Version:          opts.Version,
		Runtime:          opts.Manifest.Runtime,
		IOProtocol:       opts.Manifest.IOProtocol,
		RuntimeConfig:    cfg,
		HashAlgorithm:    HashAlgorithmOCI,
		ArtifactHash:     hash,
		ArtifactSize:     size,
		Artifacts:        map[string]Artifact{},
		WITWorld:         opts.Manifest.WITWorld,
		Capabilities:     opts.Manifest.Capabilities,
		Deterministic:    opts.Manifest.Deterministic,
		ResourceHints:    opts.Manifest.ResourceHints,
		CompiledArtifact: opts.Manifest.CompiledArtifact,
		BuildProvenance:  opts.Manifest.BuildProvenance,
		Cache:            opts.Manifest.Cache,
		CreatedAt:        time.Now().UTC(),
	}
	for idx := 0; idx < len(descs) && idx < maxOCIDescriptors; idx++ {
		item := descs[idx]
		desc := item.Descriptor
		m.Artifacts[item.Role] = Artifact{Kind: KindOCIDescriptor, OCI: &desc}
	}
	return m
}

func PushSnapshotImage(ctx context.Context, imageRef string, files SnapshotFiles) (SnapshotObjects, string, int64, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return SnapshotObjects{}, "", 0, err
	}
	layers, refs, err := snapshotLayers(files)
	if err != nil {
		return SnapshotObjects{}, "", 0, err
	}
	img, err := mutate.AppendLayers(empty.Image, layers...)
	if err != nil {
		return SnapshotObjects{}, "", 0, err
	}
	if err := remote.Write(ref, img, remoteOptions(ctx)...); err != nil {
		return SnapshotObjects{}, "", 0, err
	}
	attachSnapshotRepo(&refs, ref.Context())
	hash, err := img.Digest()
	if err != nil {
		return SnapshotObjects{}, "", 0, err
	}
	size := refs.Config.Size + refs.State.Size + refs.Memory.Size + refs.Rootfs.Size
	return refs, hash.String(), size, nil
}

func PushRootfsImage(ctx context.Context, imageRef string, rootfsPath string) (*RemoteRef, string, int64, error) {
	if imageRef == "" {
		return nil, "", 0, errors.New("rootfs image ref required")
	}
	if rootfsPath == "" {
		return nil, "", 0, errors.New("rootfs path required")
	}
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, "", 0, err
	}
	data, err := os.ReadFile(rootfsPath)
	if err != nil {
		return nil, "", 0, err
	}
	if len(data) == 0 {
		return nil, "", 0, errors.New("rootfs is empty")
	}
	layer := static.NewLayer(data, MediaTypeRootfsEROFS)
	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		return nil, "", 0, err
	}
	if err := remote.Write(ref, img, remoteOptions(ctx)...); err != nil {
		return nil, "", 0, err
	}
	digest, err := layer.Digest()
	if err != nil {
		return nil, "", 0, err
	}
	size, err := layer.Size()
	if err != nil {
		return nil, "", 0, err
	}
	imageDigest, err := img.Digest()
	if err != nil {
		return nil, "", 0, err
	}
	remote := &RemoteRef{
		Ref:       ref.Context().Digest(digest.String()).Name(),
		Digest:    digest.String(),
		Size:      size,
		MediaType: string(MediaTypeRootfsEROFS),
	}
	return remote, imageDigest.String(), size, nil
}

func attachSnapshotRepo(objects *SnapshotObjects, repo name.Repository) {
	for _, ref := range []*RemoteRef{objects.Config, objects.State, objects.Memory, objects.Rootfs} {
		if ref != nil {
			ref.Ref = repo.Digest(ref.Digest).Name()
		}
	}
}

func PullRemoteFile(ctx context.Context, remoteRef *RemoteRef, dest string) error {
	if remoteRef == nil {
		return errors.New("remote ref required")
	}
	ref, err := name.NewDigest(remoteRef.Ref)
	if err != nil {
		return err
	}
	layer, err := remote.Layer(ref, remoteOptions(ctx)...)
	if err != nil {
		return err
	}
	rc, err := layer.Compressed()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	n, err := io.Copy(out, rc)
	if err != nil {
		return err
	}
	if remoteRef.Size >= 0 && n != remoteRef.Size {
		return fmt.Errorf("remote size mismatch: wrote %d want %d", n, remoteRef.Size)
	}
	return nil
}

func PullOCILayer(ctx context.Context, imageRef, digest, dest string) error {
	if imageRef == "" {
		return errors.New("image ref required")
	}
	if digest == "" {
		return errors.New("digest required")
	}
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return err
	}
	layerRef := ref.Context().Digest(digest)
	layer, err := remote.Layer(layerRef, remoteOptions(ctx)...)
	if err != nil {
		return err
	}
	rc, err := layer.Compressed()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func remoteOptions(ctx context.Context) []remote.Option {
	options := []remote.Option{remote.WithContext(ctx)}
	auth := strings.TrimSpace(os.Getenv("IMAGE_PULL_AUTH"))
	if auth == "" {
		return append(options, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	}
	decoded, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		return append(options, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	}
	user, pass, ok := strings.Cut(string(decoded), ":")
	if !ok || user == "" || pass == "" {
		return append(options, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	}
	return append(options, remote.WithAuth(authn.FromConfig(authn.AuthConfig{
		Username: user,
		Password: pass,
	})))
}

func snapshotLayers(files SnapshotFiles) ([]v1.Layer, SnapshotObjects, error) {
	items := []struct {
		role      string
		path      string
		mediaType types.MediaType
		assign    func(*SnapshotObjects, *RemoteRef)
	}{
		{RoleSnapshotConfig, files.Config, MediaTypeSnapshotConfig, func(o *SnapshotObjects, r *RemoteRef) { o.Config = r }},
		{RoleSnapshotState, files.State, MediaTypeSnapshotState, func(o *SnapshotObjects, r *RemoteRef) { o.State = r }},
		{RoleSnapshotMemory, files.Memory, MediaTypeSnapshotMemory, func(o *SnapshotObjects, r *RemoteRef) { o.Memory = r }},
		{RoleSnapshotRootfs, files.Rootfs, MediaTypeSnapshotRootfs, func(o *SnapshotObjects, r *RemoteRef) { o.Rootfs = r }},
	}
	layers := make([]v1.Layer, 0, len(items))
	objects := SnapshotObjects{}
	for idx := 0; idx < len(items); idx++ {
		data, err := os.ReadFile(items[idx].path)
		if err != nil {
			return nil, objects, err
		}
		layer := static.NewLayer(data, items[idx].mediaType)
		digest, err := layer.Digest()
		if err != nil {
			return nil, objects, err
		}
		size, err := layer.Size()
		if err != nil {
			return nil, objects, err
		}
		items[idx].assign(&objects, &RemoteRef{Digest: digest.String(), Size: size, MediaType: string(items[idx].mediaType)})
		layers = append(layers, layer)
	}
	return layers, objects, nil
}
