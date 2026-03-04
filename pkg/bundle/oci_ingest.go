// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
)

func ingestRemoteReference(ctx context.Context, blobDir, refName string, reg RegistryOptions) ([]ociManifest, error) {
	slog.Debug("ingesting remote reference", "ref", refName, "arch", reg.Arch)

	// Build name.Option slice for reference parsing.
	// name.Insecure is a toggle (not bool-accepting), so only include when true.
	var nameOpts []name.Option
	if reg.PlainHTTP {
		nameOpts = append(nameOpts, name.Insecure)
	}

	ref, err := name.ParseReference(refName, nameOpts...)
	if err != nil {
		return nil, err
	}

	// Build remote.Option slice — always configure TLS transport with the requested setting
	opts := remoteOpts(ctx, reg.SkipTLSVerify)

	desc, err := remote.Get(ref, opts...)
	if err != nil {
		return nil, err
	}
	if desc.MediaType.IsSchema1() {
		return nil, fmt.Errorf("unsupported image manifest media type %q", desc.MediaType)
	}

	if desc.MediaType.IsImage() {
		img, err := desc.Image()
		if err != nil {
			return nil, err
		}
		m, err := ingestRemoteImage(blobDir, img, string(desc.MediaType), nil, reg.Concurrency)
		if err != nil {
			return nil, err
		}
		return []ociManifest{m}, nil
	}
	if !desc.MediaType.IsIndex() {
		return nil, fmt.Errorf("unsupported OCI artifact media type %q", desc.MediaType)
	}
	idx, err := desc.ImageIndex()
	if err != nil {
		return nil, err
	}
	idxManifest, err := idx.IndexManifest()
	if err != nil {
		return nil, err
	}

	filtered := filterDescriptorsByArch(idxManifest.Manifests, reg.Arch)
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no manifests found matching architecture %q in %q", reg.Arch, refName)
	}

	manifests := make([]ociManifest, len(filtered))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(reg.Concurrency)
	for i, mDesc := range filtered {
		g.Go(func() error {
			digestRef := ref.Context().Name() + "@" + mDesc.Digest.String()
			imgRef, err := name.NewDigest(digestRef, nameOpts...)
			if err != nil {
				return err
			}
			img, err := remote.Image(imgRef, remoteOpts(gctx, reg.SkipTLSVerify)...)
			if err != nil {
				return err
			}
			m, err := ingestRemoteImage(blobDir, img, string(mDesc.MediaType), mDesc.Platform, reg.Concurrency)
			if err != nil {
				return err
			}
			if m.Annotations == nil {
				m.Annotations = map[string]string{}
			}
			m.Annotations["org.opencontainers.image.ref.name"] = digestRef
			manifests[i] = m
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return manifests, nil
}

// remoteOpts builds a common set of remote.Option for go-containerregistry calls,
// always configuring TLS transport with the requested insecure setting.
func remoteOpts(ctx context.Context, skipTLSVerify bool) []remote.Option {
	// Clone the default transport to preserve proxy config, timeouts,
	// keep-alives, and HTTP/2 support while customizing TLS behavior.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: skipTLSVerify} //nolint:gosec // user-controlled via --skip-tls-verify

	opts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithTransport(transport),
	}
	return opts
}

func ingestRemoteImage(blobDir string, img v1.Image, mediaType string, platform *v1.Platform, concurrency int) (ociManifest, error) {
	manifestHash, err := img.Digest()
	if err != nil {
		return ociManifest{}, err
	}
	rawManifest, err := img.RawManifest()
	if err != nil {
		return ociManifest{}, err
	}
	if err := writeBlobBytesIfMissingAndVerify(blobDir, manifestHash, rawManifest); err != nil {
		return ociManifest{}, err
	}

	configHash, err := img.ConfigName()
	if err != nil {
		return ociManifest{}, err
	}
	rawConfig, err := img.RawConfigFile()
	if err != nil {
		return ociManifest{}, err
	}
	if err := writeBlobBytesIfMissingAndVerify(blobDir, configHash, rawConfig); err != nil {
		return ociManifest{}, err
	}

	layers, err := img.Layers()
	if err != nil {
		return ociManifest{}, err
	}
	lg := new(errgroup.Group)
	lg.SetLimit(concurrency)
	for _, layer := range layers {
		lg.Go(func() error {
			layerHash, err := layer.Digest()
			if err != nil {
				return err
			}
			return writeBlobLayerIfMissingAndVerify(blobDir, layerHash, layer)
		})
	}
	if err := lg.Wait(); err != nil {
		return ociManifest{}, err
	}

	if mediaType == "" {
		mediaType = "application/vnd.oci.image.manifest.v1+json"
	}
	var specPlatform *specv1.Platform
	if platform != nil {
		specPlatform = &specv1.Platform{
			Architecture: platform.Architecture,
			OS:           platform.OS,
			OSVersion:    platform.OSVersion,
			OSFeatures:   platform.OSFeatures,
			Variant:      platform.Variant,
		}
	}
	return ociManifest{
		MediaType: mediaType,
		Digest:    manifestHash.String(),
		Size:      int64(len(rawManifest)),
		Platform:  specPlatform,
	}, nil
}

func writeBlobBytesIfMissingAndVerify(blobDir string, hash v1.Hash, b []byte) error {
	if hash.Algorithm != "sha256" {
		return fmt.Errorf("unsupported digest algorithm %q", hash.Algorithm)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != hash.Hex {
		return fmt.Errorf("digest mismatch for %s", hash.String())
	}
	dst := filepath.Join(blobDir, hash.Hex)
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := f.Write(b)
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(dst)
		return writeErr
	}
	return closeErr
}

func writeBlobLayerIfMissingAndVerify(blobDir string, hash v1.Hash, layer v1.Layer) error {
	if hash.Algorithm != "sha256" {
		return fmt.Errorf("unsupported digest algorithm %q", hash.Algorithm)
	}
	dst := filepath.Join(blobDir, hash.Hex)
	if _, err := os.Stat(dst); err == nil {
		return nil
	}

	r, err := layer.Compressed()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	tmp, err := os.CreateTemp(blobDir, ".blob-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(tmp, h), r)
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	if hex.EncodeToString(h.Sum(nil)) != hash.Hex {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("digest mismatch for %s", hash.String())
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func ingestLocalReference(ctx context.Context, blobDir, bundleDir, src, arch string) ([]ociManifest, error) {
	slog.Debug("ingesting local reference", "source", src, "arch", arch)
	path := src
	if !filepath.IsAbs(path) {
		path = filepath.Join(bundleDir, src)
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	layoutRoot := path
	cleanup := func() {}
	if !st.IsDir() {
		tmp, err := os.MkdirTemp("", "uds-local-oci-*")
		if err != nil {
			return nil, err
		}
		cleanup = func() { _ = os.RemoveAll(tmp) }

		if strings.HasSuffix(path, ".tar.zst") {
			if err := extractTarZst(ctx, path, tmp); err != nil {
				cleanup()
				return nil, err
			}
			layoutRoot = tmp
		} else {
			cleanup()
			return nil, fmt.Errorf("unsupported local package source %q", src)
		}
	}
	defer cleanup()

	// Check if this is a Zarf package (has zarf.yaml at root)
	// If so, convert it to OCI layer format before ingesting
	if isZarfPackage(layoutRoot) {
		return ingestZarfPackage(ctx, blobDir, layoutRoot, arch)
	}

	root, err := findOCILayoutRoot(layoutRoot)
	if err != nil {
		return nil, err
	}

	idxBytes, err := os.ReadFile(filepath.Join(root, "index.json"))
	if err != nil {
		return nil, err
	}
	var srcIndex ociIndex
	if err := json.Unmarshal(idxBytes, &srcIndex); err != nil {
		return nil, err
	}
	if len(srcIndex.Manifests) == 0 {
		return nil, fmt.Errorf("no manifests found in index.json")
	}

	srcBlobDir := filepath.Join(root, "blobs", "sha256")

	matched := filterOCIManifestsByArch(srcIndex.Manifests, arch)
	if len(matched) == 0 {
		return nil, fmt.Errorf("no manifests found matching architecture %q in %q", arch, src)
	}

	if err := copyRequiredBlobsFromLayout(blobDir, srcBlobDir, matched); err != nil {
		return nil, err
	}

	for i := range matched {
		if matched[i].MediaType == "" {
			matched[i].MediaType = "application/vnd.oci.image.manifest.v1+json"
		}
	}

	return matched, nil
}

// filterDescriptorsByArch returns only the descriptors that match the given
// target architecture. Descriptors with no platform info (nil or empty arch)
// are always included. Returns descs unchanged when arch is empty.
func filterDescriptorsByArch(descs []v1.Descriptor, arch string) []v1.Descriptor {
	if arch == "" {
		return descs
	}
	var filtered []v1.Descriptor
	for _, d := range descs {
		if d.Platform == nil || d.Platform.Architecture == "" || d.Platform.Architecture == arch {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// filterOCIManifestsByArch returns only the manifests that match the given
// target architecture. Manifests with no platform info are always included.
// Returns manifests unchanged when arch is empty.
func filterOCIManifestsByArch(manifests []ociManifest, arch string) []ociManifest {
	if arch == "" {
		return manifests
	}
	var filtered []ociManifest
	for _, m := range manifests {
		if m.Platform == nil || m.Platform.Architecture == "" || m.Platform.Architecture == arch {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// gcUnreferencedBlobs removes blobs from blobDir that are not referenced by
// any of the provided manifests (their manifest blob, config blob, and layer blobs).
// Call this after all optional-component filtering is complete so that excluded
// component layers are not shipped in the final bundle.
func gcUnreferencedBlobs(blobDir string, manifests []ociManifest) error {
	slog.Debug("garbage collecting unreferenced blobs", "manifests", len(manifests))
	keep := make(map[string]bool)
	for _, m := range manifests {
		mh, err := v1.NewHash(m.Digest)
		if err != nil {
			return err
		}
		keep[mh.Hex] = true

		data, err := os.ReadFile(filepath.Join(blobDir, mh.Hex))
		if err != nil {
			return err
		}
		var im ociImageManifest
		if err := json.Unmarshal(data, &im); err != nil {
			return err
		}
		if im.Config.Digest != "" {
			if ch, err := v1.NewHash(im.Config.Digest); err == nil {
				keep[ch.Hex] = true
			}
		}
		for _, l := range im.Layers {
			if lh, err := v1.NewHash(l.Digest); err == nil {
				keep[lh.Hex] = true
			}
		}
	}

	entries, err := os.ReadDir(blobDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if !keep[e.Name()] {
			_ = os.Remove(filepath.Join(blobDir, e.Name()))
		}
	}
	return nil
}

func copyRequiredBlobsFromLayout(dstBlobDir, srcBlobDir string, manifests []ociManifest) error {
	for _, m := range manifests {
		mh, err := v1.NewHash(m.Digest)
		if err != nil {
			return err
		}
		if mh.Algorithm != "sha256" {
			return fmt.Errorf("unsupported digest algorithm %q", mh.Algorithm)
		}

		manifestPath := filepath.Join(srcBlobDir, mh.Hex)
		st, err := os.Stat(manifestPath)
		if err != nil {
			return err
		}
		if m.Size > 0 && st.Size() != m.Size {
			return fmt.Errorf("manifest size mismatch for %s", m.Digest)
		}
		manifestBytes, err := os.ReadFile(manifestPath)
		if err != nil {
			return err
		}
		if err := writeBlobBytesIfMissingAndVerify(dstBlobDir, mh, manifestBytes); err != nil {
			return err
		}

		var im ociImageManifest
		if err := json.Unmarshal(manifestBytes, &im); err != nil {
			return err
		}
		if im.Config.Digest == "" {
			return fmt.Errorf("invalid image manifest %s: missing config digest", m.Digest)
		}

		if err := copyDescriptorBlobFromLayout(dstBlobDir, srcBlobDir, im.Config); err != nil {
			return err
		}
		for _, l := range im.Layers {
			if err := copyDescriptorBlobFromLayout(dstBlobDir, srcBlobDir, l); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyDescriptorBlobFromLayout(dstBlobDir, srcBlobDir string, d ociDescriptor) error {
	h, err := v1.NewHash(d.Digest)
	if err != nil {
		return err
	}
	if h.Algorithm != "sha256" {
		return fmt.Errorf("unsupported digest algorithm %q", h.Algorithm)
	}
	src := filepath.Join(srcBlobDir, h.Hex)
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if d.Size > 0 && st.Size() != d.Size {
		return fmt.Errorf("blob size mismatch for %s", d.Digest)
	}
	return copyBlobFileIfMissingAndVerify(dstBlobDir, src, h)
}

func copyBlobFileIfMissingAndVerify(dstBlobDir, srcPath string, hash v1.Hash) error {
	if hash.Algorithm != "sha256" {
		return fmt.Errorf("unsupported digest algorithm %q", hash.Algorithm)
	}
	dst := filepath.Join(dstBlobDir, hash.Hex)
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	tmp, err := os.CreateTemp(dstBlobDir, ".blob-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(tmp, h), in)
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	if hex.EncodeToString(h.Sum(nil)) != hash.Hex {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("digest mismatch for %s", hash.String())
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// writeBlobFileIfMissingAndVerify writes a file to the blob directory using streaming.
// This is used for potentially large files to avoid loading them entirely into memory.
func writeBlobFileIfMissingAndVerify(dstBlobDir, srcPath string, hash v1.Hash) error {
	return copyBlobFileIfMissingAndVerify(dstBlobDir, srcPath, hash)
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
	oci := filepath.Join(root, "oci")
	if isOCILayoutDir(oci) {
		return oci, nil
	}
	images := filepath.Join(root, "images")
	if isOCILayoutDir(images) {
		return images, nil
	}
	return "", fmt.Errorf("no OCI image layout found in %q", root)
}

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

// isZarfPackage checks if the directory is a Zarf package by looking for zarf.yaml
func isZarfPackage(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "zarf.yaml"))
	return err == nil
}

// ingestZarfPackage converts a traditional Zarf package directory to OCI layer format
// and ingests it into the bundle blob store. Each file in the Zarf package becomes
// an OCI layer with org.opencontainers.image.title annotation and file permissions.
func ingestZarfPackage(ctx context.Context, blobDir, pkgRoot, arch string) ([]ociManifest, error) {
	slog.Debug("ingesting zarf package", "root", pkgRoot, "arch", arch)
	// Parse zarf.yaml for metadata if it exists
	zarfMeta := readZarfMetadata(filepath.Join(pkgRoot, "zarf.yaml"))

	// Walk the package directory and create layers for each file
	var layers []ociDescriptor

	err := filepath.Walk(pkgRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Check for context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if info.IsDir() {
			return nil
		}

		// Skip symlinks to avoid following them and potential issues
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		// Get relative path from package root
		relPath, err := filepath.Rel(pkgRoot, path)
		if err != nil {
			return err
		}

		// Use streaming for large files to avoid OOM
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()

		h := sha256.New()
		size, err := io.Copy(h, f)
		if err != nil {
			return err
		}
		hexDigest := hex.EncodeToString(h.Sum(nil))
		digest := "sha256:" + hexDigest

		// Write blob to bundle using streaming
		if err := writeBlobFileIfMissingAndVerify(blobDir, path, v1.Hash{Algorithm: "sha256", Hex: hexDigest}); err != nil {
			return err
		}

		// Use forward slashes in annotations (OCI standard)
		title := filepath.ToSlash(relPath)

		// Store file permissions and size
		annotations := map[string]string{
			"org.opencontainers.image.title":      title,
			"org.defenseunicorns.zarf.file.mode":  fmt.Sprintf("%o", info.Mode().Perm()),
			"org.defenseunicorns.zarf.file.mtime": info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
		}

		layers = append(layers, ociDescriptor{
			MediaType:   "application/vnd.defenseunicorns.zarf.layer.v1",
			Digest:      digest,
			Size:        size,
			Annotations: annotations,
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking package directory: %w", err)
	}

	if len(layers) == 0 {
		return nil, fmt.Errorf("no files found in Zarf package")
	}

	// Create config blob with package metadata
	configData, err := json.Marshal(map[string]interface{}{
		"architecture": arch,
		"os":           "linux",
		"config": map[string]interface{}{
			"Labels": map[string]string{
				"org.defenseunicorns.zarf.name":        zarfMeta.Metadata.Name,
				"org.defenseunicorns.zarf.version":     zarfMeta.Metadata.Version,
				"org.defenseunicorns.zarf.description": zarfMeta.Metadata.Description,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}
	configHash := sha256.Sum256(configData)
	configHex := hex.EncodeToString(configHash[:])
	if err := writeBlobBytesIfMissingAndVerify(blobDir, v1.Hash{Algorithm: "sha256", Hex: configHex}, configData); err != nil {
		return nil, fmt.Errorf("writing config blob: %w", err)
	}

	// Create the image manifest
	imageManifest := ociImageManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: ociDescriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    "sha256:" + configHex,
			Size:      int64(len(configData)),
		},
		Layers: layers,
	}

	// Marshal and write manifest blob
	manifestData, err := json.Marshal(imageManifest)
	if err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}

	manifestHash := sha256.Sum256(manifestData)
	manifestHex := hex.EncodeToString(manifestHash[:])
	if err := writeBlobBytesIfMissingAndVerify(blobDir, v1.Hash{Algorithm: "sha256", Hex: manifestHex}, manifestData); err != nil {
		return nil, fmt.Errorf("writing manifest blob: %w", err)
	}

	// Return the manifest descriptor
	platform := &specv1.Platform{
		Architecture: arch,
		OS:           "linux",
	}

	return []ociManifest{{
		MediaType: "application/vnd.oci.image.manifest.v1+json",
		Digest:    "sha256:" + manifestHex,
		Size:      int64(len(manifestData)),
		Platform:  platform,
	}}, nil
}
