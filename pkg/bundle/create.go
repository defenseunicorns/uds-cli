// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	_, _ = fmt.Fprintf(opts.Out, "Ingesting package %q...\n", pkg.Name)

	var manifests []ociManifest
	var err error

	if IsOCIReference(pkg.Source) {
		// Strip any explicit scheme prefix (e.g. "oci://") before handing to
		// go-containerregistry which does not expect it.
		refName := TrimScheme(pkg.Source)
		manifests, err = ingestRemoteReference(ctx, opts.BlobDir, refName, opts.Arch)
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
	parser := NewHCLParser()
	b, err := parser.ParseBundleFile(ctx, opts.BundleFile)
	if err != nil {
		return "", err
	}
	if err := b.Validate(); err != nil {
		return "", err
	}

	arch := opts.Arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	creator := newLocalCreator(arch)

	srcDir := filepath.Dir(opts.BundleFile)
	root, err := os.MkdirTemp("", "uds-bundle-create-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(root) }()

	if err := writeBundleHCL(root, opts.BundleFile); err != nil {
		return "", err
	}
	if err := writeValues(root, srcDir, b.Packages); err != nil {
		return "", err
	}

	ociDir := filepath.Join(root, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		return "", err
	}
	if err := writeOCILayout(filepath.Join(ociDir, "oci-layout")); err != nil {
		return "", err
	}

	pkgOpts := CreatePackageOptions{
		BlobDir:   blobDir,
		BundleDir: srcDir,
		Arch:      arch,
		Out:       opts.Out,
	}
	for i := range b.Packages {
		if err := creator.CreatePackage(ctx, &b.Packages[i], pkgOpts); err != nil {
			return "", err
		}
	}

	if err := gcUnreferencedBlobs(blobDir, creator.manifests); err != nil {
		return "", fmt.Errorf("cleaning up unreferenced blobs: %w", err)
	}

	idx := &ociIndex{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.index.v1+json",
		Manifests:     creator.manifests,
	}
	if err := writeOCIIndex(filepath.Join(ociDir, "index.json"), idx); err != nil {
		return "", err
	}

	outPath := filepath.Join(srcDir, creator.BundleName(b))
	if err := writeTarZst(ctx, outPath, root); err != nil {
		return "", err
	}
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

func writeBundleHCL(root, bundleFile string) error {
	src, err := os.ReadFile(bundleFile)
	if err != nil {
		return fmt.Errorf("cannot read bundle file: %w", err)
	}
	return os.WriteFile(filepath.Join(root, "uds-bundle.hcl"), src, 0o644)
}

func writeValues(root, bundleDir string, pkgs []Package) error {
	for _, pkg := range pkgs {
		for i, vf := range pkg.ValueFiles {
			src := vf
			if !filepath.IsAbs(src) {
				src = filepath.Join(bundleDir, vf)
			}
			st, err := os.Stat(src)
			if err != nil {
				return fmt.Errorf("package %q: cannot stat value file %q: %w", pkg.Name, vf, err)
			}
			if st.IsDir() {
				return fmt.Errorf("package %q: value file %q is a directory", pkg.Name, vf)
			}

			dstDir := filepath.Join(root, "values", pkg.Name)
			if err := os.MkdirAll(dstDir, 0o755); err != nil {
				return err
			}
			dst := filepath.Join(dstDir, fmt.Sprintf("%d.yaml", i))

			contents, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dst, contents, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
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
