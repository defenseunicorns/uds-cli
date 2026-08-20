// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
)

// ReconfigureOptions holds configuration for the bundle reconfigure operation.
type ReconfigureOptions struct {
	// Suffix is appended to the output artifact name.
	Suffix string
	// OutputDir is used for local tarball output and must be empty for OCI sources.
	OutputDir string
	// Config provides shared configuration for the operation.
	Config *UDSBundleConfig
	// Signing controls the signature of the reconfigured output artifact.
	Signing SigningOptions
	// Verification controls verification of the source artifact.
	Verification VerificationPolicy
	// SkipSignatureVerification disables source signature verification.
	SkipSignatureVerification bool
	// Streams carries In, Out, and ErrOut for the operation.
	Streams iostreams.IOStreams
}

// ReconfigureResult represents the output of a bundle reconfigure operation.
type ReconfigureResult struct {
	OutputPath   string `json:"outputPath,omitempty" yaml:"outputPath,omitempty" text:"Output Path,omitempty"`
	OCIReference string `json:"ociReference,omitempty" yaml:"ociReference,omitempty" text:"OCI Reference,omitempty"`
}

// Reconfigure validates the defaults file and dispatches to the appropriate
// implementation based on whether the source is a local tarball or OCI reference.
func Reconfigure(ctx context.Context, source, defaultsFile string, opts ReconfigureOptions) (*ReconfigureResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if source == "" {
		return nil, fmt.Errorf("source is required: %w", ErrSourceRequired)
	}
	if defaultsFile == "" {
		return nil, fmt.Errorf("defaults file is required: %w", ErrDefaultsFileRequired)
	}
	if udsoci.IsOCIReference(source) {
		if err := validateOCIReference(source); err != nil {
			return nil, fmt.Errorf("%w %q with defaults %q: %w", ErrReconfigureBundle, source, defaultsFile, err)
		}
	}
	s := logger.Bind(opts.Streams, opts.Config.Options.LogLevel)
	s.Info("reading bundle defaults", "source", defaultsFile)
	defaultsData, err := bundleinternal.MaterializeDefaultsFile(defaultsFile)
	if err != nil {
		return nil, fmt.Errorf("%w: reading defaults file: %w", ErrReconfigureBundle, err)
	}
	state := &reconfigureState{}
	if !opts.SkipSignatureVerification {
		policy := opts.Verification
		if !policy.configured() && opts.Config.SignatureVerification != nil {
			policy = *opts.Config.SignatureVerification
		}
		if err := policy.Validate(); err != nil {
			return nil, err
		}
		if udsoci.IsOCIReference(source) {
			if err := resolveReconfigureOCIInput(ctx, source, opts, state); err != nil {
				return nil, fmt.Errorf("%w %q with defaults %q: %w", ErrReconfigureBundle, source, defaultsFile, err)
			}
		} else {
			cleanup, err := stageReconfigureLocalInput(opts.Config.Options.TmpDir, source, state)
			if err != nil {
				return nil, fmt.Errorf("%w %q with defaults %q: %w", ErrReconfigureBundle, source, defaultsFile, err)
			}
			defer cleanup()
		}
		if err := Verify(ctx, VerifyOptions{
			Source:  state.inputSource,
			Policy:  policy,
			Config:  opts.Config,
			TmpDir:  opts.Config.Options.TmpDir,
			Streams: opts.Streams,
		}); err != nil {
			return nil, fmt.Errorf("%w: verifying input bundle: %w", ErrReconfigureBundle, err)
		}
	} else {
		warnSkippedSignatureVerification(opts.Streams)
	}

	if udsoci.IsOCIReference(source) {
		reconfigurer := &ociReconfigurer{streams: s, remoteRepo: state.remoteRepo, sourceChild: state.sourceChild, sourceIndex: state.sourceIndex}
		result, err := reconfigurer.reconfigureOCIArtifact(ctx, source, defaultsData, opts)
		if err != nil {
			return result, fmt.Errorf("%w %q with defaults %q: %w", ErrReconfigureBundle, source, defaultsFile, err)
		}
		if opts.Signing.Mode == "" || opts.Signing.Mode == SigningModeUnsigned {
			s.Warn("reconfigured bundle is unsigned; its integrity and origin are not established")
		}
		return result, nil
	}
	inputSource := source
	if state.inputSource != "" {
		inputSource = state.inputSource
	}
	result, err := reconfigureLocalArtifact(ctx, s, inputSource, defaultsData, opts)
	if err != nil {
		return result, fmt.Errorf("%w %q with defaults %q: %w", ErrReconfigureBundle, source, defaultsFile, err)
	}
	if opts.Signing.Mode == "" || opts.Signing.Mode == SigningModeUnsigned {
		s.Warn("reconfigured bundle is unsigned; its integrity and origin are not established")
		return result, nil
	}
	if err := Sign(ctx, SignOptions{Source: result.OutputPath, Signing: opts.Signing, Config: opts.Config, TmpDir: opts.Config.Options.TmpDir, Streams: s}); err != nil {
		_ = os.Remove(result.OutputPath)
		return nil, fmt.Errorf("%w: signing reconfigured bundle: %w", ErrReconfigureBundle, err)
	}
	return result, nil
}

