// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Reconfigure validates the defaults file and dispatches to the appropriate
// implementation based on whether the source is a local tarball or OCI reference.
func Reconfigure(ctx context.Context, opts ReconfigureOptions) (*ReconfigureResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	defaultsData, err := bundleinternal.MaterializeDefaultsFile(opts.DefaultsFile)
	if err != nil {
		return nil, fmt.Errorf("reading defaults file: %w", err)
	}
	opts.materializedDefaults = defaultsData
	s := logger.Bind(opts.Streams, opts.Options.LogLevel)

	if IsOCIReference(opts.Source) {
		return (&ociReconfigurer{streams: s}).Reconfigure(ctx, opts)
	}
	return (&localReconfigurer{streams: s}).Reconfigure(ctx, opts)
}

type localReconfigurer struct {
	streams iostreams.IOStreams
}

func (r *localReconfigurer) Reconfigure(ctx context.Context, opts ReconfigureOptions) (*ReconfigureResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	// Compute output filename and check it doesn't exist.
	baseName := filepath.Base(opts.Source)
	if !strings.HasSuffix(baseName, ".tar.zst") {
		return nil, fmt.Errorf("source must be a .tar.zst file, got: %s", baseName)
	}
	outName := reconfiguredFileOutputName(baseName, opts.Suffix)
	outPath := filepath.Join(opts.OutputDir, outName)
	if _, err := os.Stat(outPath); err == nil {
		return nil, fmt.Errorf("output file already exists: %s", outPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("checking output path: %w", err)
	}

	// Extract to temp dir.
	tmp, err := os.MkdirTemp(opts.Options.TmpDir, "uds-bundle-reconfigure-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(tmp); rmErr != nil {
			r.streams.Warn("failed to remove temp dir", "path", tmp, "error", rmErr)
		}
	}()

	if err := artifact.ExtractTarZst(ctx, r.streams, opts.Source, tmp); err != nil {
		return nil, fmt.Errorf("extracting bundle: %w", err)
	}

	ociDir := filepath.Join(tmp, "oci")
	// Parse index.json and find bundle definition manifest.
	idxBytes, err := os.ReadFile(filepath.Join(ociDir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("reading index.json: %w", err)
	}
	var idx ocispec.Index
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return nil, fmt.Errorf("parsing index: %w", err)
	}
	if !udsoci.IsBundleIndex(idx) {
		return nil, fmt.Errorf("%s does not appear to be a UDS bundle: index does not declare artifactType %s", opts.Source, MediaTypeBundle)
	}
	if err := validateLocalReconfigureIndex(idx); err != nil {
		return nil, err
	}
	sourceArtifactDigest := godigest.FromBytes(idxBytes).String()
	readStore, err := udsoci.OpenReadOnlyStore(ociDir)
	if err != nil {
		return nil, fmt.Errorf("opening OCI layout for metadata reads: %w", err)
	}

	defEntry, defPos, err := udsoci.FindBundleDefinition(idx)
	if err != nil {
		return nil, fmt.Errorf("%s does not appear to be a UDS bundle: %w", opts.Source, err)
	}

	// Read and parse the bundle definition manifest before opening the eager writable store.
	manifestBytes, err := udsoci.FetchBytes(ctx, readStore, defEntry)
	if err != nil {
		return nil, fmt.Errorf("reading bundle definition manifest: %w", err)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parsing bundle definition manifest: %w", err)
	}

	defaultsData, err := reconfigureDefaultsData(opts)
	if err != nil {
		return nil, fmt.Errorf("reading defaults file: %w", err)
	}

	hclLayer, err := findLayerByTitle(manifest, BundleFileName)
	if err != nil {
		return nil, err
	}
	hclBytes, err := udsoci.FetchBytes(ctx, readStore, hclLayer)
	if err != nil {
		return nil, fmt.Errorf("reading bundle HCL: %w", err)
	}
	store, err := udsoci.OpenStore(ociDir)
	if err != nil {
		return nil, fmt.Errorf("opening OCI layout for writes: %w", err)
	}
	newDefaultsDesc, err := store.PushBytes(ctx, MediaTypeBundleHCL, defaultsData)
	if err != nil {
		return nil, fmt.Errorf("writing defaults blob: %w", err)
	}
	newDefaultsDesc.Annotations = map[string]string{ocispec.AnnotationTitle: BundleDefaultsFileName}

	splicedHCL, err := spliceHCLName(hclBytes, opts.Suffix)
	if err != nil {
		return nil, fmt.Errorf("updating bundle name: %w", err)
	}
	newHCLDesc, err := store.PushBytes(ctx, MediaTypeBundleHCL, splicedHCL)
	if err != nil {
		return nil, fmt.Errorf("writing updated HCL blob: %w", err)
	}
	newHCLDesc.Annotations = map[string]string{ocispec.AnnotationTitle: BundleFileName}

	// Rebuild the bundle definition manifest.
	newManifestBytes, err := rebuildDefinitionManifest(manifest, newDefaultsDesc, newHCLDesc, sourceArtifactDigest)
	if err != nil {
		return nil, fmt.Errorf("rebuilding manifest: %w", err)
	}
	newManifestDesc, err := udsoci.PushManifestBytes(ctx, store, ocispec.MediaTypeImageManifest, MediaTypeBundleDefinition, newManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("writing manifest blob: %w", err)
	}

	// Update index.json, re-sorting by digest to preserve the deterministic
	// ordering invariant of bundle indexes (ADR-0015).
	idx.Manifests[defPos] = newManifestDesc
	udsoci.SortDescriptors(idx.Manifests)
	if err := udsoci.WriteIndex(filepath.Join(ociDir, "index.json"), &idx); err != nil {
		return nil, fmt.Errorf("writing index.json: %w", err)
	}

	// Remove unreferenced blobs.
	if err := store.PruneUnreferencedBlobs(ctx, r.streams, idx.Manifests); err != nil {
		return nil, fmt.Errorf("cleaning unreferenced blobs: %w", err)
	}

	// Repackage as tar.zst.
	if err := artifact.WriteTarZst(ctx, r.streams, outPath, tmp); err != nil {
		return nil, fmt.Errorf("writing output archive: %w", err)
	}

	r.streams.Info("bundle reconfigured", "output", outPath)
	return &ReconfigureResult{OutputPath: outPath}, nil
}

