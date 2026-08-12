// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gopkg.in/yaml.v3"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
)

type inspectBlobFetcher func(context.Context, ocispec.Descriptor) ([]byte, error)

type packageSigningMetadata struct {
	Build struct {
		Signed *bool `yaml:"signed"`
	} `yaml:"build"`
}

type inspectOCIIndex = udsoci.OciIndex
type inspectOCIManifest = udsoci.OciManifest
type inspectOCIDescriptor = udsoci.OciDescriptor
type inspectOCIImageManifest = udsoci.OciImageManifest

const maxInspectMetadataSize = 16 << 20

// InspectOptions contains the internal inputs for built bundle inspection.
type InspectOptions struct {
	Source  string
	Config  *bundleinternal.UDSBundleConfig
	Streams iostreams.IOStreams
}

// InspectTargetResolver provides an internal test seam for OCI sources.
type InspectTargetResolver func(context.Context, string, *InspectOptions) (oras.Target, error)

// InspectResult contains the parsed bundle and metadata extracted during inspection.
type InspectResult struct {
	Bundle            *spec.UDSBundle
	ArtifactDigest    string
	ReconfiguredFrom  string
	PackageSignatures map[string]PackageSignatureSummary
}

// PackageSignatureSummary contains package signing and verification metadata.
type PackageSignatureSummary struct {
	Signed       PackageSigningStatus
	Verification PackageVerificationStatus
}

// PackageSigningStatus describes signing metadata persisted in a package.
type PackageSigningStatus uint8

const (
	PackageSigningStatusUnknown PackageSigningStatus = iota
	PackageSigningStatusSigned
	PackageSigningStatusUnsigned
)

// PackageVerificationStatus describes the verification posture persisted in a package manifest.
type PackageVerificationStatus uint8

const (
	PackageVerificationStatusUnknown PackageVerificationStatus = iota
	PackageVerificationStatusVerified
	PackageVerificationStatusSkipped
)

// Inspect reads a built local or OCI bundle.
// It reads metadata only and does not verify package content or signatures.
func Inspect(ctx context.Context, opts InspectOptions, targetResolver InspectTargetResolver) (*InspectResult, error) {
	if udsoci.IsOCIReference(opts.Source) {
		return inspectOCIReference(ctx, opts, targetResolver)
	}
	return inspectLocalArtifact(ctx, opts)
}

func inspectLocalArtifact(ctx context.Context, opts InspectOptions) (*InspectResult, error) {
	workspace, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-bundle-inspect-*")
	if err != nil {
		return nil, fmt.Errorf("creating inspection workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	if err := ExtractTarZst(ctx, opts.Streams, opts.Source, workspace); err != nil {
		return nil, fmt.Errorf("extracting bundle artifact: %w", err)
	}

	ociDir, err := udsoci.FindOCILayoutRoot(workspace)
	if err != nil {
		return nil, fmt.Errorf("locating bundle OCI layout: %w", err)
	}

	indexPath := filepath.Join(ociDir, "index.json")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("reading bundle index: %w", err)
	}

	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	fetch := func(_ context.Context, desc ocispec.Descriptor) ([]byte, error) {
		return udsoci.ReadLocalBlob(blobDir, desc, maxInspectMetadataSize)
	}
	return inspectBundleIndex(ctx, opts.Streams, indexBytes, digest.FromBytes(indexBytes).String(), fetch)
}

func inspectOCIReference(ctx context.Context, opts InspectOptions, targetResolver InspectTargetResolver) (*InspectResult, error) {
	target, err := resolveInspectTarget(ctx, opts, targetResolver)
	if err != nil {
		return nil, fmt.Errorf("resolving inspect source %s: %w", opts.Source, err)
	}
	reference, err := udsoci.ReferenceIdentifier(opts.Source)
	if err != nil {
		return nil, err
	}
	childDesc, indexBytes, err := udsoci.ResolveBundleChild(ctx, target, reference, opts.Config.Options.Architecture)
	if err != nil {
		return nil, fmt.Errorf("resolving bundle from %s: %w", opts.Source, err)
	}

	fetch := func(ctx context.Context, desc ocispec.Descriptor) ([]byte, error) {
		if err := udsoci.ValidateBlobSize(desc, maxInspectMetadataSize); err != nil {
			return nil, err
		}
		data, err := content.FetchAll(ctx, target, desc)
		if err != nil {
			return nil, fmt.Errorf("fetching metadata %s: %w", desc.Digest, err)
		}
		return data, nil
	}
	return inspectBundleIndex(ctx, opts.Streams, indexBytes, childDesc.Digest.String(), fetch)
}