type reconfigureState struct {
	inputSource string
	remoteRepo  oras.Target
	sourceChild ocispec.Descriptor
	sourceIndex []byte
}

func stageReconfigureLocalInput(tmpDir, source string, state *reconfigureState) (func(), error) {
	workspace, err := os.MkdirTemp(tmpDir, "uds-bundle-reconfigure-input-*")
	if err != nil {
		return nil, fmt.Errorf("creating input workspace: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(workspace) }

	input, err := os.Open(source)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("opening input bundle: %w", err)
	}
	defer func() { _ = input.Close() }()

	stagedPath := filepath.Join(workspace, filepath.Base(source))
	output, err := os.OpenFile(stagedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("creating staged input bundle: %w", err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		cleanup()
		return nil, fmt.Errorf("staging input bundle: %w", copyErr)
	}
	if closeErr != nil {
		cleanup()
		return nil, fmt.Errorf("closing staged input bundle: %w", closeErr)
	}
	state.inputSource = stagedPath
	return cleanup, nil
}

func resolveReconfigureOCIInput(ctx context.Context, source string, opts ReconfigureOptions, state *reconfigureState) error {
	trimmed := udsoci.TrimScheme(source)
	ref, err := name.ParseReference(trimmed)
	if err != nil {
		return fmt.Errorf("parsing OCI reference: %w", err)
	}
	if _, isDigest := ref.(name.Digest); isDigest {
		return fmt.Errorf("OCI source must use a tag reference (e.g. :v1.0.0), not a digest")
	}
	repo, err := udsoci.NewRemoteRepository(ctx, trimmed, toInternalConfigOptions(*opts.Config.Options))
	if err != nil {
		return fmt.Errorf("connecting to registry: %w", err)
	}
	child, index, err := udsoci.ResolveBundleChild(ctx, repo, ref.Identifier(), opts.Config.Options.Architecture)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", source, err)
	}
	state.remoteRepo = repo
	state.sourceChild = child
	state.sourceIndex = index
	state.inputSource = "oci://" + ref.Context().String() + "@" + child.Digest.String()
	return nil
}

func reconfigureLocalArtifact(ctx context.Context, streams iostreams.IOStreams, source string, defaultsData []byte, opts ReconfigureOptions) (*ReconfigureResult, error) {
	// Compute output filename and check it doesn't exist.
	baseName := filepath.Base(source)
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
	tmp, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-bundle-reconfigure-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(tmp); rmErr != nil {
			streams.Warn("failed to remove temp dir", "path", tmp, "error", rmErr)
		}
	}()

	if err := artifact.ExtractTarZst(ctx, streams, source, tmp); err != nil {
		return nil, fmt.Errorf("extracting bundle: %w", err)
	}
	if err := os.Remove(filepath.Join(tmp, bundleSignatureFileName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("removing inherited bundle signature evidence: %w", err)
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
		return nil, fmt.Errorf("%s does not appear to be a UDS bundle: index does not declare artifactType %s", source, udsoci.MediaTypeBundle)
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
		return nil, fmt.Errorf("%s does not appear to be a UDS bundle: %w", source, err)
	}
	if err := validateReconfigurePackageIdentities(idx, defPos); err != nil {
		return nil, err
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

	hclLayer, err := findLayerByTitle(manifest, bundleFileName)
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
	newDefaultsDesc, err := store.PushBytes(ctx, udsoci.MediaTypeBundleHCL, defaultsData)
	if err != nil {
		return nil, fmt.Errorf("writing defaults blob: %w", err)
	}
	newDefaultsDesc.Annotations = map[string]string{ocispec.AnnotationTitle: bundleDefaultsFileName}

	splicedHCL, err := spliceHCLName(hclBytes, opts.Suffix)
	if err != nil {
		return nil, fmt.Errorf("updating bundle name: %w", err)
	}
	newHCLDesc, err := store.PushBytes(ctx, udsoci.MediaTypeBundleHCL, splicedHCL)
	if err != nil {
		return nil, fmt.Errorf("writing updated HCL blob: %w", err)
	}
	newHCLDesc.Annotations = map[string]string{ocispec.AnnotationTitle: bundleFileName}

	// Rebuild the bundle definition manifest.
	newManifestBytes, err := rebuildDefinitionManifest(manifest, newDefaultsDesc, newHCLDesc, sourceArtifactDigest)
	if err != nil {
		return nil, fmt.Errorf("rebuilding manifest: %w", err)
	}
	newManifestDesc, err := udsoci.PushManifestBytes(ctx, store, ocispec.MediaTypeImageManifest, udsoci.MediaTypeBundleDefinition, newManifestBytes)
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
	if err := store.PruneUnreferencedBlobs(ctx, streams, idx.Manifests); err != nil {
		return nil, fmt.Errorf("cleaning unreferenced blobs: %w", err)
	}

	// Repackage as tar.zst.
	if err := artifact.WriteTarZst(ctx, streams, outPath, tmp); err != nil {
		return nil, fmt.Errorf("writing output archive: %w", err)
	}

	streams.Info("bundle reconfigured", "output", outPath)
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

func validateReconfigurePackageIdentities(idx ocispec.Index, defIdx int) error {
	for i, desc := range idx.Manifests {
		if i == defIdx {
			continue
		}
		if desc.Annotations[udsoci.AnnotationPackageName] == "" {
			return fmt.Errorf("package manifest at index %d has no %s annotation; recreate the bundle with package-name identity", i, udsoci.AnnotationPackageName)
		}
	}
	return nil
}

type ociReconfigurer struct {
	streams     iostreams.IOStreams
	remoteRepo  oras.Target
	sourceChild ocispec.Descriptor
	sourceIndex []byte
}

func (r *ociReconfigurer) reconfigureOCIArtifact(ctx context.Context, source string, defaultsData []byte, opts ReconfigureOptions) (*ReconfigureResult, error) {
	if opts.OutputDir != "" {
		return nil, fmt.Errorf("--output-dir is not supported for OCI sources")
	}

	// Compute the source tag and derivative target tag.
	trimmed := udsoci.TrimScheme(source)
	sourceTag, targetTag, targetRef, err := udsoci.TaggedDerivativeReference(trimmed, opts.Suffix)
	if err != nil {
		return nil, err
	}

	// Get registry target.
	repo := r.remoteRepo
	if repo == nil {
		remote, err := udsoci.NewRemoteRepository(ctx, trimmed, toInternalConfigOptions(*opts.Config.Options))
		if err != nil {
			return nil, fmt.Errorf("connecting to registry: %w", err)
		}
		repo = remote
	}

	// Resolve source to the canonical single-arch bundle (child) index,
	// platform-selecting from the root index when the tag points at one.
	sourceChild := r.sourceChild
	indexBytes := r.sourceIndex
	if sourceChild.Digest == "" {
		sourceChild, indexBytes, err = udsoci.ResolveBundleChild(ctx, repo, sourceTag, opts.Config.Options.Architecture)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", source, err)
		}
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
		return nil, fmt.Errorf("%s is not a UDS bundle: %w", source, err)
	}
	if err := validateReconfigurePackageIdentities(idx, defPos); err != nil {
		return nil, err
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
	newDefaultsDesc, err := udsoci.PushBytes(ctx, repo, udsoci.MediaTypeBundleHCL, defaultsData, map[string]string{ocispec.AnnotationTitle: bundleDefaultsFileName})
	if err != nil {
		return nil, fmt.Errorf("pushing defaults blob: %w", err)
	}

	// Fetch, splice, push HCL.
	hclLayer, err := findLayerByTitle(manifest, bundleFileName)
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

	newHCLDesc, err := udsoci.PushBytes(ctx, repo, udsoci.MediaTypeBundleHCL, splicedHCL, map[string]string{ocispec.AnnotationTitle: bundleFileName})
	if err != nil {
		return nil, fmt.Errorf("pushing HCL blob: %w", err)
	}

	// Rebuild and push manifest.
	newManifestBytes, err := rebuildDefinitionManifest(manifest, newDefaultsDesc, newHCLDesc, sourceChild.Digest.String())
	if err != nil {
		return nil, fmt.Errorf("rebuilding manifest: %w", err)
	}
	newManifestOCIDesc, err := udsoci.PushManifestBytes(ctx, repo, ocispec.MediaTypeImageManifest, udsoci.MediaTypeBundleDefinition, newManifestBytes)
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
	if opts.Signing.Mode != "" && opts.Signing.Mode != SigningModeUnsigned {
		workspace, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-bundle-oci-reconfigure-sign-*")
		if err != nil {
			return nil, fmt.Errorf("creating signing workspace: %w", err)
		}
		defer func() { _ = os.RemoveAll(workspace) }()
		indexPath := filepath.Join(workspace, "index.json")
		evidencePath := filepath.Join(workspace, bundleSignatureFileName)
		if err := os.WriteFile(indexPath, newIndexBytes, 0o600); err != nil {
			return nil, fmt.Errorf("writing bundle index for signing: %w", err)
		}
		if err := signBundleIndex(ctx, indexPath, evidencePath, opts.Signing); err != nil {
			return nil, fmt.Errorf("signing reconfigured bundle: %w", err)
		}
		evidence, err := os.ReadFile(evidencePath)
		if err != nil {
			return nil, fmt.Errorf("reading bundle signature evidence: %w", err)
		}
		if err := udsoci.PublishBundleSignature(ctx, repo, newIndexDesc, evidence, opts.Signing.Overwrite); err != nil {
			return nil, fmt.Errorf("publishing reconfigured bundle signature: %w", err)
		}
	}

	// Publish the target tag as a root index wrapping the new child (ADR-0015).
	arch := idx.Annotations[udsoci.AnnotationBundleArchitecture]
	if arch == "" {
		return nil, fmt.Errorf("%s does not record its architecture: index is missing the %s annotation", source, udsoci.AnnotationBundleArchitecture)
	}
	newIndexDesc = udsoci.BundleChildDescriptor(newIndexDesc, arch)
	if err := udsoci.PublishBundleRootIndex(ctx, repo, targetTag, newIndexDesc); err != nil {
		return nil, err
	}

	r.streams.Info("bundle reconfigured", "ref", targetRef)
	return &ReconfigureResult{OCIReference: targetRef}, nil
}

// spliceHCLName uses the HCL AST to locate the metadata.name attribute and appends
// the suffix to its value via in-byte replacement. This preserves all original
// formatting, comments, template expressions, and whitespace.
//
// For quoted strings ("my-bundle"), the suffix is inserted before the closing quote.
// For bare references (local.name), the expression is wrapped into a template
// string ("${local.name}<suffix>").
func spliceHCLName(hclBytes []byte, suffix string) ([]byte, error) {
	file, diags := hclsyntax.ParseConfig(hclBytes, bundleFileName, hcl.Pos{Line: 1, Column: 1})
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
		case bundleFileName:
			layers = append(layers, newHCLDesc)
			hclReplaced = true
		case bundleDefaultsFileName:
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
	maps.Copy(annotations, original.Annotations)
	// Pin to epoch for reproducible manifest digests, matching the create path.
	annotations[ocispec.AnnotationCreated] = "1970-01-01T00:00:00Z"
	annotations[udsoci.AnnotationReconfiguredFrom] = sourceArtifactDigest

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
