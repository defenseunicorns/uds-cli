// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

const bundleDefinitionDeployDiagnostic = "WARNING: deploying directly from a bundle definition; bundle provenance and bundle-signature verification are unavailable"

// DevDeployOptions holds options for bundle definition deployment.
type DevDeployOptions struct {
	BundlePath string
	Packages   []string
	Force      bool
	Config     *bundlepkg.UDSBundleConfig
	Printer    printer.ResourcePrinter

	flags     CLIFlags
	runDeploy deployRunnerFunc

	iostreams.IOStreams
}

// NewDevDeployOptions returns development deploy options with default values.
func NewDevDeployOptions(streams iostreams.IOStreams) *DevDeployOptions {
	return &DevDeployOptions{IOStreams: streams, runDeploy: runDeploy}
}

// NewDevDeployCommand creates the bundle definition deploy command.
func NewDevDeployCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewDevDeployOptions(streams)

	cmd := &cobra.Command{
		Use:   "deploy [bundle-definition]",
		Short: "Deploy a bundle directly from its bundle definition",
		Long: `Deploy a UDS bundle directly from its bundle definition (bundle.uds.hcl).

The optional bundle-definition can be a directory containing bundle.uds.hcl or
a direct path to bundle.uds.hcl. If omitted, the current directory is used.

This development workflow does not create an intermediate bundle artifact, so
bundle provenance and bundle-signature verification are unavailable. Created
local and OCI bundle artifacts must use uds bundle deploy instead.`,
		Example: `  # Deploy from the bundle definition in the current directory
  uds bundle dev deploy

  # Deploy from a bundle definition in a specific directory
  uds bundle dev deploy ./my-bundle

  # Deploy selected packages with confirmation
  uds bundle dev deploy ./my-bundle --packages nginx,podinfo --prompt`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run(cmd.Context()))
		},
	}

	addDeployFlags(cmd, &o.Packages, &o.Force)

	return cmd
}

// Complete fills development deploy options from command-line arguments.
func (o *DevDeployOptions) Complete(cmd *cobra.Command, args []string) error {
	o.BundlePath = "."
	if len(args) > 0 {
		o.BundlePath = args[0]
	}
	o.flags = SnapshotFlags(cmd)

	p, err := ResolvePrinter(cmd)
	if err != nil {
		return err
	}
	o.Printer = p
	return nil
}

// Validate validates development deploy options without modifying state.
func (o *DevDeployOptions) Validate() error {
	return ValidateDevDeployPath(o.BundlePath)
}

// Run executes bundle definition deployment.
func (o *DevDeployOptions) Run(ctx context.Context) error {
	o.IOStreams = logger.Bind(o.IOStreams, o.flags.LogLevel)

	baseConfig, _, err := NewConfigResolver().resolveBase(ctx, o.IOStreams, o.flags)
	if err != nil {
		return err
	}
	o.Config = baseConfig
	o.IOStreams = logger.Bind(o.IOStreams, baseConfig.Options.LogLevel)

	if _, err := fmt.Fprintln(o.ErrOut(), bundleDefinitionDeployDiagnostic); err != nil {
		return fmt.Errorf("%w for bundle definition diagnostic: %w", ErrWriteDefinitionNotice, err)
	}

	runner := o.runDeploy
	if runner == nil {
		runner = runDeploy
	}
	result, err := runner(ctx, o.IOStreams, baseConfig, resolveBundlePath(o.BundlePath), o.Packages, o.Force, o.flags.Prompt)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return o.Printer.PrintObj(result, o.Out())
}
