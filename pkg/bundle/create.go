// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
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
	if err := opts.Validate(); err != nil {
		return err
	}
	opts.Streams.Info("ingesting package", "name", pkg.Name, "source", pkg.Source)

	source := NewPackageSource(pkg.Source, *opts.Config.Options, opts.BundleDir, opts.Streams)

	filter := BuildComponentFilter(pkg.OptionalComponents)

	manifests, err := source.IngestFiltered(ctx, filter, opts.BlobDir)
	if err != nil {
		return fmt.Errorf("package %q: %w", pkg.Name, err)
	}

	// Set the OCI reference annotation for each manifest
	refName := pkg.Name
	if IsOCIReference(pkg.Source) {
		refName = TrimScheme(pkg.Source)
	}
	for i := range manifests {
		ensureAnnotation(&manifests[i], "org.opencontainers.image.ref.name", refName)
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
func Create(ctx context.Context, opts CreateOptions) (*CreateResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	s := logger.Bind(opts.Streams, opts.Config.Global.LogLevel)

	s.Debug("parsing bundle file", "path", opts.BundleFile)
	parser := NewHCLParser(opts.Config.Options.Architecture, s)
	b, bundleHCL, err := parser.parseAndMaterializeBundleFile(ctx, opts.BundleFile)
	if err != nil {
		return nil, err
	}
	s.Debug("bundle parsed", "name", b.Metadata.Name, "packages", len(b.Packages))

	if err := b.Validate(); err != nil {
		return nil, err
	}
	s.Debug("bundle validated")

	creator := newLocalCreator(opts.Config.Options.Architecture)

	srcDir := filepath.Dir(opts.BundleFile)
	var defaultsHCL []byte
	defaultsPath := filepath.Join(srcDir, BundleDefaultsFileName)
	if _, err := os.Stat(defaultsPath); err == nil {
		defaultsHCL, err = materializeDefaultsFile(defaultsPath)
		if err != nil {
			return nil, fmt.Errorf("materializing defaults HCL: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("accessing defaults HCL: %w", err)
	}
	root, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-bundle-create-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		err = os.RemoveAll(root)
		if err != nil {
			s.Warn("failed to remove temporary directory", "path", root, "error", err)
		}
	}()

	ociDir := filepath.Join(root, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	if err := os.MkdirAll(blobDir, tempDirPerm); err != nil {
		return nil, err
	}
	if err := writeOCILayout(filepath.Join(ociDir, "oci-layout")); err != nil {
		return nil, err
	}

	pkgOpts := CreatePackageOptions{
		Config:    opts.Config,
		BlobDir:   blobDir,
		BundleDir: srcDir,
		Streams:   s,
	}
	for i := range b.Packages {
		if err := creator.CreatePackage(ctx, &b.Packages[i], pkgOpts); err != nil {
			return nil, err
		}
	}

	s.Debug("creating bundle definition manifest")
	cfgManifest, err := createBundleDefinitionManifest(ctx, s, ociDir, bundleHCL, defaultsHCL, srcDir, b.Packages)
	if err != nil {
		return nil, err
	}
	allManifests := append([]ociManifest{cfgManifest}, creator.manifests...)

	s.Debug("cleaning unreferenced blobs")
	if err := gcUnreferencedBlobs(ctx, s, blobDir, allManifests); err != nil {
		return nil, fmt.Errorf("cleaning up unreferenced blobs: %w", err)
	}

	idx := newBundleIndex(allManifests, creator.arch)
	if err := writeOCIIndex(filepath.Join(ociDir, "index.json"), idx); err != nil {
		return nil, err
	}

	outPath := filepath.Join(srcDir, creator.BundleName(b))
	s.Debug("writing bundle archive", "output", outPath)
	if err := writeTarZst(ctx, s, outPath, root); err != nil {
		return nil, err
	}
	s.Info("bundle archive written", "output", outPath)
	return &CreateResult{BundleName: b.Metadata.Name, OutputPath: outPath}, nil
}

// newBundleIndex builds the canonical single-arch bundle (child) index per
// ADR-0015: self-identified by artifactType, arch recorded as an annotation,
// and manifests sorted by digest so the index digest is deterministic
// regardless of HCL package declaration order.
func newBundleIndex(manifests []ociManifest, arch string) *ociIndex {
	if arch == "" {
		arch = runtime.GOARCH
	}
	sorted := make([]ociManifest, len(manifests))
	copy(sorted, manifests)
	sortManifestsByDigest(sorted)
	return &ociIndex{
		SchemaVersion: 2,
		MediaType:     ocispec.MediaTypeImageIndex,
		ArtifactType:  MediaTypeBundle,
		Manifests:     sorted,
		Annotations: map[string]string{
			AnnotationBundleArchitecture: arch,
		},
	}
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
func createBundleDefinitionManifest(ctx context.Context, streams iostreams.IOStreams, ociDir string, hclData, defaultsData []byte, bundleDir string, pkgs []Package) (ociManifest, error) {
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
	hclDesc, err := pushBlob(MediaTypeBundleHCL, hclData, map[string]string{
		ocispec.AnnotationTitle: "bundle.uds.hcl",
	})
	if err != nil {
		return ociManifest{}, fmt.Errorf("pushing bundle HCL: %w", err)
	}

	// defaults.uds.hcl as an optional layer if present alongside bundle.uds.hcl.
	layers := []ocispec.Descriptor{hclDesc}
	if defaultsData != nil {
		defaultsDesc, err := pushBlob(MediaTypeBundleHCL, defaultsData, map[string]string{
			ocispec.AnnotationTitle: BundleDefaultsFileName,
		})
		if err != nil {
			return ociManifest{}, fmt.Errorf("pushing defaults HCL: %w", err)
		}
		layers = append(layers, defaultsDesc)
		streams.Debug("included defaults.uds.hcl in bundle definition")
	}

	// Values files as subsequent layers, preserving logical path in the annotation.
	for _, pkg := range pkgs {
		for i, vf := range pkg.ValuesFiles {
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
		return ociManifest{}, fmt.Errorf("packing bundle definition manifest: %w", err)
	}

	return ociManifest{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: MediaTypeBundleDefinition,
		Digest:       desc.Digest.String(),
		Size:         desc.Size,
	}, nil
}