func resolveInspectTarget(ctx context.Context, opts InspectOptions, targetResolver InspectTargetResolver) (oras.Target, error) {
	if targetResolver != nil {
		target, err := targetResolver(ctx, udsoci.TrimScheme(opts.Source), &opts)
		if err != nil {
			return nil, err
		}
		if target == nil {
			return nil, fmt.Errorf("inspect target is nil")
		}
		return target, nil
	}
	return udsoci.NewRemoteRepository(ctx, udsoci.TrimScheme(opts.Source), *opts.Config.Options)
}

func inspectBundleIndex(ctx context.Context, streams iostreams.IOStreams, indexBytes []byte, artifactDigest string, fetch inspectBlobFetcher) (*InspectResult, error) {
	var idx inspectOCIIndex
	if err := json.Unmarshal(indexBytes, &idx); err != nil {
		return nil, fmt.Errorf("parsing bundle index: %w", err)
	}
	if !udsoci.IsBundleIndex(idx) {
		return nil, fmt.Errorf("artifact does not appear to be a UDS bundle: index does not declare artifactType %s", udsoci.MediaTypeBundle)
	}
	if idx.SchemaVersion != 2 {
		return nil, fmt.Errorf("bundle index has unsupported schema version %d", idx.SchemaVersion)
	}
	if idx.MediaType != "" && idx.MediaType != ocispec.MediaTypeImageIndex {
		return nil, fmt.Errorf("bundle index has unsupported media type %q", idx.MediaType)
	}
	parseArch := strings.TrimSpace(idx.Annotations[udsoci.AnnotationBundleArchitecture])
	if parseArch == "" {
		return nil, fmt.Errorf("bundle index does not record its architecture: missing the %s annotation", udsoci.AnnotationBundleArchitecture)
	}

	definitionEntry, definitionIndex, err := udsoci.FindBundleDefinitionEntry(idx)
	if err != nil {
		return nil, err
	}
	for i, entry := range idx.Manifests {
		if i != definitionIndex && entry.ArtifactType == udsoci.MediaTypeBundleDefinition {
			return nil, fmt.Errorf("bundle index contains multiple bundle definition manifests")
		}
	}
	if !udsoci.IsImageManifestMediaType(definitionEntry.MediaType) {
		return nil, fmt.Errorf("bundle definition entry has unsupported media type %q", definitionEntry.MediaType)
	}
	definitionDesc, err := udsoci.DescriptorFromOCI(inspectOCIDescriptor{
		MediaType: definitionEntry.MediaType,
		Digest:    definitionEntry.Digest,
		Size:      definitionEntry.Size,
	})
	if err != nil {
		return nil, fmt.Errorf("parsing bundle definition descriptor: %w", err)
	}
	definitionBytes, err := fetch(ctx, definitionDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching bundle definition manifest: %w", err)
	}

	var definition inspectOCIImageManifest
	if err := json.Unmarshal(definitionBytes, &definition); err != nil {
		return nil, fmt.Errorf("parsing bundle definition manifest: %w", err)
	}
	if definition.SchemaVersion != 2 {
		return nil, fmt.Errorf("bundle definition manifest has unsupported schema version %d", definition.SchemaVersion)
	}
	if definition.MediaType != "" && !udsoci.IsImageManifestMediaType(definition.MediaType) {
		return nil, fmt.Errorf("bundle definition manifest has unsupported media type %q", definition.MediaType)
	}
	hclLayer, err := udsoci.FindLayerByTitle(definition, bundleinternal.BundleFileName)
	if err != nil {
		return nil, err
	}
	hclDesc, err := udsoci.DescriptorFromOCI(hclLayer)
	if err != nil {
		return nil, fmt.Errorf("parsing bundle definition HCL descriptor: %w", err)
	}
	if hclLayer.MediaType != udsoci.MediaTypeBundleHCL {
		return nil, fmt.Errorf("bundle definition HCL layer has unsupported media type %q", hclLayer.MediaType)
	}
	hclBytes, err := fetch(ctx, hclDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching bundle definition HCL: %w", err)
	}

	b, err := bundleinternal.NewHCLParser(parseArch, streams).ParseBundleBytes(ctx, hclBytes)
	if err != nil {
		return nil, err
	}
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("invalid bundle: %w", err)
	}
	result := &InspectResult{
		Bundle:            b,
		ArtifactDigest:    artifactDigest,
		ReconfiguredFrom:  definition.Annotations[udsoci.AnnotationReconfiguredFrom],
		PackageSignatures: make(map[string]PackageSignatureSummary, len(b.Packages)),
	}
	for _, pkg := range b.Packages {
		summary, err := inspectPackageSignature(ctx, idx, pkg, fetch)
		if err != nil {
			return nil, fmt.Errorf("inspecting package %q signature: %w", pkg.Name, err)
		}
		result.PackageSignatures[pkg.Name] = *summary
	}

	return result, nil
}