func validateLocalReconfigureIndex(idx ocispec.Index) error {
	for i, desc := range idx.Manifests {
		if desc.Size > udsoci.MaxFetchBytesSize {
			return fmt.Errorf("bundle index manifest at index %d (%s) is %d bytes, larger than the %d byte metadata limit", i, desc.Digest, desc.Size, udsoci.MaxFetchBytesSize)
		}
	}
	return nil
}

type ociReconfigurer struct {
	streams iostreams.IOStreams
}

func (r *ociReconfigurer) Reconfigure(ctx context.Context, opts ReconfigureOptions) (*ReconfigureResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if opts.OutputDir != "" {
		return nil, fmt.Errorf("--output-dir is not supported for OCI sources")
	}

	// Compute the source tag and derivative target tag.
	trimmed := TrimScheme(opts.Source)
	sourceTag, targetTag, targetRef, err := udsoci.TaggedDerivativeReference(trimmed, opts.Suffix)
	if err != nil {
		return nil, err
	}

	// Get registry target.
	repo := opts.remoteRepo
	if repo == nil {
		remote, err := udsoci.NewRemoteRepository(ctx, trimmed, toInternalConfigOptions(opts.Options))
		if err != nil {
			return nil, fmt.Errorf("connecting to registry: %w", err)
		}
		repo = remote
	}

	// Resolve source to the canonical single-arch bundle (child) index,
	// platform-selecting from the root index when the tag points at one.
	sourceChild, indexBytes, err := udsoci.ResolveBundleChild(ctx, repo, sourceTag, opts.Options.Architecture)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", opts.Source, err)
	}

	// Check target tag doesn't exist.
	if err := udsoci.EnsureTagAvailable(ctx, repo, targetTag); err != nil {
		return nil, err
	}

	var idx ocispec.Index
	if err := json.Unmarshal(indexBytes, &idx); err != nil {
		return nil, fmt.Errorf("parsing index: %w", err)
	}

	// Find bundle definition manifest.
	defEntry, defPos, err := udsoci.FindBundleDefinition(idx)
	if err != nil {
		return nil, fmt.Errorf("%s is not a UDS bundle: %w", opts.Source, err)
	}

	// Fetch bundle definition manifest.
	defBytes, err := udsoci.FetchBytes(ctx, repo, defEntry)
	if err != nil {
		return nil, fmt.Errorf("fetching bundle definition manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(defBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	// Push new defaults blob.
	defaultsData, err := reconfigureDefaultsData(opts)
	if err != nil {
		return nil, fmt.Errorf("reading defaults file: %w", err)
	}
	newDefaultsDesc, err := udsoci.PushBytes(ctx, repo, MediaTypeBundleHCL, defaultsData, map[string]string{ocispec.AnnotationTitle: BundleDefaultsFileName})
	if err != nil {
		return nil, fmt.Errorf("pushing defaults blob: %w", err)
	}

	// Fetch, splice, push HCL.
	hclLayer, err := findLayerByTitle(manifest, BundleFileName)
	if err != nil {
		return nil, err
	}
	hclBytes, err := udsoci.FetchBytes(ctx, repo, hclLayer)
	if err != nil {
		return nil, fmt.Errorf("fetching HCL blob: %w", err)
	}

	splicedHCL, err := spliceHCLName(hclBytes, opts.Suffix)
	if err != nil {
		return nil, fmt.Errorf("updating bundle name: %w", err)
	}

	newHCLDesc, err := udsoci.PushBytes(ctx, repo, MediaTypeBundleHCL, splicedHCL, map[string]string{ocispec.AnnotationTitle: BundleFileName})
	if err != nil {
		return nil, fmt.Errorf("pushing HCL blob: %w", err)
	}

	// Rebuild and push manifest.
	newManifestBytes, err := rebuildDefinitionManifest(manifest, newDefaultsDesc, newHCLDesc, sourceChild.Digest.String())
	if err != nil {
		return nil, fmt.Errorf("rebuilding manifest: %w", err)
	}
	newManifestOCIDesc, err := udsoci.PushManifestBytes(ctx, repo, ocispec.MediaTypeImageManifest, MediaTypeBundleDefinition, newManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("pushing manifest: %w", err)
	}

	// Rebuild the child index (re-sorted for determinism; artifactType and the
	// architecture annotation carry over) and push it by digest.
	idx.Manifests[defPos] = newManifestOCIDesc
	udsoci.SortDescriptors(idx.Manifests)
	newIndexBytes, err := json.Marshal(idx)
	if err != nil {
		return nil, fmt.Errorf("marshaling index: %w", err)
	}

	newIndexDesc := udsoci.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, newIndexBytes)

	if err := udsoci.PushDescriptorBytes(ctx, repo, newIndexDesc, newIndexBytes); err != nil {
		return nil, fmt.Errorf("pushing index: %w", err)
	}

	// Publish the target tag as a root index wrapping the new child (ADR-0015).
	arch := idx.Annotations[AnnotationBundleArchitecture]
	if arch == "" {
		return nil, fmt.Errorf("%s does not record its architecture: index is missing the %s annotation", opts.Source, AnnotationBundleArchitecture)
	}
	newIndexDesc = udsoci.BundleChildDescriptor(newIndexDesc, arch)
	if err := udsoci.PublishBundleRootIndex(ctx, repo, targetTag, newIndexDesc); err != nil {
		return nil, err
	}

	r.streams.Info("bundle reconfigured", "ref", targetRef)
	return &ReconfigureResult{OCIReference: targetRef}, nil
}

