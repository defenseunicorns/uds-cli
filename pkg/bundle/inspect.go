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
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

const bundleSignatureNotCheckedWarning = "bundle signature verification was not performed; package verification policy and signing metadata do not establish bundle integrity"

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
			return nil, err
		}
		status = BundleSignatureStatusSkipped
		warnSkippedSignatureVerification(streams)
	} else if policy.configured() {
		verifiedSource, cleanup, err := stageInspectSource(ctx, opts, policy, streams)
		if err != nil {
			return nil, err
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
		return nil, err
	}

	result := &InspectResult{
		Name:             internalResult.Bundle.Metadata.Name,
		Description:      internalResult.Bundle.Metadata.Description,
		Version:          internalResult.Bundle.Metadata.Version,
		ArtifactDigest:   internalResult.ArtifactDigest,
		ReconfiguredFrom: internalResult.ReconfiguredFrom,
		BundleSignature:  &BundleSignatureSummary{Status: status},
		Packages:         make([]PackageSummary, len(internalResult.Packages)),
	}
	for i, pkg := range internalResult.Packages {
		summary, ok := internalResult.PackageSignatures[pkg.Name]
		if !ok {
			return nil, fmt.Errorf("package %q was not found in inspected bundle", pkg.Name)
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
		return fmt.Errorf("source must not be empty")
	}
	if err := validateConfig(o.Config); err != nil {
		return err
	}
	if !udsoci.IsOCIReference(o.Source) && !artifact.IsTarZst(o.Source) {
		return fmt.Errorf("source must be a .tar.zst bundle artifact or OCI reference")
	}
	if udsoci.IsOCIReference(o.Source) {
		if _, err := udsoci.ReferenceIdentifier(o.Source); err != nil {
			return err
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
