// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

const bundleSignatureNotCheckedWarning = "bundle signature verification was not performed; package verification policy and signing metadata do not establish bundle integrity"

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
	Bundle           *spec.UDSBundle         `json:"-" yaml:"-" text:"-"`
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

// Inspect reads metadata from a built local or OCI bundle. When a verification
// policy is provided, it verifies the bundle before parsing its metadata.
func Inspect(ctx context.Context, opts InspectOptions) (*InspectResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	streams := logger.Bind(opts.Streams, opts.Config.Options.LogLevel)
	policy := opts.verificationPolicy()

	status := BundleSignatureStatusNotChecked
	if opts.SkipSignatureVerification {
		if err := checkSkippedSignatureEvidence(ctx, opts); err != nil {
			return nil, fmt.Errorf("%w %q: %w", ErrInspectBundle, opts.Source, err)
		}
		status = BundleSignatureStatusSkipped
		warnSkippedSignatureVerification(streams)
	} else if policy.configured() {
		verifiedSource, cleanup, err := stageInspectSource(ctx, opts, policy, streams)
		if err != nil {
			return nil, fmt.Errorf("%w %q: %w", ErrInspectBundle, opts.Source, err)
		}
		defer cleanup()
		opts.Source = verifiedSource
		status = BundleSignatureStatusVerified
	} else {
		streams.Warn(bundleSignatureNotCheckedWarning)
	}

	internalResult, err := artifact.Inspect(ctx, artifact.InspectOptions{
		Source:  opts.Source,
		Config:  toInternalConfig(opts.Config),
		Streams: streams,
	})
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrInspectBundle, opts.Source, err)
	}

	result := &InspectResult{
		Name:             internalResult.Bundle.Metadata.Name,
		Description:      internalResult.Bundle.Metadata.Description,
		Version:          internalResult.Bundle.Metadata.Version,
		ArtifactDigest:   internalResult.ArtifactDigest,
		ReconfiguredFrom: internalResult.ReconfiguredFrom,
		BundleSignature:  &BundleSignatureSummary{Status: status},
		Packages:         make([]PackageSummary, len(internalResult.Packages)),
		Bundle:           internalResult.Bundle,
	}

	for i, pkg := range internalResult.Packages {
		summary, ok := internalResult.PackageSignatures[pkg.Name]
		if !ok {
			return nil, fmt.Errorf("%w %q: package %q was not found in inspected bundle", ErrInspectBundle, opts.Source, pkg.Name)
		}
		dependsOn := make([]string, len(pkg.DependsOn))
		for j, dependency := range pkg.DependsOn {
			dependsOn[j] = dependency.Name
		}
		result.Packages[i] = PackageSummary{
			Name:        pkg.Name,
			Source:      pkg.Source,
			Namespace:   pkg.Namespace,
			DependsOn:   dependsOn,
			ValuesFiles: pkg.ValuesFiles,
			Signature: &PackageSignatureSummary{
				Signed:       packageSigningStatusString(summary.Signed),
				Verification: packageVerificationStatusString(summary.Verification),
			},
		}
	}

	return result, nil
}

// Validate validates inspection options without performing I/O.
func (o InspectOptions) Validate() error {
	if strings.TrimSpace(o.Source) == "" {
		return fmt.Errorf("source is required: %w", ErrSourceRequired)
	}
	if err := validateConfig(o.Config); err != nil {
		return err
	}
	if !udsoci.IsOCIReference(o.Source) && !artifact.IsTarZst(o.Source) {
		return fmt.Errorf("source must be a .tar.zst bundle artifact or OCI reference")
	}
	if udsoci.IsOCIReference(o.Source) {
		if err := validateOCIReference(o.Source); err != nil {
			return fmt.Errorf("%w %q: %w", ErrInspectBundle, o.Source, err)
		}
	}
	if !o.SkipSignatureVerification {
		policy := o.verificationPolicy()
		if policy.configured() {
			return policy.Validate()
		}
	}
	return nil
}

func (o InspectOptions) verificationPolicy() VerificationPolicy {
	if o.Verification.configured() || o.Config.SignatureVerification == nil {
		return o.Verification
	}
	return *o.Config.SignatureVerification
}

