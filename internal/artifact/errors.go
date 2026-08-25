// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"errors"
	"fmt"

	"github.com/opencontainers/go-digest"
)

var (
	ErrManifestNotFound                  = errors.New("bundle definition manifest not found in index")
	ErrHCLLayerNotFound                  = errors.New("bundle HCL layer not found in config manifest")
	ErrBundleDefinitionLayerNotFound     = errors.New("bundle artifact contains no bundle definition layer")
	ErrBundleNil                         = errors.New("bundle must not be nil")
	ErrBundleDirRequired                 = errors.New("bundle directory is required")
	ErrPackageNil                        = errors.New("package must not be nil")
	ErrConfigNil                         = errors.New("config must not be nil")
	ErrConfigOptionsNil                  = errors.New("config.Options must not be nil")
	ErrMultipleBundleDefinitionManifests = errors.New("bundle index contains multiple bundle definition manifests")
	ErrExtractingBundleArtifact          = errors.New("extracting bundle artifact")
	ErrResolvingLayerTitle               = errors.New("resolving layer title")
	ErrCheckingLayerTitle                = errors.New("checking layer title")
	ErrReadingConfigManifest             = errors.New("reading config manifest blob")
	ErrParsingConfigManifest             = errors.New("parsing config manifest")
	ErrReadingHCLBlob                    = errors.New("reading HCL blob")
	ErrParsingHCLBlob                    = errors.New("parsing HCL blob")
	ErrReadingBundleIndex                = errors.New("reading index.json")
	ErrParsingBundleIndex                = errors.New("parsing index.json")
	ErrLocatingBundleDefinition          = errors.New("locating bundle definition")
	ErrVerifyingArtifactDigest           = errors.New("artifact digest verification failed")
	ErrReadingBundleDefinitionManifest   = errors.New("reading bundle definition manifest")
	ErrParsingBundleDefinitionManifest   = errors.New("parsing bundle definition manifest")
	ErrReadingValuesDirectory            = errors.New("reading values directory")
	ErrResolvingDestinationDirectory     = errors.New("resolving destination directory")
	ErrPruningUnreferencedBlobs          = errors.New("cleaning up unreferenced blobs")
	ErrCreatingVerificationWorkspace     = errors.New("creating package verification workspace")
	ErrPushingBundleHCL                  = errors.New("pushing bundle HCL")
	ErrPushingDefaultsHCL                = errors.New("pushing defaults HCL")
	ErrPackingBundleDefinitionManifest   = errors.New("packing bundle definition manifest")
	ErrCreatingInspectionWorkspace       = errors.New("creating inspection workspace")
	ErrFetchingBundleDefinitionManifest  = errors.New("fetching bundle definition manifest")
	ErrFetchingBundleDefinitionHCL       = errors.New("fetching bundle definition HCL")
	ErrInvalidBundle                     = errors.New("invalid bundle")
	ErrFetchingPackageManifest           = errors.New("fetching package manifest")
	ErrParsingPackageManifest            = errors.New("parsing package manifest")
	ErrFetchingZarfYAML                  = errors.New("fetching zarf.yaml")
	ErrParsingZarfYAML                   = errors.New("parsing zarf.yaml")
	ErrFetchingZarfLayer                 = errors.New("fetching Zarf package metadata layer")
)

var (
	_ error = (*LayerTitleEscapesDestinationError)(nil)
	_ error = (*OutputPathIsDirError)(nil)
	_ error = (*UnsupportedFileTypeError)(nil)
	_ error = (*InvalidBundleIndexError)(nil)
	_ error = (*ReadingPackageValuesError)(nil)
	_ error = (*ReadingLayerBlobError)(nil)
	_ error = (*CreatingLayerDirectoryError)(nil)
	_ error = (*WritingLayerError)(nil)
	_ error = (*UnsupportedPackageManifestMediaTypeError)(nil)
	_ error = (*MissingPackageManifestAnnotationError)(nil)
	_ error = (*ConflictingPackageManifestDigestError)(nil)
	_ error = (*ConfiguringPackageSignatureVerificationError)(nil)
	_ error = (*VerifyingAndIngestingPackageError)(nil)
	_ error = (*IngestingPackageError)(nil)
	_ error = (*ValueFileStatError)(nil)
	_ error = (*ValueFileIsDirectoryError)(nil)
	_ error = (*ReadingValueFileError)(nil)
	_ error = (*PushingValueFileError)(nil)
	_ error = (*ResolvingInspectSourceError)(nil)
	_ error = (*ResolvingBundleSourceError)(nil)
	_ error = (*UnsupportedSchemaVersionError)(nil)
	_ error = (*UnsupportedMediaTypeError)(nil)
	_ error = (*MissingBundleArchitectureError)(nil)
	_ error = (*InspectingPackageSignatureError)(nil)
	_ error = (*UnsupportedPackageEntryMediaTypeError)(nil)
	_ error = (*MultiplePackageManifestEntriesError)(nil)
	_ error = (*PackageManifestNotFoundError)(nil)
	_ error = (*LayerNotFoundError)(nil)
	_ error = (*MissingZarfPackageNameError)(nil)
)

