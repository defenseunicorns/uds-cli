// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	"github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/zarf-dev/zarf/src/pkg/signing"
)

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

// VerifyOptions configures verification of a bundle artifact.
type VerifyOptions struct {
	Source  string
	Policy  VerificationPolicy
	Config  *UDSBundleConfig
	TmpDir  string
	Streams iostreams.IOStreams
}

// Verify verifies local bundle signature evidence and the complete OCI graph.
func Verify(ctx context.Context, opts VerifyOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if oci.IsOCIReference(opts.Source) {
		if err := validateOCIReference(opts.Source); err != nil {
			return fmt.Errorf("%w %q: %w", ErrVerifyBundle, opts.Source, err)
		}
		if opts.Config == nil {
			return fmt.Errorf("%w: config is required for OCI bundle verification", ErrVerifyBundle)
		}
		workspace, err := os.MkdirTemp(opts.TmpDir, "uds-bundle-oci-verify-*")
		if err != nil {
			return fmt.Errorf("%w: creating OCI verification workspace: %w", ErrVerifyBundle, err)
		}
		defer func() { _ = os.RemoveAll(workspace) }()
		pulled, err := Pull(ctx, opts.Source, workspace, PullOptions{
			Config:                    opts.Config,
			Verification:              opts.Policy,
			SkipSignatureVerification: false,
			Streams:                   opts.Streams,
		})
		if err != nil {
			if errors.Is(err, oci.ErrBundleSignatureNotFound) && !errors.Is(err, ErrBundleNotSigned) {
				return fmt.Errorf("%w: pulling bundle for verification: %w: %w", ErrVerifyBundle, ErrBundleNotSigned, err)
			}
			return fmt.Errorf("%w: pulling bundle for verification: %w", ErrVerifyBundle, err)
		}
		opts.Source = pulled.OutputPath
		return Verify(ctx, opts)
	}

	workspace, err := os.MkdirTemp(opts.TmpDir, "uds-bundle-verify-*")
	if err != nil {
		return fmt.Errorf("%w: creating verification workspace: %w", ErrVerifyBundle, err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()
	signatureEntries, err := artifact.CountTarZstEntries(ctx, opts.Source, bundleSignatureFileName)
	if err != nil {
		return fmt.Errorf("%w: checking bundle signature evidence: %w", ErrVerifyBundle, err)
	}
	if signatureEntries > 1 {
		return fmt.Errorf("%w: expected exactly one bundle signature evidence entry, found %d", ErrVerifyBundle, signatureEntries)
	}
	if err := artifact.ExtractTarZst(ctx, opts.Streams, opts.Source, workspace); err != nil {
		return fmt.Errorf("%w: extracting bundle: %w", ErrVerifyBundle, err)
	}
	indexPath := filepath.Join(workspace, "oci", "index.json")
	index, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("%w: reading bundle index: %w", ErrVerifyBundle, err)
	}
	if err := validateBundleIndex(index); err != nil {
		return fmt.Errorf("%w %q: %w", ErrVerifyBundle, opts.Source, err)
	}
	evidencePath := filepath.Join(workspace, bundleSignatureFileName)
	evidenceInfo, err := os.Stat(evidencePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %w: %w", ErrVerifyBundle, ErrBundleNotSigned, err)
		}
		return fmt.Errorf("%w: accessing bundle signature evidence: %w", ErrVerifyBundle, err)
	}
	if evidenceInfo.Size() > oci.MaxFetchBytesSize {
		return fmt.Errorf("%w: bundle signature evidence is %d bytes, larger than the %d byte buffered read limit", ErrVerifyBundle, evidenceInfo.Size(), oci.MaxFetchBytesSize)
	}
	evidence, err := os.ReadFile(evidencePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %w: %w", ErrVerifyBundle, ErrBundleNotSigned, err)
		}
		return fmt.Errorf("%w: accessing bundle signature evidence: %w", ErrVerifyBundle, err)
	}
	if err := verifySignature(ctx, index, evidence, opts.Policy, opts.TmpDir); err != nil {
		return fmt.Errorf("%w %q: %w", ErrVerifyBundle, opts.Source, err)
	}
	if err := oci.VerifyLocalLayoutGraph(ctx, filepath.Join(workspace, "oci"), index); err != nil {
		return fmt.Errorf("%w: verifying bundle content: %w", ErrVerifyBundle, err)
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
	evidencePath := filepath.Join(workspace, bundleSignatureFileName)
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
		return fmt.Errorf("signature verification must configure exactly one of public key or keyless: %w", ErrInvalidVerificationPolicy)
	}
	if !hasKeyless {
		return nil
	}
	keyless := p.Keyless
	if err := exactlyOne(keyless.CertificateIdentity, keyless.CertificateIdentityRegexp, "certificate identity"); err != nil {
		return fmt.Errorf("%w for keyless verification: %w", ErrInvalidVerificationPolicy, err)
	}
	if strings.TrimSpace(keyless.CertificateIdentityRegexp) != "" {
		if _, err := regexp.Compile(keyless.CertificateIdentityRegexp); err != nil {
			return fmt.Errorf("%w: invalid certificate identity regexp: %w", ErrInvalidVerificationPolicy, err)
		}
	}
	if err := exactlyOne(keyless.CertificateOIDCIssuer, keyless.CertificateOIDCIssuerRegexp, "certificate OIDC issuer"); err != nil {
		return fmt.Errorf("%w for keyless verification: %w", ErrInvalidVerificationPolicy, err)
	}
	if strings.TrimSpace(keyless.CertificateOIDCIssuerRegexp) != "" {
		if _, err := regexp.Compile(keyless.CertificateOIDCIssuerRegexp); err != nil {
			return fmt.Errorf("%w: invalid certificate OIDC issuer regexp: %w", ErrInvalidVerificationPolicy, err)
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
		return fmt.Errorf("source is required: %w", ErrSourceRequired)
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
