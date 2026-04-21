// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// findBundleDefinitionEntry locates the bundle definition manifest entry in an OCI index.
// Returns the entry, its position in the Manifests slice, and any error.
func findBundleDefinitionEntry(idx ociIndex) (*ociManifest, int, error) {
	for i := range idx.Manifests {
		if idx.Manifests[i].ArtifactType == MediaTypeBundleDefinition {
			return &idx.Manifests[i], i, nil
		}
	}
	return nil, -1, fmt.Errorf("bundle definition manifest not found in index")
}

// findLayerByTitle finds a layer descriptor in the manifest by its title annotation.
func findLayerByTitle(manifest ociImageManifest, title string) (ociDescriptor, error) {
	for _, l := range manifest.Layers {
		if l.Annotations[ocispec.AnnotationTitle] == title {
			return l, nil
		}
	}
	return ociDescriptor{}, fmt.Errorf("%s layer not found in manifest", title)
}

// isBundleIndex reports whether idx is a UDS bundle index.
func isBundleIndex(idx ociIndex) bool {
	_, _, err := findBundleDefinitionEntry(idx)
	return err == nil
}

// writeOCILayout writes the oci-layout marker file.
func writeOCILayout(path string) error {
	b, err := json.Marshal(ociLayout{ImageLayoutVersion: "1.0.0"})
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), tmpFilePerm)
}

// writeOCIIndex writes the index.json file.
func writeOCIIndex(path string, idx *ociIndex) error {
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), tmpFilePerm)
}

// findOCILayoutRoot locates the OCI image layout directory within root.
// Search order:
//  1. root itself (minimal test layouts, single-image packages)
//  2. root/oci (UDS bundles created by this CLI)
//  3. root/images (extracted Zarf package tar archives)
func findOCILayoutRoot(root string) (string, error) {
	if isOCILayoutDir(root) {
		return root, nil
	}
	ociDir := filepath.Join(root, "oci")
	if isOCILayoutDir(ociDir) {
		return ociDir, nil
	}
	images := filepath.Join(root, "images")
	if isOCILayoutDir(images) {
		return images, nil
	}
	return "", fmt.Errorf("no OCI image layout found in %q", root)
}

// isOCILayoutDir reports whether dir contains the markers of a valid OCI image
// layout: oci-layout file, index.json, and blobs/sha256/ directory.
func isOCILayoutDir(dir string) bool {
	if dir == "" {
		return false
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "oci-layout")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "index.json")); err != nil {
		return false
	}
	if st, err := os.Stat(filepath.Join(dir, "blobs", "sha256")); err != nil || !st.IsDir() {
		return false
	}
	return true
}

// verifyOCILayoutDigests walks the full digest chain of an extracted OCI image
// layout and verifies that every blob matches its declared digest and size.
// This is checksum verification, not cryptographic signature verification, which
// detects corruption/modification but does not authenticate the artifact's
// origin.
func verifyOCILayoutDigests(ociDir string) error {
	slog.Debug("verifying OCI layout digests", "dir", ociDir)

	idxBytes, err := os.ReadFile(filepath.Join(ociDir, "index.json"))
	if err != nil {
		return fmt.Errorf("reading index.json: %w", err)
	}

	var idx ociIndex
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return fmt.Errorf("parsing index.json: %w", err)
	}

	blobDir := filepath.Join(ociDir, "blobs", "sha256")

	for _, m := range idx.Manifests {
		if err := verifyManifestDigests(blobDir, m); err != nil {
			return err
		}
	}

	slog.Debug("OCI layout digest verification passed", "manifests", len(idx.Manifests))
	return nil
}

// verifyManifestDigests verifies the digest chain for a manifest descriptor: the
// blob itself, then its contents.
func verifyManifestDigests(blobDir string, m ociManifest) error {
	// Verify the manifest/index blob itself.
	manifestDesc := ociDescriptor{
		Digest: m.Digest,
		Size:   m.Size,
	}
	if err := verifyBlobDigest(blobDir, manifestDesc); err != nil {
		return fmt.Errorf("manifest %s: %w", m.Digest, err)
	}

	// Read the blob so we can verify its contents.
	d, err := parseDigest(m.Digest)
	if err != nil {
		return err
	}
	manifestBytes, err := os.ReadFile(filepath.Join(blobDir, d.Encoded()))
	if err != nil {
		return fmt.Errorf("reading manifest %s: %w", m.Digest, err)
	}

	switch m.MediaType {
	case ocispec.MediaTypeImageIndex:
		return verifyNestedIndex(blobDir, m.Digest, manifestBytes)
	case ocispec.MediaTypeImageManifest:
		return verifyImageManifest(blobDir, m.Digest, manifestBytes)
	default:
		return fmt.Errorf("unsupported manifest media type %q for %s", m.MediaType, m.Digest)
	}
}

// verifyNestedIndex parses an index blob and recursively verifies each child
// manifest entry.
func verifyNestedIndex(blobDir, digest string, indexBytes []byte) error {
	var idx ociIndex
	if err := json.Unmarshal(indexBytes, &idx); err != nil {
		return fmt.Errorf("parsing nested index %s: %w", digest, err)
	}
	for _, child := range idx.Manifests {
		if err := verifyManifestDigests(blobDir, child); err != nil {
			return fmt.Errorf("nested index %s: %w", digest, err)
		}
	}
	return nil
}

// verifyImageManifest parses an image manifest blob and verifies its config and
// layer blobs.
func verifyImageManifest(blobDir, digest string, manifestBytes []byte) error {
	var im ociImageManifest
	if err := json.Unmarshal(manifestBytes, &im); err != nil {
		return fmt.Errorf("parsing manifest %s: %w", digest, err)
	}

	if im.Config.Digest != "" {
		if err := verifyBlobDigest(blobDir, im.Config); err != nil {
			return fmt.Errorf("config in manifest %s: %w", digest, err)
		}
	}

	for i, layer := range im.Layers {
		if err := verifyBlobDigest(blobDir, layer); err != nil {
			return fmt.Errorf("layer[%d] in manifest %s: %w", i, digest, err)
		}
	}

	return nil
}

// verifyBlobDigest verifies a single blob file matches its declared digest and size.
func verifyBlobDigest(blobDir string, desc ociDescriptor) error {
	d, err := parseDigest(desc.Digest)
	if err != nil {
		return err
	}

	blobPath := filepath.Join(blobDir, d.Encoded())

	f, err := os.Open(blobPath)
	if err != nil {
		return fmt.Errorf("opening blob %s: %w", desc.Digest, err)
	}
	defer func() { _ = f.Close() }()

	// Verify declared size matches actual size.
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat blob %s: %w", desc.Digest, err)
	}
	if info.Size() != desc.Size {
		return fmt.Errorf("size mismatch for %s: expected %d, got %d", desc.Digest, desc.Size, info.Size())
	}

	// Stream blob through go-digest verifier.
	verifier := d.Verifier()
	if _, err := io.Copy(verifier, f); err != nil {
		return fmt.Errorf("hashing blob %s: %w", desc.Digest, err)
	}
	if !verifier.Verified() {
		return fmt.Errorf("digest mismatch for blob %s", desc.Digest)
	}

	return nil
}
