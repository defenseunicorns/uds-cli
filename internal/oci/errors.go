// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"errors"
	"fmt"

	"github.com/opencontainers/go-digest"
	"oras.land/oras-go/v2/errdef"
)

var (
	ErrStoreRootRequired            = errors.New("OCI store root is required")
	ErrBundleDefinitionNotFound     = errors.New("bundle definition manifest not found in index")
	ErrTagReferenceRequired         = errors.New("OCI source must use a tag reference, not a digest")
	ErrOpenLayout                   = errors.New("opening OCI layout")
	ErrCreateBlobDirectory          = errors.New("creating OCI blob directory")
	ErrReadIndex                    = errors.New("reading index.json")
	ErrParseIndex                   = errors.New("parsing index.json")
	ErrParseReference               = errors.New("parsing OCI reference")
	ErrFetchContent                 = errors.New("fetching OCI content")
	ErrCopyGraph                    = errors.New("copying OCI graph")
	ErrCheckTargetTag               = errors.New("checking target tag")
	ErrPushRootIndex                = errors.New("pushing root index")
	ErrListBlobs                    = errors.New("listing OCI blobs")
	ErrParseBlobDigest              = errors.New("parsing OCI blob digest")
	ErrRemoveUnreferencedBlob       = errors.New("removing unreferenced blob")
	ErrReadSuccessors               = errors.New("reading descriptor successors")
	ErrVerifyDescriptor             = errors.New("verifying descriptor")
	ErrLoadCredentials              = errors.New("loading docker credentials")
	ErrDetermineRegistryTransport   = errors.New("determining registry transport")
	ErrResolveReference             = errors.New("resolving OCI reference")
	ErrConfigureTransfer            = errors.New("configuring OCI transfer")
	ErrPushContent                  = errors.New("pushing OCI content")
	ErrPullContent                  = errors.New("pulling OCI content")
	ErrTagContent                   = errors.New("tagging OCI content")
	ErrCreateTemporaryDirectory     = errors.New("creating temporary directory")
	ErrCreateOCIDirectory           = errors.New("creating OCI directory")
	ErrCreateStore                  = errors.New("creating OCI store")
	ErrWriteIndex                   = errors.New("writing index.json")
	ErrRemoveDuplicateIndexBlob     = errors.New("removing duplicate index blob")
	ErrCreateBundleArchive          = errors.New("creating bundle archive")
	ErrBundleArchiveHookRequired    = errors.New("creating bundle archive: archive hook is required")
	ErrMissingArchitecture          = errors.New("bundle is missing its architecture annotation")
	ErrReadRootDescriptor           = errors.New("reading OCI root descriptor")
	ErrReadExistingRootIndex        = errors.New("reading existing root index")
	ErrMarshalRootIndex             = errors.New("marshaling root index")
	ErrParseExistingRegistryContent = errors.New("parsing existing registry content")
	ErrInvalidBundle                = errors.New("content is not a valid UDS bundle")
	ErrArchitectureUnavailable      = errors.New("bundle architecture is unavailable")
	ErrPushTagRequired              = errors.New("bundles must be pushed to a tag reference")
	ErrCheckBundleContent           = errors.New("checking bundle content")
	ErrBundleSignatureNotFound      = errors.New("bundle signature evidence not found")
	ErrBundleSignatureDuplicate     = errors.New("duplicate bundle signature evidence")
)

var (
	_ error = (*EmptyParameterError)(nil)
	_ error = (*DescriptorTooLargeError)(nil)
	_ error = (*TargetTagExistsError)(nil)
	_ error = (*ManifestCountError)(nil)
	_ error = (*InvalidDigestError)(nil)
	_ error = (*ConflictingDescriptorSizeError)(nil)
)

type EmptyParameterError struct{ Name string }

func (e EmptyParameterError) Error() string {
	return fmt.Sprintf("%s must not be empty", e.Name)
}

type DescriptorTooLargeError struct {
	Digest digest.Digest
	Size   int64
	Limit  int64
}

func (e DescriptorTooLargeError) Error() string {
	return fmt.Sprintf("descriptor %s is %d bytes, larger than the %d byte buffered fetch limit", e.Digest, e.Size, e.Limit)
}

type TargetTagExistsError struct{ Tag string }

func (e TargetTagExistsError) Error() string {
	return fmt.Sprintf("target tag %q already exists in registry", e.Tag)
}

type ManifestCountError struct {
	Count int
	Want  int
}

func (e ManifestCountError) Error() string {
	return fmt.Sprintf("index.json contains %d manifests; expected exactly %d for a Zarf package layout", e.Count, e.Want)
}

type InvalidDigestError struct {
	Digest string
	Err    error
}

func (e InvalidDigestError) Error() string {
	return fmt.Sprintf("invalid digest %q: %v", e.Digest, e.Err)
}
func (e InvalidDigestError) Unwrap() error { return e.Err }

type ConflictingDescriptorSizeError struct {
	Digest       digest.Digest
	RecordedSize int64
	ActualSize   int64
}

func (e ConflictingDescriptorSizeError) Error() string {
	return fmt.Sprintf("descriptor %s has conflicting sizes %d and %d", e.Digest, e.RecordedSize, e.ActualSize)
}

// IsNotFound reports whether err is an ORAS not-found error.
func IsNotFound(err error) bool {
	return errors.Is(err, errdef.ErrNotFound)
}