func checkSkippedSignatureEvidence(ctx context.Context, opts InspectOptions) error {
	if !udsoci.IsOCIReference(opts.Source) {
		signatureEntries, err := artifact.CountTarZstEntries(ctx, opts.Source, bundleSignatureFileName)
		if err != nil {
			return fmt.Errorf("checking bundle signature evidence: %w", err)
		}
		if signatureEntries > 1 {
			return fmt.Errorf("expected exactly one bundle signature evidence entry, found %d", signatureEntries)
		}
		return nil
	}

	repo, err := udsoci.NewRemoteRepository(ctx, udsoci.TrimScheme(opts.Source), toInternalConfigOptions(*opts.Config.Options))
	if err != nil {
		return fmt.Errorf("checking bundle signature evidence: %w", err)
	}
	reference, err := udsoci.ReferenceIdentifier(opts.Source)
	if err != nil {
		return err
	}
	child, _, err := udsoci.ResolveBundleChild(ctx, repo, reference, opts.Config.Options.Architecture)
	if err != nil {
		return fmt.Errorf("checking bundle signature evidence: %w", err)
	}
	_, err = udsoci.FetchBundleSignature(ctx, repo, child)
	if errors.Is(err, udsoci.ErrBundleSignatureDuplicate) {
		return fmt.Errorf("reading bundle signature evidence: %w", err)
	}
	return nil
}

func stageInspectSource(ctx context.Context, opts InspectOptions, policy VerificationPolicy, streams iostreams.IOStreams) (string, func(), error) {
	workspace, err := os.MkdirTemp(opts.Config.Options.TmpDir, "uds-bundle-inspect-verified-*")
	if err != nil {
		return "", func() {}, fmt.Errorf("creating verification workspace: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(workspace) }

	if udsoci.IsOCIReference(opts.Source) {
		pulled, err := Pull(ctx, opts.Source, workspace, PullOptions{
			Config:       opts.Config,
			Verification: policy,
			Streams:      streams,
		})
		if err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("verifying bundle: %w", err)
		}
		if err := Verify(ctx, VerifyOptions{
			Source:  pulled.OutputPath,
			Policy:  policy,
			Config:  opts.Config,
			TmpDir:  opts.Config.Options.TmpDir,
			Streams: streams,
		}); err != nil {
			cleanup()
			return "", func() {}, fmt.Errorf("verifying bundle: %w", err)
		}
		return pulled.OutputPath, cleanup, nil
	}

	verifiedPath := filepath.Join(workspace, "bundle.tar.zst")
	input, err := os.Open(opts.Source)
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("staging bundle for verification: %w", err)
	}
	output, err := os.OpenFile(verifiedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = input.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("staging bundle for verification: %w", err)
	}
	_, copyErr := io.Copy(output, input)
	closeOutputErr := output.Close()
	closeInputErr := input.Close()
	if copyErr != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("staging bundle for verification: %w", copyErr)
	}
	if closeOutputErr != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("staging bundle for verification: %w", closeOutputErr)
	}
	if closeInputErr != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("staging bundle for verification: %w", closeInputErr)
	}
	if err := Verify(ctx, VerifyOptions{
		Source:  verifiedPath,
		Policy:  policy,
		Config:  opts.Config,
		TmpDir:  opts.Config.Options.TmpDir,
		Streams: streams,
	}); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("verifying bundle: %w", err)
	}
	return verifiedPath, cleanup, nil
}

func packageSigningStatusString(status artifact.PackageSigningStatus) string {
	switch status {
	case artifact.PackageSigningStatusSigned:
		return PackageSigningStatusSigned
	case artifact.PackageSigningStatusUnsigned:
		return PackageSigningStatusUnsigned
	default:
		return PackageSigningStatusUnknown
	}
}

func packageVerificationStatusString(status artifact.PackageVerificationStatus) string {
	switch status {
	case artifact.PackageVerificationStatusVerified:
		return PackageVerificationStatusVerified
	case artifact.PackageVerificationStatusSkipped:
		return PackageVerificationStatusSkipped
	default:
		return PackageVerificationStatusUnknown
	}
}
