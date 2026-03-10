// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	oraci "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
)

// Compile-time check: localCreator must implement Creator.
var _ Creator = &localCreator{}

// localCreator is the default Creator that ingests packages from local paths
// and OCI registries into an OCI image layout.
type localCreator struct {
	manifests []ociManifest
	arch      string
}

// ensureAnnotation adds an annotation to the manifest, initializing the map if needed.
// If the annotation key already exists, it won't be overwritten.
func ensureAnnotation(m *ociManifest, key, value string) {
	if m.Annotations == nil {
		m.Annotations = map[string]string{}
	}
	if _, exists := m.Annotations[key]; !exists {
		m.Annotations[key] = value
	}
}

// newLocalCreator returns a new localCreator instance for the given target arch.
func newLocalCreator(arch string) *localCreator {
	return &localCreator{arch: arch}
}

// CreatePackage ingests pkg into the OCI blob store at opts.BlobDir and
// accumulates the resulting ociManifest descriptors for index construction.
func (c *localCreator) CreatePackage(ctx context.Context, pkg *Package, opts CreatePackageOptions) error {
	slog.Info("ingesting package", "name", pkg.Name, "source", pkg.Source)

	var manifests []ociManifest
	var err error

	if IsOCIReference(pkg.Source) {
		// Strip any explicit scheme prefix (e.g. "oci://") before handing to
		// go-containerregistry which does not expect it.
		refName := TrimScheme(pkg.Source)
		manifests, err = ingestRemoteReference(ctx, opts.BlobDir, refName, opts.RegistryOptions)
		if err != nil {
			return fmt.Errorf("package %q: %w", pkg.Name, err)
		}
		for i := range manifests {
			ensureAnnotation(&manifests[i], "org.opencontainers.image.ref.name", refName)
		}
	} else {
		manifests, err = ingestLocalReference(ctx, opts.BlobDir, opts.BundleDir, pkg.Source, opts.Arch)
		if err != nil {
			return fmt.Errorf("package %q: %w", pkg.Name, err)
		}
		for i := range manifests {
			ensureAnnotation(&manifests[i], "org.opencontainers.image.ref.name", pkg.Name)
		}
	}

	// Filter optional components: with no explicit list, all optionals are excluded.
	for i, m := range manifests {
		filtered, err := filterZarfOptionalComponents(opts.BlobDir, m, pkg.OptionalComponents)
		if err != nil {
			return fmt.Errorf("package %q: optional component filtering: %w", pkg.Name, err)
		}
		manifests[i] = filtered
	}

	c.manifests = append(c.manifests, manifests...)
	return nil
}

// BundleName returns the output filename for the bundle artifact.
func (c *localCreator) BundleName(b *UDSBundle) string {
	return bundleOutputName(b, c.arch)
}

// Create creates a UDS bundle tar.zst from the given bundle definition file.
// It parses, validates, ingests all packages via a localCreator, and writes
// the resulting archive next to the bundle file.
func Create(ctx context.Context, opts CreateOptions) (string, error) {
	slog.Debug("parsing bundle file", "path", opts.BundleFile)
	parser := NewHCLParser()
	b, err := parser.ParseBundleFile(ctx, opts.BundleFile)
	if err != nil {
		return "", err
	}
	slog.Debug("bundle parsed", "name", b.Metadata.Name, "packages", len(b.Packages))

	if err := b.Validate(); err != nil {
		return "", err
	}
	slog.Debug("bundle validation passed")

	creator := newLocalCreator(opts.Arch)

	srcDir := filepath.Dir(opts.BundleFile)
	root, err := os.MkdirTemp(opts.TmpDir, "uds-bundle-create-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(root) }()

	ociDir := filepath.Join(root, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		return "", err
	}
	if err := writeOCILayout(filepath.Join(ociDir, "oci-layout")); err != nil {
		return "", err
	}

	pkgOpts := CreatePackageOptions{
		RegistryOptions: opts.RegistryOptions,
		BlobDir:         blobDir,
		BundleDir:       srcDir,
		Out:             opts.Out,
	}
	for i := range b.Packages {
		if err := creator.CreatePackage(ctx, &b.Packages[i], pkgOpts); err != nil {
			return "", err
		}
	}

	slog.Debug("creating bundle config manifest")
	cfgManifest, err := createBundleDefinitionManifest(ctx, ociDir, opts.BundleFile, srcDir, b.Packages)
	if err != nil {
		return "", err
	}
	allManifests := append([]ociManifest{cfgManifest}, creator.manifests...)

	slog.Debug("cleaning unreferenced blobs")
	if err := gcUnreferencedBlobs(blobDir, allManifests); err != nil {
		return "", fmt.Errorf("cleaning up unreferenced blobs: %w", err)
	}

	idx := &ociIndex{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.index.v1+json",
		Manifests:     allManifests,
	}
	if err := writeOCIIndex(filepath.Join(ociDir, "index.json"), idx); err != nil {
		return "", err
	}

	outPath := filepath.Join(srcDir, creator.BundleName(b))
	slog.Debug("writing bundle archive", "output", outPath)
	if err := writeTarZst(ctx, outPath, root); err != nil {
		return "", err
	}
	slog.Info("bundle archive written", "output", outPath)
	return outPath, nil
}

func bundleOutputName(b *UDSBundle, arch string) string {
	name := sanitizeFileComponent(b.Metadata.Name)
	if name == "" {
		name = "bundle"
	}
	arch = sanitizeFileComponent(arch)
	ver := sanitizeFileComponent(b.Metadata.Version)
	if ver == "" {
		if arch == "" {
			return fmt.Sprintf("uds-bundle-%s.tar.zst", name)
		}
		return fmt.Sprintf("uds-bundle-%s-%s.tar.zst", name, arch)
	}
	if arch == "" {
		return fmt.Sprintf("uds-bundle-%s-%s.tar.zst", name, ver)
	}
	return fmt.Sprintf("uds-bundle-%s-%s-%s.tar.zst", name, arch, ver)
}

func sanitizeFileComponent(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('-')
	}
	return strings.Trim(b.String(), "-")
}

