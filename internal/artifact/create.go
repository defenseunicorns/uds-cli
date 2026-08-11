// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/bundlehcl"
	"github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
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
	manifests []oci.OciManifest
	arch      string
}

// Create assembles an OCI layout and writes it as a UDS bundle archive.
func Create(ctx context.Context, opts CreateOptions) (*CreateResult, error) {
	if err := bundlehcl.ValidateConfig(opts.Config); err != nil {
		return nil, err
	}
	if opts.Bundle == nil {
		return nil, fmt.Errorf("bundle must not be nil")
	}
	if err := ValidateBundleForCreate(opts.Bundle); err != nil {
		return nil, err
	}
	if opts.BundleDir == "" {
		return nil, fmt.Errorf("BundleDir is required")
	}

	creator := newLocalCreator(opts.Config.Options.Architecture)
	root, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-bundle-create-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if removeErr := os.RemoveAll(root); removeErr != nil {
			opts.Streams.Warn("failed to remove temporary directory", "path", root, "error", removeErr)
		}
	}()

	ociDir := filepath.Join(root, "oci")
	blobDir := filepath.Join(ociDir, "blobs", "sha256")
	if err := os.MkdirAll(blobDir, tempDirPerm); err != nil {
		return nil, err
	}
	if err := oci.WriteOCILayout(filepath.Join(ociDir, "oci-layout")); err != nil {
		return nil, err
	}

	pkgOpts := CreatePackageOptions{
		Config:    opts.Config,
		BlobDir:   blobDir,
		BundleDir: opts.BundleDir,
		Streams:   opts.Streams,
	}
	for i := range opts.Bundle.Packages {
		if err := creator.CreatePackage(ctx, &opts.Bundle.Packages[i], pkgOpts); err != nil {
			return nil, err
		}
	}

	opts.Streams.Debug("creating bundle definition manifest")
	definition, err := createBundleDefinitionManifest(ctx, opts.Streams, ociDir, opts.BundleHCL, opts.DefaultsHCL, opts.BundleDir, opts.Bundle.Packages)
	if err != nil {
		return nil, err
	}
	manifests := append([]oci.OciManifest{definition}, creator.manifests...)
	if err := oci.GCUnreferencedBlobs(ctx, opts.Streams, blobDir, manifests); err != nil {
		return nil, fmt.Errorf("cleaning up unreferenced blobs: %w", err)
	}
	if err := oci.WriteOCIIndex(filepath.Join(ociDir, "index.json"), oci.NewBundleIndex(manifests, creator.arch)); err != nil {
		return nil, err
	}

	outPath := filepath.Join(opts.BundleDir, creator.BundleName(opts.Bundle))
	if err := WriteTarZst(ctx, opts.Streams, outPath, root); err != nil {
		return nil, err
	}
	opts.Streams.Info("bundle archive written", "output", outPath)
	return &CreateResult{OutputPath: outPath}, nil
}

