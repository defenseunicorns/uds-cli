// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package artifact

import (
	"io/fs"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// temp dir/file permissions modes (owner-only) for directories/files created inside temporary or test working areas.
	tempDirPerm fs.FileMode = 0o700
	tmpFilePerm fs.FileMode = 0o600
)

// ExtractedBundle holds the results of extracting a .tar.zst bundle artifact
// into a caller-owned destination directory. Dir is shaped like a bundle source
// directory so that existing config-resolution code (ParseBundleFile,
// loadBundleDefaults) works without modification.
//
// Directory lifecycle is the caller's responsibility. ExtractArtifact writes
// into Dir but does not create or remove it.
type ExtractedBundle struct {
	// Dir is the root dir that contains bundle.uds.hcl (and optionally
	// defaults.uds.hcl and values/<pkg>/<idx>.yaml) at the top level.
	Dir string

	// OCIDir is the path to the extracted OCI image layout (Dir+"/oci").
	OCIDir string

	// BundleDefPath is the absolute path to the materialized bundle definition file (bundle.uds.hcl).
	BundleDefPath string

	// PackageManifests maps each package ref.name to its OCI manifest.
	PackageManifests map[string]ocispec.Descriptor
}

// CreateOptions contains the private inputs needed to assemble a bundle artifact.
type CreateOptions struct {
	Config      *bundleinternal.UDSBundleConfig
	Bundle      *spec.UDSBundle
	BundleHCL   []byte
	DefaultsHCL []byte
	BundleDir   string
	Streams     iostreams.IOStreams
}

// CreateResult contains the path written by Create.
type CreateResult struct {
	OutputPath string
}
