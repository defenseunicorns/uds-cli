// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
)

// validSuffix enforces that the suffix is safe for use in OCI tags, filenames,
// and HCL string content. Must start with '-' and contain only safe characters.
var validSuffix = regexp.MustCompile(`^-[a-zA-Z0-9._-]+$`)

// Compile-time interface checks.
var _ Reconfigurer = &localReconfigurer{}

// Reconfigure validates the defaults file and dispatches to the appropriate
// implementation based on whether the source is a local tarball or OCI reference.
func Reconfigure(ctx context.Context, opts ReconfigureOptions) (*ReconfigureResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	defaultsData, err := materializeDefaultsFile(opts.DefaultsFile)
	if err != nil {
		return nil, fmt.Errorf("reading defaults file: %w", err)
	}
	opts.materializedDefaults = defaultsData
	s := logger.Bind(opts.Streams, opts.Options.LogLevel)

	var r Reconfigurer
	if IsOCIReference(opts.Source) {
		r = &ociReconfigurer{streams: s}
	} else {
		r = &localReconfigurer{streams: s}
	}

	return r.Reconfigure(ctx, opts)
}

type localReconfigurer struct {
	streams iostreams.IOStreams
}

func (r *localReconfigurer) Reconfigure(ctx context.Context, opts ReconfigureOptions) (*ReconfigureResult, error) {
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

	if err := extractTarZst(ctx, r.streams, opts.Source, tmp); err != nil {
		return nil, fmt.Errorf("extracting bundle: %w", err)
	}

	ociDir := filepath.Join(tmp, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")

	// Parse index.json and find bundle definition manifest.
	idxBytes, err := os.ReadFile(filepath.Join(ociDir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("reading index.json: %w", err)
	}
	var idx ociIndex
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return nil, fmt.Errorf("parsing index: %w", err)
	}
	if !isBundleIndex(idx) {
		return nil, fmt.Errorf("%s does not appear to be a UDS bundle: index does not declare artifactType %s", opts.Source, MediaTypeBundle)
	}

	defEntry, defPos, err := findBundleDefinitionEntry(idx)
	if err != nil {
		return nil, fmt.Errorf("%s does not appear to be a UDS bundle: %w", opts.Source, err)
	}

	// Read and parse the bundle definition manifest.
	manifestDigest := strings.TrimPrefix(defEntry.Digest, "sha256:")
	manifestBytes, err := os.ReadFile(filepath.Join(blobDir, manifestDigest))
	if err != nil {
		return nil, fmt.Errorf("reading bundle definition manifest: %w", err)
	}
	var manifest ociImageManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parsing bundle definition manifest: %w", err)
	}

	// Write new defaults blob.
	defaultsData, err := reconfigureDefaultsData(opts)
	if err != nil {
		return nil, fmt.Errorf("reading defaults file: %w", err)
	}
	newDefaultsDigest, err := writeAndDigestBlob(blobDir, defaultsData)
	if err != nil {
		return nil, fmt.Errorf("writing defaults blob: %w", err)
	}
	newDefaultsDesc := ociDescriptor{
		MediaType:   MediaTypeBundleHCL,
		Digest:      newDefaultsDigest.String(),
		Size:        int64(len(defaultsData)),
		Annotations: map[string]string{ocispec.AnnotationTitle: BundleDefaultsFileName},
	}

	hclLayer, err := findLayerByTitle(manifest, BundleFileName)
	if err != nil {
		return nil, err
	}
	hclBytes, err := os.ReadFile(filepath.Join(blobDir, strings.TrimPrefix(hclLayer.Digest, "sha256:")))
	if err != nil {
		return nil, fmt.Errorf("reading bundle HCL: %w", err)
	}
	splicedHCL, err := spliceHCLName(hclBytes, opts.Suffix)
	if err != nil {
		return nil, fmt.Errorf("updating bundle name: %w", err)
	}
	newHCLDigest, err := writeAndDigestBlob(blobDir, splicedHCL)
	if err != nil {
		return nil, fmt.Errorf("writing updated HCL blob: %w", err)
	}
	newHCLDesc := ociDescriptor{
		MediaType:   MediaTypeBundleHCL,
		Digest:      newHCLDigest.String(),
		Size:        int64(len(splicedHCL)),
		Annotations: map[string]string{ocispec.AnnotationTitle: BundleFileName},
	}

	// Rebuild the bundle definition manifest.
	newManifestBytes, err := rebuildDefinitionManifest(manifest, newDefaultsDesc, newHCLDesc, defEntry.Digest)
	if err != nil {
		return nil, fmt.Errorf("rebuilding manifest: %w", err)
	}
	newManifestDigest, err := writeAndDigestBlob(blobDir, newManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("writing manifest blob: %w", err)
	}

	// Update index.json, re-sorting by digest to preserve the deterministic
	// ordering invariant of bundle indexes (ADR-0015).
	idx.Manifests[defPos] = ociManifest{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: MediaTypeBundleDefinition,
		Digest:       newManifestDigest.String(),
		Size:         int64(len(newManifestBytes)),
	}
	sortManifestsByDigest(idx.Manifests)
	if err := writeOCIIndex(filepath.Join(ociDir, "index.json"), &idx); err != nil {
		return nil, fmt.Errorf("writing index.json: %w", err)
	}

	// Remove unreferenced blobs.
	if err := gcUnreferencedBlobs(ctx, r.streams, blobDir, idx.Manifests); err != nil {
		return nil, fmt.Errorf("cleaning unreferenced blobs: %w", err)
	}

	// Repackage as tar.zst.
	if err := writeTarZst(ctx, r.streams, outPath, tmp); err != nil {
		return nil, fmt.Errorf("writing output archive: %w", err)
	}

	r.streams.Info("bundle reconfigured", "output", outPath)
	return &ReconfigureResult{OutputPath: outPath}, nil
}