func reconfigureDefaultsData(opts ReconfigureOptions) ([]byte, error) {
	if opts.materializedDefaults != nil {
		return opts.materializedDefaults, nil
	}
	return bundleinternal.MaterializeDefaultsFile(opts.DefaultsFile)
}

// AnnotationReconfiguredFrom is the OCI manifest annotation that records
// the digest of the original bundle's canonical child index.
const AnnotationReconfiguredFrom = udsoci.AnnotationReconfiguredFrom

// spliceHCLName uses the HCL AST to locate the metadata.name attribute and appends
// the suffix to its value via in-byte replacement. This preserves all original
// formatting, comments, template expressions, and whitespace.
//
// For quoted strings ("my-bundle"), the suffix is inserted before the closing quote.
// For bare references (local.name), the expression is wrapped into a template
// string ("${local.name}<suffix>").
func spliceHCLName(hclBytes []byte, suffix string) ([]byte, error) {
	file, diags := hclsyntax.ParseConfig(hclBytes, BundleFileName, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse bundle HCL: %s", diags.Error())
	}

	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("unexpected HCL body type")
	}

	for _, block := range body.Blocks {
		if block.Type != "metadata" {
			continue
		}

		nameAttr, exists := block.Body.Attributes["name"]
		if !exists {
			continue
		}

		exprRange := nameAttr.Expr.Range()
		startByte := exprRange.Start.Byte
		endByte := exprRange.End.Byte
		if endByte <= startByte {
			return nil, fmt.Errorf("metadata.name has an empty expression range")
		}

		var result []byte
		if hclBytes[endByte-1] == '"' {
			// Quoted string: insert suffix before closing quote.
			insertAt := endByte - 1
			result = make([]byte, 0, len(hclBytes)+len(suffix))
			result = append(result, hclBytes[:insertAt]...)
			result = append(result, []byte(suffix)...)
			result = append(result, hclBytes[insertAt:]...)
		} else {
			// Bare reference: wrap as "${<expr>}<suffix>".
			exprText := hclBytes[startByte:endByte]
			replacement := fmt.Sprintf(`"${%s}%s"`, exprText, suffix)
			result = make([]byte, 0, len(hclBytes)+len(replacement)-len(exprText))
			result = append(result, hclBytes[:startByte]...)
			result = append(result, []byte(replacement)...)
			result = append(result, hclBytes[endByte:]...)
		}

		return result, nil
	}

	return nil, fmt.Errorf("metadata.name attribute not found in bundle HCL")
}

