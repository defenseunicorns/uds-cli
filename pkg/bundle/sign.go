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

const bundleSignatureFileName = oci.BundleSignatureFileName

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

// SignOptions configures signing an existing bundle artifact.
type SignOptions struct {
	Source  string
	Signing SigningOptions
	Config  *UDSBundleConfig
	TmpDir  string
	Streams iostreams.IOStreams
}

// warnSkippedSignatureVerification reports the insecure bundle verification bypass.
func warnSkippedSignatureVerification(streams iostreams.IOStreams) {
	streams.Warn("signature verification was skipped; bundle integrity and origin are not established")
}

// Sign adds Sigstore bundle evidence to a local bundle archive.
func Sign(ctx context.Context, opts SignOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	if oci.IsOCIReference(opts.Source) {
		if err := validateOCIReference(opts.Source); err != nil {
			return fmt.Errorf("%w %q: %w", ErrSignBundle, opts.Source, err)
		}
		if opts.Config == nil || opts.Config.Options == nil {
			return fmt.Errorf("%w %q: config is required for OCI bundle signing", ErrSignBundle, opts.Source)
		}
		if err := signOCI(ctx, opts); err != nil {
			return fmt.Errorf("%w %q: %w", ErrSignBundle, opts.Source, err)
		}
		return nil
	}

	workspace, err := os.MkdirTemp(opts.TmpDir, "uds-bundle-sign-*")
	if err != nil {
		return fmt.Errorf("%w %q: creating signing workspace under %q: %w", ErrSignBundle, opts.Source, opts.TmpDir, err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()
	if err := artifact.ExtractTarZst(ctx, opts.Streams, opts.Source, workspace); err != nil {
		return fmt.Errorf("%w %q: extracting into %q: %w", ErrSignBundle, opts.Source, workspace, err)
	}

	indexPath := filepath.Join(workspace, "oci", "index.json")
	index, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("%w %q: reading bundle index %q: %w", ErrSignBundle, opts.Source, indexPath, err)
	}
	if err := validateLocalBundleIndex(index); err != nil {
		return fmt.Errorf("%w %q: %w", ErrSignBundle, opts.Source, err)
	}
	layoutPath := filepath.Join(workspace, "oci")
	if err := oci.VerifyLocalLayoutGraph(ctx, layoutPath, index); err != nil {
		return fmt.Errorf("%w %q: verifying bundle content before signing in OCI layout %q: %w", ErrSignBundle, opts.Source, layoutPath, err)
	}

	evidencePath := filepath.Join(workspace, bundleSignatureFileName)
	if err := signBundleIndex(ctx, indexPath, evidencePath, opts.Signing); err != nil {
		return fmt.Errorf("%w %q using %s mode: %w", ErrSignBundle, opts.Source, opts.Signing.Mode, err)
	}
	if err := artifact.WriteTarZst(ctx, opts.Streams, opts.Source, workspace); err != nil {
		return fmt.Errorf("%w %q: writing signed bundle from %q: %w", ErrSignBundle, opts.Source, workspace, err)
	}
	return nil
}

func signOCI(ctx context.Context, opts SignOptions) error {
	repo, err := oci.NewRemoteRepository(ctx, oci.TrimScheme(opts.Source), toInternalConfigOptions(*opts.Config.Options))
	if err != nil {
		return fmt.Errorf("connecting to registry for %q: %w", opts.Source, err)
	}
	reference, err := oci.ReferenceIdentifier(opts.Source)
	if err != nil {
		return err
	}
	child, index, err := oci.ResolveBundleChild(ctx, repo, reference, opts.Config.Options.Architecture)
	if err != nil {
		return fmt.Errorf("resolving bundle %q for architecture %q: %w", opts.Source, opts.Config.Options.Architecture, err)
	}
	if err := validateBundleIndex(index); err != nil {
		return fmt.Errorf("validating resolved child %s for %q: %w", child.Digest, opts.Source, err)
	}

	workspace, err := os.MkdirTemp(opts.TmpDir, "uds-bundle-oci-sign-*")
	if err != nil {
		return fmt.Errorf("creating OCI signing workspace for %q under %q: %w", opts.Source, opts.TmpDir, err)
	}
	defer func() { _ = os.RemoveAll(workspace) }()
	indexPath := filepath.Join(workspace, "index.json")
	if err := os.WriteFile(indexPath, index, 0o600); err != nil {
		return fmt.Errorf("writing child %s index to %q for signing: %w", child.Digest, indexPath, err)
	}
	evidencePath := filepath.Join(workspace, bundleSignatureFileName)
	if err := signBundleIndex(ctx, indexPath, evidencePath, opts.Signing); err != nil {
		return err
	}
	evidence, err := os.ReadFile(evidencePath)
	if err != nil {
		return fmt.Errorf("reading signature evidence %q for child %s: %w", evidencePath, child.Digest, err)
	}
	if err := oci.PublishBundleSignature(ctx, repo, child, evidence, opts.Signing.Overwrite); err != nil {
		return fmt.Errorf("publishing signature for %q child %s (overwrite=%t): %w", opts.Source, child.Digest, opts.Signing.Overwrite, err)
	}
	return nil
}

func signBundleIndex(ctx context.Context, indexPath, evidencePath string, options SigningOptions) error {
	signOpts := signingOptions(options)
	signOpts.BundlePath = evidencePath
	if _, err := signing.CosignSignBlobWithOptions(ctx, indexPath, signOpts); err != nil {
		return fmt.Errorf("signing bundle index %q with %s mode, evidence %q: %w", indexPath, options.Mode, evidencePath, err)
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
		return fmt.Errorf("signing key cannot be combined with keyless signing: %w", ErrInvalidSigningOptions)
	}
	if o.Mode == SigningModeKey {
		if strings.TrimSpace(o.Key) == "" {
			return fmt.Errorf("signing key is required for key signing: %w", ErrInvalidSigningOptions)
		}
		return nil
	}
	if o.Mode == SigningModeKeyless || o.Mode == SigningModeUnsigned {
		return nil
	}
	return fmt.Errorf("signing mode %q must be key, keyless, or unsigned: %w", o.Mode, ErrInvalidSigningOptions)
}

// Validate validates SignOptions.
func (o SignOptions) Validate() error {
	if o.Source == "" {
		return fmt.Errorf("source is required: %w", ErrSourceRequired)
	}
	if err := o.Signing.Validate(); err != nil {
		return err
	}
	if o.Signing.Mode == SigningModeUnsigned {
		return fmt.Errorf("bundle sign does not accept unsigned mode: %w", ErrInvalidSigningOptions)
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