// createBundleDefinitionManifest builds an OCI 1.1 artifact manifest that stores the bundle HCL file and all package values files as
// content-addressed layers. The manifest does not contain an "org.opencontainers.image.ref.name" annotation since it is from local
// files on disk and not from a remote registry. It is identified by artifactType so consumers can better identify it in the index.
func createBundleDefinitionManifest(ctx context.Context, ociDir, bundleFile, bundleDir string, pkgs []Package) (ociManifest, error) {
	store, err := oraci.New(ociDir)
	if err != nil {
		return ociManifest{}, fmt.Errorf("opening OCI store: %w", err)
	}
	// We write index.json ourselves at the end of Create(); prevent ORAS from
	// overwriting it on every Push/Tag call.
	store.AutoSaveIndex = false

	pushBlob := func(mediaType string, data []byte, annotations map[string]string) (ocispec.Descriptor, error) {
		desc := content.NewDescriptorFromBytes(mediaType, data)
		desc.Annotations = annotations
		if err := store.Push(ctx, desc, bytes.NewReader(data)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
			return ocispec.Descriptor{}, err
		}
		return desc, nil
	}

	// HCL file as the first layer.
	hclData, err := os.ReadFile(bundleFile)
	if err != nil {
		return ociManifest{}, fmt.Errorf("reading bundle file: %w", err)
	}
	hclDesc, err := pushBlob(MediaTypeBundleHCL, hclData, map[string]string{
		ocispec.AnnotationTitle: "bundle.uds.hcl",
	})
	if err != nil {
		return ociManifest{}, fmt.Errorf("pushing bundle HCL: %w", err)
	}

	// Values files as subsequent layers, preserving logical path in the annotation.
	layers := []ocispec.Descriptor{hclDesc}
	for _, pkg := range pkgs {
		for i, vf := range pkg.ValueFiles {
			src := vf
			if !filepath.IsAbs(src) {
				src = filepath.Join(bundleDir, vf)
			}
			st, err := os.Stat(src)
			if err != nil {
				return ociManifest{}, fmt.Errorf("package %q: cannot stat value file %q: %w", pkg.Name, vf, err)
			}
			if st.IsDir() {
				return ociManifest{}, fmt.Errorf("package %q: value file %q is a directory", pkg.Name, vf)
			}
			data, err := os.ReadFile(src)
			if err != nil {
				return ociManifest{}, fmt.Errorf("package %q: reading value file %q: %w", pkg.Name, vf, err)
			}
			valDesc, err := pushBlob(MediaTypeBundleValuesYAML, data, map[string]string{
				ocispec.AnnotationTitle: fmt.Sprintf("values/%s/%d.yaml", pkg.Name, i),
			})
			if err != nil {
				return ociManifest{}, fmt.Errorf("package %q: pushing value file: %w", pkg.Name, err)
			}
			layers = append(layers, valDesc)
		}
	}

	// PackManifest pushes the empty-JSON config blob, builds the OCI 1.1 artifact manifest with our artifactType,
	// and pushes the manifest blob.  We pin the created timestamp so the manifest digest is reproducible.
	desc, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, MediaTypeBundleDefinition, oras.PackManifestOptions{
		Layers: layers,
		ManifestAnnotations: map[string]string{
			ocispec.AnnotationCreated: "1970-01-01T00:00:00Z",
		},
	})
	if err != nil {
		return ociManifest{}, fmt.Errorf("packing bundle config manifest: %w", err)
	}

	return ociManifest{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: MediaTypeBundleDefinition,
		Digest:       desc.Digest.String(),
		Size:         desc.Size,
	}, nil
}

func writeOCILayout(path string) error {
	b, err := json.Marshal(ociLayout{ImageLayoutVersion: "1.0.0"})
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func writeOCIIndex(path string, idx *ociIndex) error {
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
