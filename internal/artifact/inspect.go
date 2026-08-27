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
	"oras.land/oras-go/v2/content"
)

// InspectOptions contains the internal inputs for built bundle inspection.
type InspectOptions struct {
	Source  string
	Config  *bundleinternal.UDSBundleConfig
	Streams iostreams.IOStreams
}

// InspectResult contains the parsed bundle and metadata extracted during inspection.
type InspectResult struct {
	Bundle            *spec.UDSBundle
	Packages          []*spec.Package
	ArtifactDigest    string
	ReconfiguredFrom  string
	PackageSignatures map[string]PackageSignatureSummary
	PackageZarfNames  map[string]string
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
func Inspect(ctx context.Context, opts InspectOptions) (*InspectResult, error) {
	if udsoci.IsOCIReference(opts.Source) {
		return inspectOCIReference(ctx, opts)
	}
	return inspectLocalArtifact(ctx, opts)
}

func inspectLocalArtifact(ctx context.Context, opts InspectOptions) (*InspectResult, error) {
	workspace, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-bundle-inspect-*")
	if err != nil {
		return nil, fmt.Errorf("%w under %q: %w", ErrCreatingInspectionWorkspace, opts.Config.Options.TmpDir, err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	if err := ExtractTarZst(ctx, opts.Streams, opts.Source, workspace); err != nil {
		return nil, fmt.Errorf("%w %q to %q: %w", ErrExtractingBundleArtifact, opts.Source, workspace, err)
	}

	ociDir := filepath.Join(workspace, "oci")
	indexPath := filepath.Join(ociDir, "index.json")
	indexBytes, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrReadingBundleIndex, indexPath, err)
	}

	store, err := udsoci.OpenReadOnlyStore(ociDir)
	if err != nil {
		return nil, err
	}
	return InspectBundleIndex(ctx, opts.Streams, indexBytes, digest.FromBytes(indexBytes).String(), store)
}

func inspectOCIReference(ctx context.Context, opts InspectOptions) (*InspectResult, error) {
	target, err := udsoci.NewRemoteRepository(ctx, udsoci.TrimScheme(opts.Source), *opts.Config.Options)
	if err != nil {
		return nil, ResolvingInspectSourceError{Source: opts.Source, Err: err}
	}
	reference, err := udsoci.ReferenceIdentifier(opts.Source)
	if err != nil {
		return nil, err
	}
	childDesc, indexBytes, err := udsoci.ResolveBundleChild(ctx, target, reference, opts.Config.Options.Architecture)
	if err != nil {
		return nil, ResolvingBundleSourceError{Source: opts.Source, Err: err}
	}

	return InspectBundleIndex(ctx, opts.Streams, indexBytes, childDesc.Digest.String(), target)
}

// InspectBundleIndex reads bundle metadata from an already-resolved OCI index.
// The fetcher may be backed by a local OCI layout or a remote registry; only
// the definition manifest, bundle HCL, and selected package manifests and
// zarf.yaml layers are fetched. When packageNames is empty, all packages are
// inspected.
func InspectBundleIndex(ctx context.Context, streams iostreams.IOStreams, indexBytes []byte, artifactDigest string, fetcher content.Fetcher, packageNames ...string) (*InspectResult, error) {
	var idx ocispec.Index
	if err := json.Unmarshal(indexBytes, &idx); err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrParsingBundleIndex, artifactDigest, err)
	}
	if !udsoci.IsBundleIndex(idx) {
		return nil, InvalidBundleIndexError{Source: "artifact", ArtifactType: udsoci.MediaTypeBundle}
	}
	if idx.SchemaVersion != 2 {
		return nil, UnsupportedSchemaVersionError{Artifact: "bundle index", Version: idx.SchemaVersion}
	}
	if idx.MediaType != "" && idx.MediaType != ocispec.MediaTypeImageIndex {
		return nil, UnsupportedMediaTypeError{Artifact: "bundle index", MediaType: idx.MediaType}
	}
	parseArch := strings.TrimSpace(idx.Annotations[udsoci.AnnotationBundleArchitecture])
	if parseArch == "" {
		return nil, MissingBundleArchitectureError{Annotation: udsoci.AnnotationBundleArchitecture}
	}

	definitionEntry, definitionIndex, err := udsoci.FindBundleDefinition(idx)
	if err != nil {
		return nil, err
	}
	for i, entry := range idx.Manifests {
		if i != definitionIndex && entry.ArtifactType == udsoci.MediaTypeBundleDefinition {
			return nil, ErrMultipleBundleDefinitionManifests
		}
	}
	if !udsoci.IsImageManifestMediaType(definitionEntry.MediaType) {
		return nil, UnsupportedMediaTypeError{Artifact: "bundle definition entry", MediaType: definitionEntry.MediaType}
	}
	definitionBytes, err := udsoci.FetchBytes(ctx, fetcher, definitionEntry)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrFetchingBundleDefinitionManifest, definitionEntry.Digest, err)
	}

	var definition ocispec.Manifest
	if err := json.Unmarshal(definitionBytes, &definition); err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrParsingBundleDefinitionManifest, definitionEntry.Digest, err)
	}
	if definition.SchemaVersion != 2 {
		return nil, UnsupportedSchemaVersionError{Artifact: "bundle definition manifest", Version: definition.SchemaVersion}
	}
	if definition.MediaType != "" && !udsoci.IsImageManifestMediaType(definition.MediaType) {
		return nil, UnsupportedMediaTypeError{Artifact: "bundle definition manifest", MediaType: definition.MediaType}
	}
	hclDesc, err := findLayerByTitle(definition, bundleinternal.BundleFileName)
	if err != nil {
		return nil, err
	}
	if hclDesc.MediaType != udsoci.MediaTypeBundleHCL {
		return nil, UnsupportedMediaTypeError{Artifact: "bundle definition HCL layer", MediaType: hclDesc.MediaType}
	}
	hclBytes, err := udsoci.FetchBytes(ctx, fetcher, hclDesc)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrFetchingBundleDefinitionHCL, hclDesc.Digest, err)
	}

	b, err := bundleinternal.NewHCLParser(parseArch, streams).ParseBundleBytes(ctx, hclBytes)
	if err != nil {
		return nil, err
	}
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrInvalidBundle, artifactDigest, err)
	}
	dag, err := bundleinternal.BuildDependencyGraph(ctx, streams, b)
	if err != nil {
		return nil, fmt.Errorf("building package dependency graph: %w", err)
	}
	packages, err := dag.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("ordering packages: %w", err)
	}
	result := &InspectResult{
		Bundle:            b,
		Packages:          packages,
		ArtifactDigest:    artifactDigest,
		ReconfiguredFrom:  definition.Annotations[udsoci.AnnotationReconfiguredFrom],
		PackageSignatures: make(map[string]PackageSignatureSummary, len(b.Packages)),
		PackageZarfNames:  make(map[string]string, len(b.Packages)),
	}
	selected := make(map[string]struct{}, len(packageNames))
	for _, name := range packageNames {
		selected[name] = struct{}{}
	}
	for _, pkg := range b.Packages {
		if len(selected) > 0 {
			if _, ok := selected[pkg.Name]; !ok {
				continue
			}
		}
		summary, err := inspectPackageSignature(ctx, idx, pkg, fetcher)
		if err != nil {
			return nil, InspectingPackageSignatureError{Package: pkg.Name, Err: err}
		}
		result.PackageSignatures[pkg.Name] = *summary

		entry, err := findPackageManifest(idx, pkg)
		if err != nil {
			return nil, err
		}
		zarfPkg, found, err := fetchZarfPackage(ctx, pkg.Name, *entry, fetcher)
		if err != nil {
			return nil, err
		}
		if found && zarfPkg.Metadata.Name != "" {
			result.PackageZarfNames[pkg.Name] = zarfPkg.Metadata.Name
		}
	}

	return result, nil
}

