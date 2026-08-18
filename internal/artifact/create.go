// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// Create assembles an OCI layout and writes it as a UDS bundle archive.
func Create(ctx context.Context, opts CreateOptions) (*CreateResult, error) {
	if err := bundleinternal.ValidateConfig(opts.Config); err != nil {
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
	store, err := oci.CreateStore(ociDir)
	if err != nil {
		return nil, fmt.Errorf("opening OCI store: %w", err)
	}

	var packageManifests []ocispec.Descriptor
	for i := range opts.Bundle.Packages {
		manifests, err := ingestSource(ctx, &opts.Bundle.Packages[i], opts.Config, store, opts.BundleDir, opts.Streams)
		if err != nil {
			return nil, err
		}
		packageManifests = append(packageManifests, manifests...)
	}

	opts.Streams.Debug("creating bundle definition manifest")
	definition, err := createBundleDefinitionManifest(ctx, opts.Streams, ociDir, opts.BundleHCL, opts.DefaultsHCL, opts.BundleDir, opts.Bundle.Packages)
	if err != nil {
		return nil, err
	}
	manifests := append([]ocispec.Descriptor{definition}, packageManifests...)
	idx := oci.NewBundleIndex(manifests, opts.Config.Options.Architecture)
	if err := store.PruneUnreferencedBlobs(ctx, opts.Streams, idx.Manifests); err != nil {
		return nil, fmt.Errorf("cleaning up unreferenced blobs: %w", err)
	}
	if err := oci.WriteIndex(filepath.Join(ociDir, "index.json"), idx); err != nil {
		return nil, err
	}

	outPath := filepath.Join(opts.BundleDir, bundleOutputName(opts.Bundle, opts.Config.Options.Architecture))
	if err := WriteTarZst(ctx, opts.Streams, outPath, root); err != nil {
		return nil, err
	}
	opts.Streams.Info("bundle archive written", "output", outPath)
	return &CreateResult{OutputPath: outPath}, nil
}

// ingestSource ingests one package source into the OCI blob store.
func ingestSource(ctx context.Context, pkg *spec.Package, config *bundleinternal.UDSBundleConfig, store *oci.Store, bundleDir string, streams iostreams.IOStreams) ([]ocispec.Descriptor, error) {
	if pkg == nil {
		return nil, fmt.Errorf("package must not be nil")
	}
	if config == nil {
		return nil, fmt.Errorf("config must not be nil")
	}
	if config.Options == nil {
		return nil, fmt.Errorf("config.Options must not be nil")
	}
	if err := zarf.ValidatePackageSignatureVerification(pkg.Name, pkg.SignatureVerification); err != nil {
		return nil, err
	}
	streams.Info("ingesting package", "name", pkg.Name, "source", pkg.Source)

	zarfConfig := zarf.ConfigOptions{
		LogLevel:      config.Options.LogLevel,
		Architecture:  config.Options.Architecture,
		PlainHTTP:     config.Options.PlainHTTP,
		SkipTLSVerify: config.Options.SkipTLSVerify,
		TmpDir:        config.Options.TmpDir,
		Concurrency:   config.Options.Concurrency,
	}
	source := zarf.NewPackageSource(pkg.Source, zarfConfig, bundleDir, streams)

	filter := zarf.BuildComponentFilter(pkg.OptionalComponents)
	verificationWorkspace, err := os.MkdirTemp(config.Options.TmpDir, "uds-package-verify-*")
	if err != nil {
		return nil, fmt.Errorf("creating package verification workspace: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(verificationWorkspace); err != nil {
			streams.Warn("failed to remove package verification workspace", "path", verificationWorkspace, "error", err)
		}
	}()

	loadOptions, err := zarf.PackageSignatureVerificationOptions(pkg, filepath.Join(verificationWorkspace, "material"), config.Options.TmpDir)
	if err != nil {
		return nil, fmt.Errorf("package %q: configuring signature verification: %w", pkg.Name, err)
	}
	loadOptions.Filter = filter
	if loadOptions.VerificationStrategy == layout.VerifyNever {
		streams.Warn("package signature verification is disabled; resulting bundle will contain an unverified package", "name", pkg.Name)
	} else if pkg.SignatureVerification.Keyless != nil {
		keyless := pkg.SignatureVerification.Keyless
		if keyless.InsecureIgnoreTlog || keyless.InsecureIgnoreSCT {
			streams.Warn("keyless package signature verification has reduced protections", "name", pkg.Name, "ignoreTlog", keyless.InsecureIgnoreTlog, "ignoreSCT", keyless.InsecureIgnoreSCT)
		}
	}

	var descriptors []ocispec.Descriptor
	if loadOptions.VerificationStrategy != layout.VerifyNever {
		stagingDir, err := os.MkdirTemp(verificationWorkspace, "package-*")
		if err != nil {
			return nil, fmt.Errorf("creating package verification workspace: %w", err)
		}
		descriptors, err = source.VerifyAndIngestFiltered(ctx, stagingDir, loadOptions, store)
		if err != nil {
			return nil, fmt.Errorf("package %q: verifying and ingesting package: %w", pkg.Name, err)
		}
	} else {
		descriptors, err = source.IngestFiltered(ctx, filter, store)
		if err != nil {
			return nil, fmt.Errorf("package %q: %w", pkg.Name, err)
		}
	}

	manifests := make([]ocispec.Descriptor, len(descriptors))
	for i, desc := range descriptors {
		annotations := maps.Clone(desc.Annotations)
		if annotations == nil {
			annotations = map[string]string{}
		}
		refName := pkg.Name
		if oci.IsOCIReference(pkg.Source) {
			refName = oci.TrimScheme(pkg.Source)
		}
		annotations[ocispec.AnnotationRefName] = refName
		delete(annotations, oci.AnnotationPackageVerification)
		desc.Annotations = annotations
		manifests[i] = desc
	}
	annotatePackageVerification(manifests, loadOptions.VerificationStrategy != layout.VerifyNever)

	return manifests, nil
}

func annotatePackageVerification(manifests []ocispec.Descriptor, verified bool) {
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
// content-addressed layers. It is identified by artifactType so consumers can better identify it in the index.
func createBundleDefinitionManifest(ctx context.Context, streams iostreams.IOStreams, ociDir string, hclData, defaultsData []byte, bundleDir string, pkgs []spec.Package) (ocispec.Descriptor, error) {
	store, err := oci.CreateStore(ociDir)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("opening OCI store: %w", err)
	}
	// We write index.json ourselves at the end of Create(); prevent ORAS from
	// overwriting it on every Push/Tag call.
	store.AutoSaveIndex = false

	pushBlob := func(mediaType string, data []byte, annotations map[string]string) (ocispec.Descriptor, error) {
		return oci.PushBytes(ctx, store, mediaType, data, annotations)
	}

	// HCL file as the first layer.
	hclDesc, err := pushBlob(oci.MediaTypeBundleHCL, hclData, map[string]string{
		ocispec.AnnotationTitle: "bundle.uds.hcl",
	})
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pushing bundle HCL: %w", err)
	}

	// defaults.uds.hcl as an optional layer if present alongside bundle.uds.hcl.
	layers := []ocispec.Descriptor{hclDesc}
	if defaultsData != nil {
		defaultsDesc, err := pushBlob(oci.MediaTypeBundleHCL, defaultsData, map[string]string{
			ocispec.AnnotationTitle: bundleinternal.BundleDefaultsFileName,
		})
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("pushing defaults HCL: %w", err)
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
				return ocispec.Descriptor{}, fmt.Errorf("package %q: cannot stat value file %q: %w", pkg.Name, vf, err)
			}
			if st.IsDir() {
				return ocispec.Descriptor{}, fmt.Errorf("package %q: value file %q is a directory", pkg.Name, vf)
			}
			data, err := os.ReadFile(src)
			if err != nil {
				return ocispec.Descriptor{}, fmt.Errorf("package %q: reading value file %q: %w", pkg.Name, vf, err)
			}
			valDesc, err := pushBlob(oci.MediaTypeBundleValuesYAML, data, map[string]string{
				ocispec.AnnotationTitle: fmt.Sprintf("values/%s/%d.yaml", pkg.Name, i),
			})
			if err != nil {
				return ocispec.Descriptor{}, fmt.Errorf("package %q: pushing value file: %w", pkg.Name, err)
			}
			layers = append(layers, valDesc)
		}
	}

	// PackManifest pushes the empty-JSON config blob, builds the OCI 1.1 artifact manifest with our artifactType,
	// and pushes the manifest blob with a reproducible created timestamp.
	desc, err := oci.PackBundleDefinitionManifest(ctx, store, layers)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("packing bundle definition manifest: %w", err)
	}

	desc.MediaType = ocispec.MediaTypeImageManifest
	desc.ArtifactType = oci.MediaTypeBundleDefinition
	return desc, nil
}