// ensureAnnotation adds an annotation to the manifest, initializing the map if needed.
// If the annotation key already exists, it won't be overwritten.
func ensureAnnotation(m *oci.OciManifest, key, value string) {
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
func (c *localCreator) CreatePackage(ctx context.Context, pkg *spec.Package, opts CreatePackageOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if pkg == nil {
		return fmt.Errorf("package is required")
	}
	if err := zarf.ValidatePackageSignatureVerification(pkg.Name, pkg.SignatureVerification); err != nil {
		return err
	}
	opts.Streams.Info("ingesting package", "name", pkg.Name, "source", pkg.Source)

	config := zarf.ConfigOptions{
		LogLevel:      opts.Config.Options.LogLevel,
		Architecture:  opts.Config.Options.Architecture,
		PlainHTTP:     opts.Config.Options.PlainHTTP,
		SkipTLSVerify: opts.Config.Options.SkipTLSVerify,
		UDSCache:      opts.Config.Options.UDSCache,
		TmpDir:        opts.Config.Options.TmpDir,
		Concurrency:   opts.Config.Options.Concurrency,
	}
	source := zarf.NewPackageSource(pkg.Source, config, opts.BundleDir, opts.Streams)

	filter := zarf.BuildComponentFilter(pkg.OptionalComponents)
	verificationWorkspace, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-package-verify-*")
	if err != nil {
		return fmt.Errorf("creating package verification workspace: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(verificationWorkspace); err != nil {
			opts.Streams.Warn("failed to remove package verification workspace", "path", verificationWorkspace, "error", err)
		}
	}()

	loadOptions, err := zarf.PackageSignatureVerificationOptions(pkg, filepath.Join(verificationWorkspace, "material"), opts.Config.Options.TmpDir)
	if err != nil {
		return fmt.Errorf("package %q: configuring signature verification: %w", pkg.Name, err)
	}
	loadOptions.Filter = filter
	if loadOptions.VerificationStrategy == layout.VerifyNever {
		opts.Streams.Warn("package signature verification is disabled; resulting bundle will contain an unverified package", "name", pkg.Name)
	} else if pkg.SignatureVerification.Keyless != nil {
		keyless := pkg.SignatureVerification.Keyless
		if keyless.InsecureIgnoreTlog || keyless.InsecureIgnoreSCT {
			opts.Streams.Warn("keyless package signature verification has reduced protections", "name", pkg.Name, "ignoreTlog", keyless.InsecureIgnoreTlog, "ignoreSCT", keyless.InsecureIgnoreSCT)
		}
	}

	var manifests []oci.OciManifest
	if loadOptions.VerificationStrategy != layout.VerifyNever {
		stagingDir, err := os.MkdirTemp(verificationWorkspace, "package-*")
		if err != nil {
			return fmt.Errorf("creating package verification workspace: %w", err)
		}
		manifests, err = source.VerifyAndIngestFiltered(ctx, stagingDir, loadOptions, opts.BlobDir)
		if err != nil {
			return fmt.Errorf("package %q: verifying and ingesting package: %w", pkg.Name, err)
		}
	} else {
		manifests, err = source.IngestFiltered(ctx, filter, opts.BlobDir)
		if err != nil {
			return fmt.Errorf("package %q: %w", pkg.Name, err)
		}
	}

	// Set the OCI reference annotation for each manifest
	refName := pkg.Name
	if oci.IsOCIReference(pkg.Source) {
		refName = oci.TrimScheme(pkg.Source)
	}
	for i := range manifests {
		ensureAnnotation(&manifests[i], "org.opencontainers.image.ref.name", refName)
	}
	annotatePackageVerification(manifests, loadOptions.VerificationStrategy != layout.VerifyNever)

	c.manifests = append(c.manifests, manifests...)
	return nil
}

func annotatePackageVerification(manifests []oci.OciManifest, verified bool) {
	if !verified {
		return
	}
	for i := range manifests {
		if manifests[i].Annotations == nil {
			manifests[i].Annotations = map[string]string{}
		}
		manifests[i].Annotations[oci.AnnotationPackageVerification] = oci.AnnotationPackageVerificationVerified
	}
}

// BundleName returns the output filename for the bundle artifact.
func (c *localCreator) BundleName(b *spec.UDSBundle) string {
	return bundleOutputName(b, c.arch)
}

// bundleOutputName builds the artifact filename from bundle metadata and architecture.
func bundleOutputName(b *spec.UDSBundle, arch string) string {
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

// sanitizeFileComponent replaces characters that are unsafe in artifact filenames.
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
func createBundleDefinitionManifest(ctx context.Context, streams iostreams.IOStreams, ociDir string, hclData, defaultsData []byte, bundleDir string, pkgs []spec.Package) (oci.OciManifest, error) {
	store, err := oraci.New(ociDir)
	if err != nil {
		return oci.OciManifest{}, fmt.Errorf("opening OCI store: %w", err)
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
	hclDesc, err := pushBlob(oci.MediaTypeBundleHCL, hclData, map[string]string{
		ocispec.AnnotationTitle: "bundle.uds.hcl",
	})
	if err != nil {
		return oci.OciManifest{}, fmt.Errorf("pushing bundle HCL: %w", err)
	}

	// defaults.uds.hcl as an optional layer if present alongside bundle.uds.hcl.
	layers := []ocispec.Descriptor{hclDesc}
	if defaultsData != nil {
		defaultsDesc, err := pushBlob(oci.MediaTypeBundleHCL, defaultsData, map[string]string{
			ocispec.AnnotationTitle: bundlehcl.BundleDefaultsFileName,
		})
		if err != nil {
			return oci.OciManifest{}, fmt.Errorf("pushing defaults HCL: %w", err)
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
				return oci.OciManifest{}, fmt.Errorf("package %q: cannot stat value file %q: %w", pkg.Name, vf, err)
			}
			if st.IsDir() {
				return oci.OciManifest{}, fmt.Errorf("package %q: value file %q is a directory", pkg.Name, vf)
			}
			data, err := os.ReadFile(src)
			if err != nil {
				return oci.OciManifest{}, fmt.Errorf("package %q: reading value file %q: %w", pkg.Name, vf, err)
			}
			valDesc, err := pushBlob(oci.MediaTypeBundleValuesYAML, data, map[string]string{
				ocispec.AnnotationTitle: fmt.Sprintf("values/%s/%d.yaml", pkg.Name, i),
			})
			if err != nil {
				return oci.OciManifest{}, fmt.Errorf("package %q: pushing value file: %w", pkg.Name, err)
			}
			layers = append(layers, valDesc)
		}
	}

	// PackManifest pushes the empty-JSON config blob, builds the OCI 1.1 artifact manifest with our artifactType,
	// and pushes the manifest blob.  We pin the created timestamp so the manifest digest is reproducible.
	desc, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1, oci.MediaTypeBundleDefinition, oras.PackManifestOptions{
		Layers: layers,
		ManifestAnnotations: map[string]string{
			ocispec.AnnotationCreated: "1970-01-01T00:00:00Z",
		},
	})
	if err != nil {
		return oci.OciManifest{}, fmt.Errorf("packing bundle definition manifest: %w", err)
	}

	return oci.OciManifest{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: oci.MediaTypeBundleDefinition,
		Digest:       desc.Digest.String(),
		Size:         desc.Size,
	}, nil
}
