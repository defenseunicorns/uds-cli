// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	"github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/signing"
)

// BundleSignatureFileName is the archive-root filename for bundle signature evidence.
const BundleSignatureFileName = oci.BundleSignatureFileName

// WarnSkippedSignatureVerification reports the insecure bundle verification bypass.
func WarnSkippedSignatureVerification(streams iostreams.IOStreams) {
	streams.Warn("signature verification was skipped; bundle integrity and origin are not established")
}

// Sign adds Sigstore bundle evidence to a local bundle archive.
func Sign(ctx context.Context, opts SignOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if IsOCIReference(opts.Source) {
		if opts.Config == nil || opts.Config.Options == nil {
			return fmt.Errorf("config is required for OCI bundle signing")
		}
		return signOCI(ctx, opts)
	}

	workspace, err := os.MkdirTemp(opts.TmpDir, "uds-bundle-sign-*")
	if err != nil {
		return fmt.Errorf("creating signing workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()
	if err := artifact.ExtractTarZst(ctx, opts.Streams, opts.Source, workspace); err != nil {
		return fmt.Errorf("extracting bundle: %w", err)
	}

	indexPath := filepath.Join(workspace, "oci", "index.json")
	index, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("reading bundle index: %w", err)
	}
	if err := validateLocalBundleIndex(index); err != nil {
		return err
	}
	if err := oci.VerifyLocalLayoutGraph(ctx, filepath.Join(workspace, "oci"), index); err != nil {
		return fmt.Errorf("verifying bundle content before signing: %w", err)
	}

	evidencePath := filepath.Join(workspace, BundleSignatureFileName)
	if err := signBundleIndex(ctx, indexPath, evidencePath, opts.Signing); err != nil {
		return err
	}
	if err := artifact.WriteTarZst(ctx, opts.Streams, opts.Source, workspace); err != nil {
		return fmt.Errorf("writing signed bundle: %w", err)
	}
	return nil
}

func signOCI(ctx context.Context, opts SignOptions) error {
	repo, err := oci.NewRemoteRepository(ctx, TrimScheme(opts.Source), toInternalConfigOptions(*opts.Config.Options))
	if err != nil {
		return fmt.Errorf("connecting to registry: %w", err)
	}
	reference, err := oci.ReferenceIdentifier(opts.Source)
	if err != nil {
		return err
	}
	child, index, err := oci.ResolveBundleChild(ctx, repo, reference, opts.Config.Options.Architecture)
	if err != nil {
		return fmt.Errorf("resolving bundle from %s: %w", opts.Source, err)
	}
	if err := validateBundleIndex(index); err != nil {
		return err
	}

	workspace, err := os.MkdirTemp(opts.TmpDir, "uds-bundle-oci-sign-*")
	if err != nil {
		return fmt.Errorf("creating OCI signing workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()
	indexPath := filepath.Join(workspace, "index.json")
	if err := os.WriteFile(indexPath, index, 0o600); err != nil {
		return fmt.Errorf("writing bundle index for signing: %w", err)
	}
	evidencePath := filepath.Join(workspace, BundleSignatureFileName)
	if err := signBundleIndex(ctx, indexPath, evidencePath, opts.Signing); err != nil {
		return err
	}
	evidence, err := os.ReadFile(evidencePath)
	if err != nil {
		return fmt.Errorf("reading bundle signature evidence: %w", err)
	}
	if err := oci.PublishBundleSignature(ctx, repo, child, evidence, opts.Signing.Overwrite); err != nil {
		return fmt.Errorf("publishing bundle signature: %w", err)
	}
	return nil
}

func signBundleIndex(ctx context.Context, indexPath, evidencePath string, options SigningOptions) error {
	signOpts := signingOptions(options)
	signOpts.BundlePath = evidencePath
	if _, err := signing.CosignSignBlobWithOptions(ctx, indexPath, signOpts); err != nil {
		return fmt.Errorf("signing bundle index: %w", err)
	}
	return nil
}

func signingOptions(options SigningOptions) signing.SignBlobOptions {
	signOpts := signing.DefaultSignBlobOptions()
	signOpts.Key = options.Key
	signOpts.Password = options.KeyPassword
	signOpts.Keyless = options.Mode == SigningModeKeyless
	signOpts.Fulcio.IdentityToken = options.IdentityToken
	signOpts.Fulcio.URL = options.FulcioURL
	signOpts.Fulcio.AuthFlow = options.FulcioAuthFlow
	signOpts.OIDC.Issuer = options.OIDCIssuer
	if options.OIDCClientID != "" {
		signOpts.OIDC.ClientID = options.OIDCClientID
	}
	signOpts.Rekor.URL = options.RekorURL
	signOpts.TSAServerURL = options.TSAServerURL
	signOpts.Overwrite = options.Overwrite
	signOpts.TlogUpload = options.Mode == SigningModeKeyless
	signOpts.UseSigningConfig = signOpts.TlogUpload
	return signOpts
}

// Validate validates signing options.
func (o SigningOptions) Validate() error {
	if o.Mode == SigningModeKeyless && strings.TrimSpace(o.Key) != "" {
		return fmt.Errorf("signing key cannot be combined with keyless signing")
	}
	if o.Mode == SigningModeKey {
		if strings.TrimSpace(o.Key) == "" {
			return fmt.Errorf("signing key is required for key signing")
		}
		return nil
	}
	if o.Mode == SigningModeKeyless || o.Mode == SigningModeUnsigned {
		return nil
	}
	return fmt.Errorf("signing mode must be key, keyless, or unsigned")
}

// Validate validates SignOptions.
func (o SignOptions) Validate() error {
	if o.Source == "" {
		return fmt.Errorf("source is required")
	}
	if err := o.Signing.Validate(); err != nil {
		return err
	}
	if o.Signing.Mode == SigningModeUnsigned {
		return fmt.Errorf("bundle sign does not accept unsigned mode")
	}
	return nil
}

func validateBundleIndex(index []byte) error {
	_, err := parseBundleIndex(index)
	return err
}

func validateLocalBundleIndex(index []byte) error {
	parsed, err := parseBundleIndex(index)
	if err != nil {
		return err
	}
	if _, _, err := oci.FindBundleDefinition(parsed); err != nil {
		return fmt.Errorf("validating bundle definition before signing: %w", err)
	}
	return nil
}

func parseBundleIndex(index []byte) (ocispec.Index, error) {
	var parsed ocispec.Index
	if err := json.Unmarshal(index, &parsed); err != nil {
		return ocispec.Index{}, fmt.Errorf("parsing bundle index: %w", err)
	}
	if !oci.IsBundleIndex(parsed) {
		return ocispec.Index{}, fmt.Errorf("artifact is not a UDS bundle")
	}
	return parsed, nil
}
