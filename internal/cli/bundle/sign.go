// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// SignOptions holds options for the bundle sign command.
type SignOptions struct {
	Source  string
	Signing bundlepkg.SigningOptions
	Config  *bundlepkg.UDSBundleConfig

	iostreams.IOStreams
}

// NewSignCommand creates the bundle sign command.
func NewSignCommand(streams iostreams.IOStreams) *cobra.Command {
	o := &SignOptions{IOStreams: streams}
	cmd := &cobra.Command{
		Use:   "sign <bundle-artifact>",
		Short: "Sign a created bundle artifact",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run(cmd.Context()))
		},
	}
	addSigningFlags(cmd, &o.Signing)
	cmd.Flags().BoolVar(&o.Signing.Overwrite, "overwrite", false, "replace existing bundle signature evidence")
	return cmd
}

// Complete fills sign options from command arguments and flags.
func (o *SignOptions) Complete(cmd *cobra.Command, args []string) error {
	o.Source = args[0]
	if err := completeSigningOptions(cmd, &o.Signing); err != nil {
		return err
	}
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

// Validate validates sign options.
func (o *SignOptions) Validate() error {
	if o.Source == "" {
		return fmt.Errorf("bundle artifact is required")
	}
	return o.Signing.Validate()
}

// Run signs a bundle artifact.
func (o *SignOptions) Run(ctx context.Context) error {
	return bundlepkg.Sign(ctx, bundlepkg.SignOptions{Source: o.Source, Signing: o.Signing, Config: o.Config, TmpDir: o.Config.Options.TmpDir, Streams: o.IOStreams})
}

func addSigningFlags(cmd *cobra.Command, options *bundlepkg.SigningOptions) {
	cmd.Flags().StringVar(&options.Key, "signing-key", "", "Cosign private key path or KMS URI")
	cmd.Flags().StringVar(&options.KeyPassword, "signing-key-pass", "", "private key password")
	cmd.Flags().Bool("keyless", false, "sign with an OIDC identity")
	cmd.Flags().StringVar(&options.IdentityToken, "identity-token", "", "OIDC identity token")
	cmd.Flags().StringVar(&options.FulcioURL, "fulcio-url", "", "Fulcio URL")
	cmd.Flags().StringVar(&options.FulcioAuthFlow, "fulcio-auth-flow", "", "Fulcio authentication flow")
	cmd.Flags().StringVar(&options.OIDCIssuer, "oidc-issuer", "", "OIDC issuer")
	cmd.Flags().StringVar(&options.OIDCClientID, "oidc-client-id", "", "OIDC client ID")
	cmd.Flags().StringVar(&options.RekorURL, "rekor-url", "", "Rekor URL")
	cmd.Flags().StringVar(&options.TSAServerURL, "tsa-server-url", "", "timestamp authority URL")
}

func completeSigningOptions(cmd *cobra.Command, options *bundlepkg.SigningOptions) error {
	keyless, err := cmd.Flags().GetBool("keyless")
	if err != nil {
		return err
	}
	if options.Key != "" && keyless {
		return fmt.Errorf("--signing-key and --keyless cannot be combined")
	}
	if keyless {
		options.Mode = bundlepkg.SigningModeKeyless
		return nil
	}
	if options.Key == "" {
		return fmt.Errorf("one of --signing-key or --keyless is required")
	}
	options.Mode = bundlepkg.SigningModeKey
	return nil
}