func inspectPackageSignature(ctx context.Context, idx inspectOCIIndex, pkg spec.Package, fetch inspectBlobFetcher) (*PackageSignatureSummary, error) {
	entry, err := findPackageManifest(idx, pkg)
	if err != nil {
		return nil, err
	}
	summary := &PackageSignatureSummary{
		Signed:       PackageSigningStatusUnknown,
		Verification: packageVerificationStatus(pkg.SignatureVerification, entry),
	}
	manifestDesc, err := udsoci.DescriptorFromOCI(inspectOCIDescriptor{
		MediaType: entry.MediaType,
		Digest:    entry.Digest,
		Size:      entry.Size,
	})
	if err != nil {
		return nil, fmt.Errorf("parsing package manifest descriptor: %w", err)
	}
	manifestBytes, err := fetch(ctx, manifestDesc)
	if err != nil {
		return nil, fmt.Errorf("fetching package manifest: %w", err)
	}
	var manifest inspectOCIImageManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parsing package manifest: %w", err)
	}
	if manifest.SchemaVersion != 2 {
		return nil, fmt.Errorf("package manifest has unsupported schema version %d", manifest.SchemaVersion)
	}
	if manifest.MediaType != "" && !udsoci.IsImageManifestMediaType(manifest.MediaType) {
		return nil, fmt.Errorf("package manifest has unsupported media type %q", manifest.MediaType)
	}

	zarfLayer, ok := findLayerByTitleOptional(manifest, "zarf.yaml")
	if ok {
		desc, err := udsoci.DescriptorFromOCI(zarfLayer)
		if err != nil {
			return nil, fmt.Errorf("parsing zarf.yaml descriptor: %w", err)
		}
		zarfBytes, err := fetch(ctx, desc)
		if err != nil {
			return nil, fmt.Errorf("fetching zarf.yaml: %w", err)
		}
		var metadata packageSigningMetadata
		if err := yaml.Unmarshal(zarfBytes, &metadata); err != nil {
			return nil, fmt.Errorf("parsing zarf.yaml: %w", err)
		}
		if metadata.Build.Signed != nil {
			if *metadata.Build.Signed {
				summary.Signed = PackageSigningStatusSigned
			} else {
				summary.Signed = PackageSigningStatusUnsigned
			}
		}
	}
	return summary, nil
}

func findPackageManifest(idx inspectOCIIndex, pkg spec.Package) (*inspectOCIManifest, error) {
	key := pkg.Name
	if udsoci.IsOCIReference(pkg.Source) {
		key = udsoci.TrimScheme(pkg.Source)
	}
	var match *inspectOCIManifest
	for i := range idx.Manifests {
		entry := &idx.Manifests[i]
		if entry.ArtifactType == udsoci.MediaTypeBundleDefinition {
			continue
		}
		if entry.Annotations["org.opencontainers.image.ref.name"] == key {
			if !udsoci.IsImageManifestMediaType(entry.MediaType) {
				return nil, fmt.Errorf("package %q entry has unsupported media type %q", pkg.Name, entry.MediaType)
			}
			if match != nil {
				return nil, fmt.Errorf("package %q has multiple matching manifest entries", pkg.Name)
			}
			match = entry
		}
	}
	if match != nil {
		return match, nil
	}
	return nil, fmt.Errorf("package %q with source %q was not found in bundle index", pkg.Name, pkg.Source)
}

func findLayerByTitleOptional(manifest inspectOCIImageManifest, title string) (inspectOCIDescriptor, bool) {
	for _, layer := range manifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == title {
			return layer, true
		}
	}
	return inspectOCIDescriptor{}, false
}

func packageVerificationStatus(verification *spec.PackageSignatureVerification, manifest *inspectOCIManifest) PackageVerificationStatus {
	if manifest != nil && manifest.Annotations[udsoci.AnnotationPackageVerification] == udsoci.AnnotationPackageVerificationVerified {
		return PackageVerificationStatusVerified
	}
	if verification == nil {
		return PackageVerificationStatusUnknown
	}
	if verification.Verify != nil && !*verification.Verify {
		return PackageVerificationStatusSkipped
	}
	return PackageVerificationStatusUnknown
}
