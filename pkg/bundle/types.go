// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import "github.com/defenseunicorns/uds-cli/pkg/iostreams"
import "context"

// Variables contains user-defined bundle configuration variables.
type Variables map[string]any

// PackageStagingRootProvider optionally identifies a directory where package
// staging can be colocated with loader-owned immutable content. An empty return
// value means no preferred staging location, so deployment uses the configured
// temporary directory instead.
type PackageStagingRootProvider interface {
	PackageStagingRoot(ctx context.Context) string
}

// GlobalOptions holds process-wide settings retained for compatibility with
// bundle configuration consumers. Command behavior is resolved into ConfigOptions.
type GlobalOptions struct {
	LogLevel string
	Prompt   bool
}

// UDSBundleConfig is the resolved public bundle configuration.
type UDSBundleConfig struct {
	Global                *GlobalOptions
	Options               *ConfigOptions
	SignatureVerification *VerificationPolicy
	Variables             Variables
}

// ConfigOptions holds bundle operation settings.
type ConfigOptions struct {
	LogLevel      string
	Architecture  string
	PlainHTTP     bool
	SkipTLSVerify bool
	TmpDir        string
	Concurrency   int
}

// InspectOptions configures inspection of a built bundle.
type InspectOptions struct {
	Source                    string
	Config                    *UDSBundleConfig
	Verification              VerificationPolicy
	SkipSignatureVerification bool
	Streams                   iostreams.IOStreams
}

// InspectResult represents the output of a bundle inspect operation.
type InspectResult struct {
	Name             string                  `json:"name" yaml:"name" text:"Name"`
	Description      string                  `json:"description,omitempty" yaml:"description,omitempty" text:"Description,omitempty"`
	Version          string                  `json:"version,omitempty" yaml:"version,omitempty" text:"Version,omitempty"`
	ArtifactDigest   string                  `json:"artifactDigest,omitempty" yaml:"artifactDigest,omitempty" text:"Artifact Digest,omitempty"`
	ReconfiguredFrom string                  `json:"reconfiguredFrom,omitempty" yaml:"reconfiguredFrom,omitempty" text:"Reconfigured From,omitempty"`
	BundleSignature  *BundleSignatureSummary `json:"bundleSignature,omitempty" yaml:"bundleSignature,omitempty" text:"Bundle Signature,omitempty"`
	Packages         []PackageSummary        `json:"packages" yaml:"packages" text:"Packages"`
}

// BundleSignatureSummary reports bundle signature status.
// Package metadata is not proof of bundle integrity.
type BundleSignatureSummary struct {
	Status string `json:"status" yaml:"status" text:"Status"`
}

const (
	// BundleSignatureStatusVerified means the bundle signature matched the configured policy.
	BundleSignatureStatusVerified = "verified"
	// BundleSignatureStatusUnverified is retained as an alias for an unchecked bundle.
	BundleSignatureStatusUnverified = "not_checked"
	// BundleSignatureStatusNotChecked means inspection did not authenticate the bundle.
	BundleSignatureStatusNotChecked = "not_checked"
	// BundleSignatureStatusSkipped means the caller explicitly bypassed verification.
	BundleSignatureStatusSkipped = "skipped"
)

// PackageSummary is a serializable summary of a package within a bundle.
// Packages are listed in deployment order.
type PackageSummary struct {
	Name        string                   `json:"name" yaml:"name" text:"Name"`
	Source      string                   `json:"source" yaml:"source" text:"Source"`
	Namespace   string                   `json:"namespace,omitempty" yaml:"namespace,omitempty" text:"Namespace,omitempty"`
	DependsOn   []string                 `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty" text:"DependsOn,omitempty"`
	ValuesFiles []string                 `json:"valuesFiles,omitempty" yaml:"valuesFiles,omitempty" text:"Value Files,omitempty"`
	Signature   *PackageSignatureSummary `json:"signature,omitempty" yaml:"signature,omitempty" text:"Signature,omitempty"`
}

// PackageSignatureSummary reports package metadata and the verification result
// recorded during bundle creation. Inspect does not perform package signature verification.
type PackageSignatureSummary struct {
	Signed       string `json:"signed" yaml:"signed" text:"Signed"`
	Verification string `json:"verification" yaml:"verification" text:"Verification Posture"`
}

const (
	// PackageSigningStatusSigned means package signing metadata records a signature.
	PackageSigningStatusSigned = "signed"
	// PackageSigningStatusUnsigned means package signing metadata records no signature.
	PackageSigningStatusUnsigned = "unsigned"
	// PackageSigningStatusUnknown means package signing metadata was unavailable or unrecognized.
	PackageSigningStatusUnknown = "unknown"

	// PackageVerificationStatusVerified means package verification metadata records a successful verification.
	PackageVerificationStatusVerified = "verified"
	// PackageVerificationStatusSkipped means package verification was explicitly disabled during bundle creation.
	PackageVerificationStatusSkipped = "skipped"
	// PackageVerificationStatusUnknown means package verification metadata was unavailable or unrecognized.
	PackageVerificationStatusUnknown = "unknown"
)

// SigningMode identifies the credentials used to sign a bundle.
type SigningMode string

const (
	// SigningModeKey signs with a configured private key.
	SigningModeKey SigningMode = "key"
	// SigningModeKeyless signs with a keyless Sigstore identity.
	SigningModeKeyless SigningMode = "keyless"
	// SigningModeUnsigned leaves the bundle unsigned.
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