func inspectPackageSignature(ctx context.Context, idx ocispec.Index, pkg spec.Package, fetcher content.Fetcher) (*PackageSignatureSummary, error) {
	entry, err := findPackageManifest(idx, pkg)
	if err != nil {
		return nil, err
	}
	summary := &PackageSignatureSummary{
		Signed:       PackageSigningStatusUnknown,
		Verification: packageVerificationStatus(pkg.SignatureVerification, entry),
	}
	zarfPkg, found, err := fetchZarfPackage(ctx, pkg.Name, *entry, fetcher)
	if err != nil {
		return nil, err
	}
	if !found {
		return summary, nil
	}

	if zarfPkg.Build.Signed != nil {
		if *zarfPkg.Build.Signed {
			summary.Signed = PackageSigningStatusSigned
		} else {
			summary.Signed = PackageSigningStatusUnsigned
		}
	}

	return summary, nil
}

func findPackageManifest(idx ocispec.Index, pkg spec.Package) (*ocispec.Descriptor, error) {
	var match *ocispec.Descriptor
	for i := range idx.Manifests {
		entry := &idx.Manifests[i]
		if entry.ArtifactType == udsoci.MediaTypeBundleDefinition {
			continue
		}
		packageName := entry.Annotations[udsoci.AnnotationPackageName]
		if packageName == "" {
			err := MissingPackageManifestAnnotationError{Index: i, Annotation: udsoci.AnnotationPackageName}
			return nil, fmt.Errorf("%w; recreate the bundle with package-name identity", err)
		}
		if packageName == pkg.Name {
			if !udsoci.IsImageManifestMediaType(entry.MediaType) {
				return nil, UnsupportedPackageEntryMediaTypeError{Package: pkg.Name, MediaType: entry.MediaType}
			}
			if match != nil {
				return nil, MultiplePackageManifestEntriesError{Package: pkg.Name}
			}
			match = entry
		}
	}
	if match != nil {
		return match, nil
	}
	return nil, PackageManifestNotFoundError{Package: pkg.Name, Source: pkg.Source}
}

func findLayerByTitle(manifest ocispec.Manifest, title string) (ocispec.Descriptor, error) {
	for _, layer := range manifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == title {
			return layer, nil
		}
	}
	return ocispec.Descriptor{}, LayerNotFoundError{Title: title}
}

func packageVerificationStatus(verification *spec.PackageSignatureVerification, manifest *ocispec.Descriptor) PackageVerificationStatus {
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
