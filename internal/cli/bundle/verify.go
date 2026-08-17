// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"

	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// VerifyOptions holds options for the bundle verify command.
type VerifyOptions struct {
	Source                    string
	Config                    *bundlepkg.UDSBundleConfig
	PublicKey                 string
	Identity                  string
	IdentityRE                string
	Issuer                    string
	IssuerRE                  string
	TrustedRoot               string
	SkipSignatureVerification bool

	iostreams.IOStreams
}

// NewVerifyCommand creates the bundle verify command.
func NewVerifyCommand(streams iostreams.IOStreams) *cobra.Command {
	o := &VerifyOptions{IOStreams: streams}
	cmd := &cobra.Command{
		Use:   "verify <bundle-artifact>",
		Short: "Verify a created bundle artifact",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run(cmd.Context()))
		},
	}
	addVerificationFlags(cmd, o, false)
	return cmd
}

// Complete fills verification options from command arguments, config, and flags.
func (o *VerifyOptions) Complete(cmd *cobra.Command, args []string) error {
	o.Source = args[0]
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, _, err := NewConfigResolver().Resolve(ctx, o.IOStreams, SnapshotFlags(cmd), "")
	if err != nil {
		return err
	}
	o.Config = cfg
	return nil
}

// Validate validates verification command options.
func (o *VerifyOptions) Validate() error {
	if o.Source == "" {
		return fmt.Errorf("bundle artifact is required")
	}
	_, err := o.policy()
	return err
}

// Run verifies a bundle artifact.
func (o *VerifyOptions) Run(ctx context.Context) error {
	policy, err := o.policy()
	if err != nil {
		return err
	}
	return bundlepkg.Verify(ctx, bundlepkg.VerifyOptions{Source: o.Source, Policy: policy, Config: o.Config, TmpDir: o.Config.Options.TmpDir, Streams: o.IOStreams})
}

func addVerificationFlags(cmd *cobra.Command, o *VerifyOptions, allowSkip bool) {
	cmd.Flags().StringVar(&o.PublicKey, "public-key", "", "path to trusted bundle public key")
	cmd.Flags().StringVar(&o.Identity, "certificate-identity", "", "trusted keyless certificate identity")
	cmd.Flags().StringVar(&o.IdentityRE, "certificate-identity-regexp", "", "trusted keyless certificate identity regexp")
	cmd.Flags().StringVar(&o.Issuer, "certificate-oidc-issuer", "", "trusted keyless OIDC issuer")
	cmd.Flags().StringVar(&o.IssuerRE, "certificate-oidc-issuer-regexp", "", "trusted keyless OIDC issuer regexp")
	cmd.Flags().StringVar(&o.TrustedRoot, "trusted-root", "", "path to Sigstore trusted root")
	if allowSkip {
		cmd.Flags().BoolVar(&o.SkipSignatureVerification, "skip-signature-verification", false, "skip bundle signature verification (insecure)")
	}
}

func (o *VerifyOptions) policy() (bundlepkg.VerificationPolicy, error) {
	policy := bundlepkg.VerificationPolicy{}
	if o.Config != nil && o.Config.SignatureVerification != nil {
		policy = *o.Config.SignatureVerification
	}
	if o.PublicKey != "" {
		data, err := os.ReadFile(o.PublicKey)
		if err != nil {
			return bundlepkg.VerificationPolicy{}, fmt.Errorf("reading public key: %w", err)
		}
		policy = bundlepkg.VerificationPolicy{PublicKey: string(data)}
	}
	if o.Identity != "" || o.IdentityRE != "" || o.Issuer != "" || o.IssuerRE != "" || o.TrustedRoot != "" {
		policy.PublicKey = ""
		keyless := bundlepkg.KeylessVerification{}
		if policy.Keyless != nil {
			keyless = *policy.Keyless
		}
		if o.Identity != "" {
			keyless.CertificateIdentity = o.Identity
		}
		if o.IdentityRE != "" {
			keyless.CertificateIdentityRegexp = o.IdentityRE
		}
		if o.Issuer != "" {
			keyless.CertificateOIDCIssuer = o.Issuer
		}
		if o.IssuerRE != "" {
			keyless.CertificateOIDCIssuerRegexp = o.IssuerRE
		}
		if o.TrustedRoot != "" {
			data, err := os.ReadFile(o.TrustedRoot)
			if err != nil {
				return bundlepkg.VerificationPolicy{}, fmt.Errorf("reading trusted root: %w", err)
			}
			keyless.TrustedRoot = string(data)
		}
		policy.Keyless = &keyless
	}
	if err := policy.Validate(); err != nil {
		return bundlepkg.VerificationPolicy{}, err
	}
	return policy, nil
}