type LayerTitleEscapesDestinationError struct{ Title string }

func (e LayerTitleEscapesDestinationError) Error() string {
	return fmt.Sprintf("layer title %q escapes destination directory", e.Title)
}

type OutputPathIsDirError struct{ Path string }

func (e OutputPathIsDirError) Error() string {
	return fmt.Sprintf("output path %q is a directory", e.Path)
}

type UnsupportedFileTypeError struct{ Path string }

func (e UnsupportedFileTypeError) Error() string {
	return fmt.Sprintf("unsupported file type %q in bundle staging dir", e.Path)
}

type InvalidBundleIndexError struct {
	Source       string
	ArtifactType string
}

func (e InvalidBundleIndexError) Error() string {
	return fmt.Sprintf("%s does not appear to be a UDS bundle: index does not declare artifactType %s", e.Source, e.ArtifactType)
}

type ReadingPackageValuesError struct {
	Package string
	Err     error
}

func (e ReadingPackageValuesError) Error() string {
	return fmt.Sprintf("reading values for package %s: %v", e.Package, e.Err)
}
func (e ReadingPackageValuesError) Unwrap() error { return e.Err }

type ReadingLayerBlobError struct {
	Title  string
	Digest digest.Digest
	Err    error
}

func (e ReadingLayerBlobError) Error() string {
	return fmt.Sprintf("reading layer blob %s (%s): %v", e.Title, e.Digest, e.Err)
}
func (e ReadingLayerBlobError) Unwrap() error { return e.Err }

type CreatingLayerDirectoryError struct {
	Title string
	Err   error
}

func (e CreatingLayerDirectoryError) Error() string {
	return fmt.Sprintf("creating directory for %s: %v", e.Title, e.Err)
}
func (e CreatingLayerDirectoryError) Unwrap() error { return e.Err }

type WritingLayerError struct {
	Title string
	Err   error
}

func (e WritingLayerError) Error() string { return fmt.Sprintf("writing %s: %v", e.Title, e.Err) }
func (e WritingLayerError) Unwrap() error { return e.Err }

type UnsupportedPackageManifestMediaTypeError struct {
	Index     int
	MediaType string
}

func (e UnsupportedPackageManifestMediaTypeError) Error() string {
	return fmt.Sprintf("package manifest at index %d has unsupported media type %q", e.Index, e.MediaType)
}

type MissingPackageManifestAnnotationError struct {
	Index      int
	Annotation string
}

func (e MissingPackageManifestAnnotationError) Error() string {
	return fmt.Sprintf("package manifest at index %d has no %s annotation", e.Index, e.Annotation)
}

type ConflictingPackageManifestDigestError struct {
	Annotation     string
	Reference      string
	ExistingDigest digest.Digest
	Digest         digest.Digest
}

func (e ConflictingPackageManifestDigestError) Error() string {
	return fmt.Sprintf("duplicate %s %q with digests %s and %s", e.Annotation, e.Reference, e.ExistingDigest, e.Digest)
}

type ConfiguringPackageSignatureVerificationError struct {
	Package string
	Err     error
}

func (e ConfiguringPackageSignatureVerificationError) Error() string {
	return fmt.Sprintf("package %q: configuring signature verification: %v", e.Package, e.Err)
}
func (e ConfiguringPackageSignatureVerificationError) Unwrap() error { return e.Err }

type VerifyingAndIngestingPackageError struct {
	Package string
	Err     error
}

