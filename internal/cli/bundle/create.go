// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// CreateOptions holds options for the create command.
type CreateOptions struct {
	BundlePath string // Path to bundle file or directory (user input, resolved in Run)
	Prompt     bool
	Config     *bundle.UDSBundleConfig
	Printer    printer.ResourcePrinter
	Signing    bundle.SigningOptions

	iostreams.IOStreams
}

// NewCreateOptions returns a CreateOptions with default values.
func NewCreateOptions(streams iostreams.IOStreams) *CreateOptions {
	return &CreateOptions{
		IOStreams: streams,
	}
}

// NewCreateCommand creates the create command.
func NewCreateCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewCreateOptions(streams)

	cmd := &cobra.Command{
		Use:   "create [directory]",
		Short: "Create a new UDS bundle",
		Long:  "Create a new UDS bundle from an HCL configuration file",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			ctx := cmd.Context()
			util.CheckErr(o.Run(ctx))
		},
	}
	addSigningFlags(cmd, &o.Signing)
	cmd.Flags().Bool("unsigned", false, "create an unsigned bundle")

	return cmd
}

// Complete fills in options from command line args.
func (o *CreateOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundlePath = args[0]
	} else {
		o.BundlePath = "."
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	flags := SnapshotFlags(cmd)
	o.Prompt = flags.Prompt
	cfg, _, err := NewConfigResolver().Resolve(ctx, o.IOStreams, flags, o.BundlePath)
	if err != nil {
		return err
	}
	o.Config = cfg
	if err := completeCreateSigningOptions(cmd, &o.Signing); err != nil {
		return err
	}

	p, err := ResolvePrinter(cmd)
	if err != nil {
		return err
	}
	o.Printer = p

	return nil
}

// Validate validates the options without modifying state.
// Config validation is performed by the library entry point.
func (o *CreateOptions) Validate() error {
	if err := ValidateBundlePath(o.BundlePath); err != nil {
		return err
	}
	return o.Signing.Validate()
}

// Run executes the create command.
func (o *CreateOptions) Run(ctx context.Context) error {
	// Resolve the bundle path
	bundlePath := resolveBundlePath(o.BundlePath)
	o.IOStreams = logger.Bind(o.IOStreams, o.Config.Options.LogLevel)
	if o.Prompt {
		confirmed, err := PromptConfirmation(o.IOStreams, "Create this bundle?")
		if err != nil {
			return err
		}
		if !confirmed {
			o.Info("create cancelled")
			return nil
		}
	}
	o.Info("creating bundle", "source", bundlePath)

	result, err := bundle.Create(ctx, bundlePath, bundle.CreateOptions{
		Config:  o.Config,
		Signing: o.Signing,
		Streams: o.IOStreams,
	})
	if err != nil {
		return err
	}

	return o.Printer.PrintObj(result, o.Out())
}

func completeCreateSigningOptions(cmd *cobra.Command, options *bundle.SigningOptions) error {
	if cmd.Flags().Lookup("unsigned") == nil {
		return nil
	}
	unsigned, err := cmd.Flags().GetBool("unsigned")
	if err != nil {
		return err
	}
	keyless, err := cmd.Flags().GetBool("keyless")
	if err != nil {
		return err
	}
	if unsigned && (options.Key != "" || keyless) {
		return fmt.Errorf("--unsigned cannot be combined with --signing-key or --keyless")
	}
	if unsigned {
		options.Mode = bundle.SigningModeUnsigned
		return nil
	}
	if options.Key == "" && !keyless {
		return fmt.Errorf("one of --signing-key, --keyless, or --unsigned is required")
	}
	return completeSigningOptions(cmd, options)
}