// rebuildDefinitionManifest replaces the HCL and defaults layers in the bundle
// definition manifest, adds provenance annotation, and returns serialized JSON.
func rebuildDefinitionManifest(original ocispec.Manifest, newDefaultsDesc, newHCLDesc ocispec.Descriptor, sourceArtifactDigest string) ([]byte, error) {
	var layers []ocispec.Descriptor
	defaultsReplaced := false
	hclReplaced := false

	for _, l := range original.Layers {
		title := l.Annotations[ocispec.AnnotationTitle]
		switch title {
		case BundleFileName:
			layers = append(layers, newHCLDesc)
			hclReplaced = true
		case BundleDefaultsFileName:
			layers = append(layers, newDefaultsDesc)
			defaultsReplaced = true
		default:
			layers = append(layers, l)
		}
	}

	if !hclReplaced {
		return nil, fmt.Errorf("bundle.uds.hcl layer not found in manifest")
	}

	// Insert defaults after HCL layer if original had none.
	if !defaultsReplaced {
		layers = slices.Insert(layers, 1, newDefaultsDesc)
	}

	// Copy existing annotations and add/overwrite reconfigure-specific ones.
	annotations := make(map[string]string)
	for k, v := range original.Annotations {
		annotations[k] = v
	}
	// Pin to epoch for reproducible manifest digests, matching the create path.
	annotations[ocispec.AnnotationCreated] = "1970-01-01T00:00:00Z"
	annotations[AnnotationReconfiguredFrom] = sourceArtifactDigest

	manifest := ocispec.Manifest{
		Versioned:    original.Versioned,
		MediaType:    original.MediaType,
		ArtifactType: original.ArtifactType,
		Config:       original.Config,
		Layers:       layers,
		Annotations:  annotations,
	}

	return json.Marshal(manifest)
}

func findLayerByTitle(manifest ocispec.Manifest, title string) (ocispec.Descriptor, error) {
	for _, layer := range manifest.Layers {
		if layer.Annotations[ocispec.AnnotationTitle] == title {
			return layer, nil
		}
	}
	return ocispec.Descriptor{}, fmt.Errorf("%s layer not found in manifest", title)
}

// reconfiguredFileOutputName inserts the suffix before the arch/version tokens so the
// filename matches bundleOutputName's "uds-bundle-<name>-<arch>-<version>.tar.zst"
// convention. Falls back to appending if no known arch token is found.
func reconfiguredFileOutputName(sourceBaseName, suffix string) string {
	stem := strings.TrimSuffix(sourceBaseName, ".tar.zst")
	for _, arch := range []string{"amd64", "arm64"} {
		token := "-" + arch + "-"
		if idx := strings.LastIndex(stem, token); idx >= 0 {
			return stem[:idx] + suffix + stem[idx:] + ".tar.zst"
		}
	}
	return stem + suffix + ".tar.zst"
}
