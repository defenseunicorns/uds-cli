// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package spec

// UDSBundle represents a parsed UDS bundle definition.
type UDSBundle struct {
	UDS      UDSBlock
	Metadata Metadata
	Packages []Package
}

// UDSBlock contains tooling and schema constraints.
type UDSBlock struct {
	BundleAPIVersion string
}

// Metadata holds bundle-level identity and descriptive fields.
type Metadata struct {
	Name        string
	Description string
	Version     string
}

// Package represents a Zarf package entry in a bundle.
type Package struct {
	Name                  string
	Source                string
	Namespace             string
	DependsOn             []PackageRef
	ValuesFiles           []string
	OptionalComponents    []string
	SignatureVerification *PackageSignatureVerification
}

// PackageSignatureVerification declares how a package signature is verified
// when the package enters a bundle.
type PackageSignatureVerification struct {
	Verify    *bool
	PublicKey string
	Keyless   *KeylessSignatureVerification
}

// KeylessSignatureVerification constrains the keyless signer trusted for a
// package signature.
type KeylessSignatureVerification struct {
	CertificateIdentity         string
	CertificateIdentityRegexp   string
	CertificateOIDCIssuer       string
	CertificateOIDCIssuerRegexp string
	TrustedRoot                 string
	InsecureIgnoreTlog          bool
	InsecureIgnoreSCT           bool
	UseSignedTimestamps         bool
}

// PackageRef identifies another package in the bundle.
type PackageRef struct {
	Name string
}
