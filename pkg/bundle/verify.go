// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	"github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/zarf-dev/zarf/src/pkg/signing"
)

// Verify verifies local bundle signature evidence and the complete OCI graph.
func Verify(ctx context.Context, opts VerifyOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if IsOCIReference(opts.Source) {
		if opts.Config == nil {
			return fmt.Errorf("config is required for OCI bundle verification")
		}
		workspace, err := os.MkdirTemp(opts.TmpDir, "uds-bundle-oci-verify-*")
		if err != nil {
			return fmt.Errorf("creating OCI verification workspace: %w", err)
		}
		defer func() { _ = os.RemoveAll(workspace) }()
		pulled, err := Pull(ctx, opts.Source, workspace, PullOptions{
			Config:       opts.Config,
			Verification: opts.Policy,
			Streams:      opts.Streams,
		})
		if err != nil {
			return fmt.Errorf("pulling bundle for verification: %w", err)
		}
		opts.Source = pulled.OutputPath
		return Verify(ctx, opts)
	}

	workspace, err := os.MkdirTemp(opts.TmpDir, "uds-bundle-verify-*")
	if err != nil {
		return fmt.Errorf("creating verification workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()
	signatureEntries, err := artifact.CountTarZstEntries(ctx, opts.Source, BundleSignatureFileName)
	if err != nil {
		return fmt.Errorf("checking bundle signature evidence: %w", err)
	}
	if signatureEntries > 1 {
		return fmt.Errorf("expected exactly one bundle signature evidence entry, found %d", signatureEntries)
	}
	if err := artifact.ExtractTarZst(ctx, opts.Streams, opts.Source, workspace); err != nil {
		return fmt.Errorf("extracting bundle: %w", err)
	}
	indexPath := filepath.Join(workspace, "oci", "index.json")
	index, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("reading bundle index: %w", err)
	}
	if err := validateBundleIndex(index); err != nil {
		return err
	}
	evidencePath := filepath.Join(workspace, BundleSignatureFileName)
	evidenceInfo, err := os.Stat(evidencePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrBundleNotSigned
		}
		return fmt.Errorf("accessing bundle signature evidence: %w", err)
	}
	if evidenceInfo.Size() > oci.MaxFetchBytesSize {
		return fmt.Errorf("bundle signature evidence is %d bytes, larger than the %d byte buffered read limit", evidenceInfo.Size(), oci.MaxFetchBytesSize)
	}
	evidence, err := os.ReadFile(evidencePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrBundleNotSigned
		}
		return fmt.Errorf("accessing bundle signature evidence: %w", err)
	}
	if err := verifySignature(ctx, index, evidence, opts.Policy, opts.TmpDir); err != nil {
		return err
	}
	if err := oci.VerifyLocalLayoutGraph(ctx, filepath.Join(workspace, "oci"), index); err != nil {
		return fmt.Errorf("verifying bundle content: %w", err)
	}
	return nil
}

func verifySignature(ctx context.Context, index, evidence []byte, policy VerificationPolicy, tmpDir string) error {
	workspace, err := os.MkdirTemp(tmpDir, "uds-bundle-signature-verify-*")
	if err != nil {
		return fmt.Errorf("creating signature verification workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	indexPath := filepath.Join(workspace, "index.json")
	if err := os.WriteFile(indexPath, index, 0o600); err != nil {
		return fmt.Errorf("writing bundle index for signature verification: %w", err)
	}
	evidencePath := filepath.Join(workspace, BundleSignatureFileName)
	if err := os.WriteFile(evidencePath, evidence, 0o600); err != nil {
		return fmt.Errorf("writing bundle signature evidence: %w", err)
	}

	verifyOpts, err := validationOptions(policy, workspace)
	if err != nil {
		return err
	}
	verifyOpts.BundlePath = evidencePath
	if err := signing.CosignVerifyBlobWithOptions(ctx, indexPath, verifyOpts); err != nil {
		return fmt.Errorf("verifying bundle signature: %w", err)
	}
	return nil
}

// Validate validates verification policy.
func (p VerificationPolicy) Validate() error {
	hasKey := strings.TrimSpace(p.PublicKey) != ""
	hasKeyless := p.Keyless != nil
	if hasKey == hasKeyless {
		return fmt.Errorf("signature verification must configure exactly one of public key or keyless")
	}
	if !hasKeyless {
		return nil
	}
	keyless := p.Keyless
	if err := exactlyOne(keyless.CertificateIdentity, keyless.CertificateIdentityRegexp, "certificate identity"); err != nil {
		return err
	}
	if strings.TrimSpace(keyless.CertificateIdentityRegexp) != "" {
		if _, err := regexp.Compile(keyless.CertificateIdentityRegexp); err != nil {
			return fmt.Errorf("invalid certificate identity regexp: %w", err)
		}
	}
	if err := exactlyOne(keyless.CertificateOIDCIssuer, keyless.CertificateOIDCIssuerRegexp, "certificate OIDC issuer"); err != nil {
		return err
	}
	if strings.TrimSpace(keyless.CertificateOIDCIssuerRegexp) != "" {
		if _, err := regexp.Compile(keyless.CertificateOIDCIssuerRegexp); err != nil {
			return fmt.Errorf("invalid certificate OIDC issuer regexp: %w", err)
		}
	}
	return nil
}

func (p VerificationPolicy) configured() bool {
	return strings.TrimSpace(p.PublicKey) != "" || p.Keyless != nil
}

// Validate validates VerifyOptions.
func (o VerifyOptions) Validate() error {
	if o.Source == "" {
		return fmt.Errorf("source is required")
	}
	return o.Policy.Validate()
}

func validationOptions(policy VerificationPolicy, tmpDir string) (signing.VerifyBlobOptions, error) {
	options := signing.DefaultVerifyBlobOptions()
	options.TempDir = tmpDir
	if policy.PublicKey != "" {
		// Key signatures do not upload to Rekor.
		options.CommonVerifyOptions.IgnoreTlog = true
		options.CertVerify.IgnoreSCT = true
		keyPath := filepath.Join(tmpDir, "bundle-public-key.pem")
		if err := os.WriteFile(keyPath, []byte(policy.PublicKey), 0o600); err != nil {
			return signing.VerifyBlobOptions{}, fmt.Errorf("writing public key: %w", err)
		}
		options.Key = keyPath
		return options, nil
	}
	keyless := policy.Keyless
	options.CommonVerifyOptions.IgnoreTlog = false
	options.CertVerify.IgnoreSCT = false
	options.CertVerify.CertIdentity = keyless.CertificateIdentity
	options.CertVerify.CertIdentityRegexp = keyless.CertificateIdentityRegexp
	options.CertVerify.CertOidcIssuer = keyless.CertificateOIDCIssuer
	options.CertVerify.CertOidcIssuerRegexp = keyless.CertificateOIDCIssuerRegexp
	if keyless.TrustedRoot != "" {
		rootPath := filepath.Join(tmpDir, "trusted-root.json")
		if err := os.WriteFile(rootPath, []byte(keyless.TrustedRoot), 0o600); err != nil {
			return signing.VerifyBlobOptions{}, fmt.Errorf("writing trusted root: %w", err)
		}
		options.CommonVerifyOptions.TrustedRootPath = rootPath
	}
	return options, nil
}

func exactlyOne(first, second, name string) error {
	if (strings.TrimSpace(first) == "") == (strings.TrimSpace(second) == "") {
		return fmt.Errorf("keyless verification requires exactly one %s or %s regexp", name, name)
	}
	return nil
}
