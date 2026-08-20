// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"

	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// ReconfigureOptions holds options for the reconfigure command.
type ReconfigureOptions struct {
	Source       string
	DefaultsFile string
	Suffix       string
	OutputDir    string
	Prompt       bool
	Config       *bundle.UDSBundleConfig
	Signing      bundle.SigningOptions
	Verification VerifyOptions
	Printer      printer.ResourcePrinter

	iostreams.IOStreams
}

// NewReconfigureOptions returns a ReconfigureOptions with default values.
func NewReconfigureOptions(streams iostreams.IOStreams) *ReconfigureOptions {
	return &ReconfigureOptions{
		Suffix:    "-reconfigured",
		IOStreams: streams,
	}
}

// NewReconfigureCommand creates the reconfigure command.
func NewReconfigureCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewReconfigureOptions(streams)

	cmd := &cobra.Command{
		Use:   "reconfigure <source> --defaults <defaults-file>",
		Short: "Reconfigure a bundle with new default values",
		Long:  "Replace the defaults in a bundle artifact with new values, producing a new derivative artifact",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			ctx := cmd.Context()
			util.CheckErr(o.Run(ctx))
		},
	}

	cmd.Flags().String("defaults", "", "path to new defaults.uds.hcl file (required)")
	_ = cmd.MarkFlagRequired("defaults")
	cmd.Flags().String("suffix", "-reconfigured", "suffix for output artifact name")
	cmd.Flags().StringVarP(&o.OutputDir, "output-dir", "", "", "output directory for reconfigured local tarball (default: current directory)")
	addSigningFlags(cmd, &o.Signing)
	cmd.Flags().Bool("unsigned", false, "create an unsigned reconfigured bundle")
	addVerificationFlags(cmd, &o.Verification, true)

	return cmd
}

// Complete fills in options from command args and flags.
func (o *ReconfigureOptions) Complete(cmd *cobra.Command, args []string) error {
	o.Source = args[0]

	var err error
	o.DefaultsFile, err = cmd.Flags().GetString("defaults")
	if err != nil {
		return err
	}

	o.Suffix, err = cmd.Flags().GetString("suffix")
	if err != nil {
		return err
	}

	// Default OutputDir to CWD for local sources.
	if o.OutputDir == "" && !isOCIReference(o.Source) {
		o.OutputDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w: %w", ErrResolvePath, err)
		}
	}

	// Resolve config options from persistent flags (plain-http, skip-tls-verify, etc.)
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	flags := SnapshotFlags(cmd)
	o.Prompt = flags.Prompt
	cfg, _, err := NewConfigResolver().Resolve(ctx, o.IOStreams, flags, "")
	if err != nil {
		return err
	}
	o.Config = cfg
	o.Verification.Config = cfg
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

// Validate checks that options are valid.
func (o *ReconfigureOptions) Validate() error {
	if o.Source == "" {
		return fmt.Errorf("source is required: %w", ErrInvalidArgument)
	}
	if o.DefaultsFile == "" {
		return fmt.Errorf("--defaults is required: %w", ErrInvalidArgument)
	}
	if _, err := os.Stat(o.DefaultsFile); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("defaults file not found: %s: %w: %w", o.DefaultsFile, ErrPathNotFound, err)
		}
		return fmt.Errorf("cannot access defaults file %s: %w: %w", o.DefaultsFile, ErrInvalidPath, err)
	}
	if o.Suffix == "" {
		return fmt.Errorf("--suffix must not be empty: %w", ErrInvalidArgument)
	}

	if o.OutputDir != "" && isOCIReference(o.Source) {
		return fmt.Errorf("--output-dir is not supported for OCI sources: %w", ErrUnsupportedSource)
	}
	if err := o.Signing.Validate(); err != nil {
		return err
	}
	if !o.Verification.SkipSignatureVerification {
		if _, err := o.Verification.policy(); err != nil {
			return err
		}
	}

	return nil
}

// Run executes the reconfigure command.
func (o *ReconfigureOptions) Run(ctx context.Context) error {
	o.IOStreams = logger.Bind(o.IOStreams, o.Config.Options.LogLevel)
	if o.Prompt {
		confirmed, err := PromptConfirmation(o.IOStreams, "Reconfigure this bundle?")
		if err != nil {
			return err
		}
		if !confirmed {
			o.Info("reconfigure cancelled")
			return nil
		}
	}
	o.Info("reconfiguring bundle", "source", o.Source)

	policy := bundle.VerificationPolicy{}
	if !o.Verification.SkipSignatureVerification {
		var err error
		policy, err = o.Verification.policy()
		if err != nil {
			return err
		}
	}
	result, err := bundle.Reconfigure(ctx, o.Source, o.DefaultsFile, bundle.ReconfigureOptions{
		Suffix:                    o.Suffix,
		OutputDir:                 o.OutputDir,
		Config:                    o.Config,
		Signing:                   o.Signing,
		Verification:              policy,
		SkipSignatureVerification: o.Verification.SkipSignatureVerification,
		Streams:                   o.IOStreams,
	})
	if err != nil {
		return err
	}

	return o.Printer.PrintObj(result, o.Out())
}