var _ Reconfigurer = &ociReconfigurer{}

type ociReconfigurer struct {
	streams iostreams.IOStreams
}

func (r *ociReconfigurer) Reconfigure(ctx context.Context, opts ReconfigureOptions) (*ReconfigureResult, error) {
	if opts.OutputDir != "" {
		return nil, fmt.Errorf("--output-dir is not supported for OCI sources")
	}

	// Parse reference and compute target tag.
	trimmed := TrimScheme(opts.Source)
	ref, err := name.ParseReference(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parsing OCI reference: %w", err)
	}
	if _, isDigest := ref.(name.Digest); isDigest {
		return nil, fmt.Errorf("OCI source must use a tag reference (e.g. :v1.0.0), not a digest")
	}
	sourceTag := ref.Identifier()
	targetTag := sourceTag + opts.Suffix
	targetRef := "oci://" + ref.Context().String() + ":" + targetTag

	// Get registry target.
	var repo oras.Target
	if opts.remoteRepo != nil {
		repo = opts.remoteRepo
	} else {
		remote, err := newRemoteRepository(ctx, trimmed, opts.Options)
		if err != nil {
			return nil, fmt.Errorf("connecting to registry: %w", err)
		}
		repo = remote
	}

	// Resolve source to the canonical single-arch bundle (child) index,
	// platform-selecting from the root index when the tag points at one.
	_, indexBytes, err := resolveBundleChild(ctx, repo, sourceTag, opts.Options.Architecture)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", opts.Source, err)
	}

	// Check target tag doesn't exist.
	if _, resolveErr := repo.Resolve(ctx, targetTag); resolveErr == nil {
		return nil, fmt.Errorf("target tag %q already exists in registry", targetTag)
	} else if !errors.Is(resolveErr, errdef.ErrNotFound) {
		return nil, fmt.Errorf("checking target tag %q: %w", targetTag, resolveErr)
	}

	var idx ociIndex
	if err := json.Unmarshal(indexBytes, &idx); err != nil {
		return nil, fmt.Errorf("parsing index: %w", err)
	}

	// Find bundle definition manifest.
	defEntry, defPos, err := findBundleDefinitionEntry(idx)
	if err != nil {
		return nil, fmt.Errorf("%s is not a UDS bundle: %w", opts.Source, err)
	}

	// Fetch bundle definition manifest (digest-verified by content.FetchAll).
	defDigest, err := godigest.Parse(defEntry.Digest)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest digest: %w", err)
	}
	defBytes, err := content.FetchAll(ctx, repo, ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    defDigest,
		Size:      defEntry.Size,
	})
	if err != nil {
		return nil, fmt.Errorf("fetching bundle definition manifest: %w", err)
	}

	var manifest ociImageManifest
	if err := json.Unmarshal(defBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	// Push new defaults blob.
	defaultsData, err := reconfigureDefaultsData(opts)
	if err != nil {
		return nil, fmt.Errorf("reading defaults file: %w", err)
	}
	newDefaultsOCIDesc := content.NewDescriptorFromBytes(MediaTypeBundleHCL, defaultsData)
	newDefaultsOCIDesc.Annotations = map[string]string{ocispec.AnnotationTitle: BundleDefaultsFileName}
	if err := repo.Push(ctx, newDefaultsOCIDesc, bytes.NewReader(defaultsData)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return nil, fmt.Errorf("pushing defaults blob: %w", err)
	}
	newDefaultsDesc := ociDescriptor{
		MediaType:   MediaTypeBundleHCL,
		Digest:      newDefaultsOCIDesc.Digest.String(),
		Size:        newDefaultsOCIDesc.Size,
		Annotations: newDefaultsOCIDesc.Annotations,
	}

	// Fetch, splice, push HCL.
	hclLayer, err := findLayerByTitle(manifest, BundleFileName)
	if err != nil {
		return nil, err
	}
	hclDigest, err := godigest.Parse(hclLayer.Digest)
	if err != nil {
		return nil, fmt.Errorf("parsing HCL digest: %w", err)
	}
	hclBytes, err := content.FetchAll(ctx, repo, ocispec.Descriptor{
		MediaType: MediaTypeBundleHCL,
		Digest:    hclDigest,
		Size:      hclLayer.Size,
	})
	if err != nil {
		return nil, fmt.Errorf("fetching HCL blob: %w", err)
	}

	splicedHCL, err := spliceHCLName(hclBytes, opts.Suffix)
	if err != nil {
		return nil, fmt.Errorf("updating bundle name: %w", err)
	}

	newHCLOCIDesc := content.NewDescriptorFromBytes(MediaTypeBundleHCL, splicedHCL)
	newHCLOCIDesc.Annotations = map[string]string{ocispec.AnnotationTitle: BundleFileName}
	if err := repo.Push(ctx, newHCLOCIDesc, bytes.NewReader(splicedHCL)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return nil, fmt.Errorf("pushing HCL blob: %w", err)
	}
	newHCLDesc := ociDescriptor{
		MediaType:   MediaTypeBundleHCL,
		Digest:      newHCLOCIDesc.Digest.String(),
		Size:        newHCLOCIDesc.Size,
		Annotations: newHCLOCIDesc.Annotations,
	}

	// Rebuild and push manifest.
	newManifestBytes, err := rebuildDefinitionManifest(manifest, newDefaultsDesc, newHCLDesc, defEntry.Digest)
	if err != nil {
		return nil, fmt.Errorf("rebuilding manifest: %w", err)
	}
	newManifestOCIDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, newManifestBytes)
	if err := repo.Push(ctx, newManifestOCIDesc, bytes.NewReader(newManifestBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return nil, fmt.Errorf("pushing manifest: %w", err)
	}

	// Rebuild the child index (re-sorted for determinism; artifactType and the
	// architecture annotation carry over) and push it by digest.
	idx.Manifests[defPos] = ociManifest{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: MediaTypeBundleDefinition,
		Digest:       newManifestOCIDesc.Digest.String(),
		Size:         newManifestOCIDesc.Size,
	}
	sortManifestsByDigest(idx.Manifests)
	newIndexBytes, err := json.Marshal(idx)
	if err != nil {
		return nil, fmt.Errorf("marshaling index: %w", err)
	}

	newIndexDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, newIndexBytes)

	if err := repo.Push(ctx, newIndexDesc, bytes.NewReader(newIndexBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return nil, fmt.Errorf("pushing index: %w", err)
	}

	// Publish the target tag as a root index wrapping the new child (ADR-0015).
	arch := idx.Annotations[AnnotationBundleArchitecture]
	if arch == "" {
		return nil, fmt.Errorf("%s does not record its architecture: index is missing the %s annotation", opts.Source, AnnotationBundleArchitecture)
	}
	newIndexDesc.ArtifactType = MediaTypeBundle
	newIndexDesc.Platform = &ocispec.Platform{Architecture: arch, OS: oci.MultiOS}

	rootBytes, rootDesc, err := mergeRootIndex(ctx, repo, targetTag, newIndexDesc)
	if err != nil {
		return nil, err
	}
	if err := repo.Push(ctx, rootDesc, bytes.NewReader(rootBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return nil, fmt.Errorf("pushing root index: %w", err)
	}
	tagger, ok := repo.(interface {
		Tag(ctx context.Context, desc ocispec.Descriptor, reference string) error
	})
	if !ok {
		return nil, fmt.Errorf("registry target does not support tagging")
	}
	if err := tagger.Tag(ctx, rootDesc, targetTag); err != nil {
		return nil, fmt.Errorf("tagging root index as %s: %w", targetTag, err)
	}

	r.streams.Info("bundle reconfigured", "ref", targetRef)
	return &ReconfigureResult{OCIReference: targetRef}, nil
}

func reconfigureDefaultsData(opts ReconfigureOptions) ([]byte, error) {
	if opts.materializedDefaults != nil {
		return opts.materializedDefaults, nil
	}
	return materializeDefaultsFile(opts.DefaultsFile)
}

// AnnotationReconfiguredFrom is the OCI manifest annotation that records
// the digest of the original bundle definition manifest.
const AnnotationReconfiguredFrom = "org.defenseunicorns.uds.reconfigured-from"

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
func rebuildDefinitionManifest(original ociImageManifest, newDefaultsDesc ociDescriptor, newHCLDesc ociDescriptor, originalManifestDigest string) ([]byte, error) {
	var layers []ociDescriptor
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
	annotations[AnnotationReconfiguredFrom] = originalManifestDigest

	manifest := ociImageManifest{
		SchemaVersion: original.SchemaVersion,
		MediaType:     original.MediaType,
		ArtifactType:  original.ArtifactType,
		Config:        original.Config,
		Layers:        layers,
		Annotations:   annotations,
	}

	return json.Marshal(manifest)
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
