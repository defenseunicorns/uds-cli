// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	specv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// ociIndex is the top-level OCI image index written to index.json.
type ociIndex struct {
	SchemaVersion int           `json:"schemaVersion"`
	MediaType     string        `json:"mediaType"`
	Manifests     []ociManifest `json:"manifests"`
}

// ociManifest is a descriptor entry inside an OCI image index.
type ociManifest struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Platform    *specv1.Platform  `json:"platform,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
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
	Config        ociDescriptor     `json:"config"`
	Layers        []ociDescriptor   `json:"layers"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}
