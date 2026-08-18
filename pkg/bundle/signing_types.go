// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import "github.com/defenseunicorns/uds-cli/pkg/iostreams"

const (
	BundleSignatureStatusVerified = "verified"
)

// BundleSignatureSummary reports the status of bundle signature verification.
type BundleSignatureSummary struct {
	Status string `json:"status" yaml:"status" text:"Status"`
}

// InspectResult is the serializable result shape emitted by bundle inspection.
type InspectResult struct {
	Name            string                  `json:"name" yaml:"name" text:"Name"`
	Description     string                  `json:"description,omitempty" yaml:"description,omitempty" text:"Description,omitempty"`
	Version         string                  `json:"version,omitempty" yaml:"version,omitempty" text:"Version,omitempty"`
	ArtifactDigest  string                  `json:"artifactDigest,omitempty" yaml:"artifactDigest,omitempty" text:"Artifact Digest,omitempty"`
	BundleSignature *BundleSignatureSummary `json:"bundleSignature,omitempty" yaml:"bundleSignature,omitempty" text:"Bundle Signature,omitempty"`
}

// SigningMode identifies the credentials used to sign a bundle.
type SigningMode string

const (
	SigningModeKey      SigningMode = "key"
	SigningModeKeyless  SigningMode = "keyless"
	SigningModeUnsigned SigningMode = "unsigned"
)

// SigningOptions configures a bundle signature operation.
type SigningOptions struct {
	Mode           SigningMode
	Key            string
	KeyPassword    string
	IdentityToken  string
	FulcioURL      string
	FulcioAuthFlow string
	OIDCIssuer     string
	OIDCClientID   string
	RekorURL       string
	TSAServerURL   string
	Overwrite      bool
}

// KeylessVerification constrains the certificate identity trusted for a keyless signature.
type KeylessVerification struct {
	CertificateIdentity         string
	CertificateIdentityRegexp   string
	CertificateOIDCIssuer       string
	CertificateOIDCIssuerRegexp string
	TrustedRoot                 string
}

// VerificationPolicy is consumer-controlled trust material for a bundle signature.
type VerificationPolicy struct {
	PublicKey string
	Keyless   *KeylessVerification
}

// SignOptions configures signing an existing bundle artifact.
type SignOptions struct {
	Source  string
	Signing SigningOptions
	Config  *UDSBundleConfig
	TmpDir  string
	Streams iostreams.IOStreams
}

// VerifyOptions configures verification of a bundle artifact.
type VerifyOptions struct {
	Source  string
	Policy  VerificationPolicy
	Config  *UDSBundleConfig
	TmpDir  string
	Streams iostreams.IOStreams
}
