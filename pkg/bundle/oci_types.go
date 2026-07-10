// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// Media types for UDS bundle OCI artifacts.
	MediaTypeBundleDefinition = "application/vnd.defenseunicorns.uds.bundle.definition.v1"
	MediaTypeBundleHCL        = "application/vnd.defenseunicorns.uds.bundle.hcl.v1"
	MediaTypeBundleValuesYAML = "application/vnd.defenseunicorns.uds.bundle.values.v1+yaml"

	// MediaTypeBundle is the artifactType of the canonical single-arch bundle
	// index (the child index a published tag's root index points at, and the
	// index.json inside a bundle .tar.zst). See ADR-0015.
	MediaTypeBundle = "application/vnd.defenseunicorns.uds.bundle.v1"

	// MediaTypeZarfLayer is the media type for Zarf package file layers.
	MediaTypeZarfLayer = "application/vnd.defenseunicorns.zarf.layer.v1"
)

// AnnotationBundleArchitecture is the child bundle index annotation recording
// the single architecture the bundle was built for. Member package entries
// carry no platform field (ADR-0015), so this keeps the artifact
// self-describing and lets push populate the root index's platform entry.
const AnnotationBundleArchitecture = "uds.dev/architecture"

// ociIndex is the top-level OCI image index written to index.json.
// For a UDS bundle child index, ArtifactType is MediaTypeBundle and
// Annotations carries AnnotationBundleArchitecture; a multi-arch root index
// has neither (it is a plain platform router, see ADR-0015).
type ociIndex struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType"`
	ArtifactType  string            `json:"artifactType,omitempty"`
	Manifests     []ociManifest     `json:"manifests"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

// ociManifest is a descriptor entry inside an OCI image index.
type ociManifest struct {
	MediaType    string            `json:"mediaType"`
	ArtifactType string            `json:"artifactType,omitempty"`
	Digest       string            `json:"digest"`
	Size         int64             `json:"size"`
	Platform     *specv1.Platform  `json:"platform,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
}

// ociLayout is the content of the oci-layout marker file.
type ociLayout struct {
	ImageLayoutVersion string `json:"imageLayoutVersion"`
}

// ociDescriptor is a generic OCI content descriptor used inside image manifests.
type ociDescriptor struct {
	MediaType   string            `json:"mediaType,omitempty"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	URLs        []string          `json:"urls,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ociImageManifest is the image manifest JSON blob referenced by an ociManifest entry.
type ociImageManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType,omitempty"`
	ArtifactType  string            `json:"artifactType,omitempty"`
	Config        ociDescriptor     `json:"config"`
	Layers        []ociDescriptor   `json:"layers"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}