func (e VerifyingAndIngestingPackageError) Error() string {
	return fmt.Sprintf("package %q: verifying and ingesting package: %v", e.Package, e.Err)
}
func (e VerifyingAndIngestingPackageError) Unwrap() error { return e.Err }

type IngestingPackageError struct {
	Package string
	Err     error
}

func (e IngestingPackageError) Error() string { return fmt.Sprintf("package %q: %v", e.Package, e.Err) }
func (e IngestingPackageError) Unwrap() error { return e.Err }

type ValueFileStatError struct {
	Package string
	Path    string
	Err     error
}

func (e ValueFileStatError) Error() string {
	return fmt.Sprintf("package %q: cannot stat value file %q: %v", e.Package, e.Path, e.Err)
}
func (e ValueFileStatError) Unwrap() error { return e.Err }

type ValueFileIsDirectoryError struct {
	Package string
	Path    string
}

func (e ValueFileIsDirectoryError) Error() string {
	return fmt.Sprintf("package %q: value file %q is a directory", e.Package, e.Path)
}

type ReadingValueFileError struct {
	Package string
	Path    string
	Err     error
}

func (e ReadingValueFileError) Error() string {
	return fmt.Sprintf("package %q: reading value file %q: %v", e.Package, e.Path, e.Err)
}
func (e ReadingValueFileError) Unwrap() error { return e.Err }

type PushingValueFileError struct {
	Package string
	Err     error
}

func (e PushingValueFileError) Error() string {
	return fmt.Sprintf("package %q: pushing value file: %v", e.Package, e.Err)
}
func (e PushingValueFileError) Unwrap() error { return e.Err }

type ResolvingInspectSourceError struct {
	Source string
	Err    error
}

func (e ResolvingInspectSourceError) Error() string {
	return fmt.Sprintf("resolving inspect source %s: %v", e.Source, e.Err)
}
func (e ResolvingInspectSourceError) Unwrap() error { return e.Err }

type ResolvingBundleSourceError struct {
	Source string
	Err    error
}

func (e ResolvingBundleSourceError) Error() string {
	return fmt.Sprintf("resolving bundle from %s: %v", e.Source, e.Err)
}
func (e ResolvingBundleSourceError) Unwrap() error { return e.Err }

type UnsupportedSchemaVersionError struct {
	Artifact string
	Version  int
}

func (e UnsupportedSchemaVersionError) Error() string {
	return fmt.Sprintf("%s has unsupported schema version %d", e.Artifact, e.Version)
}

type UnsupportedMediaTypeError struct {
	Artifact  string
	MediaType string
}

func (e UnsupportedMediaTypeError) Error() string {
	return fmt.Sprintf("%s has unsupported media type %q", e.Artifact, e.MediaType)
}

type MissingBundleArchitectureError struct{ Annotation string }

func (e MissingBundleArchitectureError) Error() string {
	return fmt.Sprintf("bundle index does not record its architecture: missing the %s annotation", e.Annotation)
}

type InspectingPackageSignatureError struct {
	Package string
	Err     error
}

func (e InspectingPackageSignatureError) Error() string {
	return fmt.Sprintf("inspecting package %q signature: %v", e.Package, e.Err)
}
func (e InspectingPackageSignatureError) Unwrap() error { return e.Err }

type UnsupportedPackageEntryMediaTypeError struct {
	Package   string
	MediaType string
}

func (e UnsupportedPackageEntryMediaTypeError) Error() string {
	return fmt.Sprintf("package %q entry has unsupported media type %q", e.Package, e.MediaType)
}

type MultiplePackageManifestEntriesError struct{ Package string }

func (e MultiplePackageManifestEntriesError) Error() string {
	return fmt.Sprintf("package %q has multiple matching manifest entries", e.Package)
}

type PackageManifestNotFoundError struct {
	Package string
	Source  string
}

func (e PackageManifestNotFoundError) Error() string {
	return fmt.Sprintf("package %q with source %q was not found in bundle index", e.Package, e.Source)
}

type LayerNotFoundError struct{ Title string }

func (e LayerNotFoundError) Error() string {
	return fmt.Sprintf("%s layer not found in manifest", e.Title)
}

type MissingZarfPackageNameError struct{ Package string }

func (e MissingZarfPackageNameError) Error() string {
	return fmt.Sprintf("package %q has no metadata.name in its embedded zarf.yaml", e.Package)
}
