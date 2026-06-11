// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
	"github.com/defenseunicorns/uds-cli/pkg/printer"
	"github.com/spf13/cobra"
)

// DeployOptions holds options for the deploy command.
type DeployOptions struct {
	BundlePath string // Path to bundle file, directory, or .tar.zst artifact (user input, resolved in Run)
	Config     *bundle.UDSBundleConfig
	Printer    printer.ResourcePrinter

	// flags is a snapshot of CLI flag values taken in Complete. Resolve() is
	// deferred to Run() because for tar.zst artifact deploys the bundle
	// directory (where defaults.uds.hcl lives) only exists after extraction.
	flags CLIFlags

	iostreams.IOStreams
}

// NewDeployOptions returns a DeployOptions with default values.
func NewDeployOptions(streams iostreams.IOStreams) *DeployOptions {
	return &DeployOptions{
		IOStreams: streams,
	}
}

// NewDeployCommand creates the deploy command.
func NewDeployCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewDeployOptions(streams)

	cmd := &cobra.Command{
		Use:   "deploy [bundle-path]",
		Short: "Deploy a bundle to a Kubernetes cluster",
		Long: `Deploy a UDS bundle to a Kubernetes cluster.

The bundle-path can be:
  - A directory containing bundle.uds.hcl
  - A path to a bundle.uds.hcl file
  - A path to a .tar.zst bundle artifact
  - If omitted, uses the current directory

The CLI is non-interactive by default (suitable for CI/CD pipelines) and
deploys packages within a level in parallel (see --concurrency, default 10).
Use --prompt to enable interactive confirmation before deployment.

Examples:
  # Deploy bundle in current directory (parallel, non-interactive)
  uds bundle deploy

  # Deploy bundle from specific directory
  uds bundle deploy ./my-bundle

  # Deploy bundle from artifact
  uds bundle deploy uds-bundle-example-amd64-0.1.0.tar.zst

  # Deploy with interactive confirmation prompt
  uds bundle deploy --prompt

  # Deploy serially without prompt
  uds bundle deploy --concurrency 1`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			ctx := cmd.Context()
			util.CheckErr(o.Run(ctx))
		},
	}

	return cmd
}

// Complete fills in options from command line args. It snapshots CLI flags for
// later use in Run(); config resolution is deferred to Run() because for
// tar.zst artifact deploys defaults.uds.hcl only exists after extraction.
func (o *DeployOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundlePath = args[0]
	} else {
		// Default to looking for bundle.uds.hcl in current directory
		o.BundlePath = "."
	}

	o.flags = SnapshotFlags(cmd)

	p, err := ResolvePrinter(cmd)
	if err != nil {
		return err
	}
	o.Printer = p

	return nil
}

// Validate validates the options without modifying state.
func (o *DeployOptions) Validate() error {
	return ValidateBundlePath(o.BundlePath, AllowArtifactBundlePath())
}

// Run executes the deploy command.
func (o *DeployOptions) Run(ctx context.Context) error {
	// Bind from the flag so logs here honor --log-level; re-bound after config resolves.
	o.IOStreams = logger.Bind(o.IOStreams, o.flags.LogLevel)

	deploySrc, err := bundle.PrepareDeploySource(ctx, o.IOStreams, o.BundlePath, "")
	if err != nil {
		return err
	}
	defer func() {
		if err := deploySrc.Close(); err != nil {
			o.Warn("failed to close deploy source", "error", err)
		}
	}()

	// Resolve config against the bundle-source or extracted bundle archive workspace.
	// This must happen after PrepareDeploySource because for tar.zst artifacts
	// defaults.uds.hcl is only available in the extracted workspace.
	cfg, _, err := NewConfigResolver().Resolve(ctx, o.IOStreams, o.flags, deploySrc.BundlePath)
	if err != nil {
		return err
	}
	o.Config = cfg

	o.IOStreams = logger.Bind(o.IOStreams, o.Config.Global.LogLevel)
	o.Debug("deploying bundle", "path", deploySrc.BundlePath, "prompt", o.Config.Global.Prompt)

	parsedBundle, err := bundle.NewHCLParser(o.Config.Options.Architecture, o.IOStreams).ParseBundleFile(ctx, deploySrc.BundlePath)
	if err != nil {
		return fmt.Errorf("failed to parse bundle: %w", err)
	}
	if err := parsedBundle.Validate(); err != nil {
		return fmt.Errorf("invalid bundle: %w", err)
	}

	o.Info("bundle to deploy", "name", parsedBundle.Metadata.Name, "packages", len(parsedBundle.Packages))

	if o.Config.Global.Prompt {
		confirmed, err := PromptConfirmation(o.IOStreams, "Deploy this bundle?")
		if err != nil {
			return err
		}
		if !confirmed {
			o.Info("deployment cancelled")
			return nil
		}
	}

	result, err := bundle.Deploy(ctx, bundle.DeployOptions{
		Config:     o.Config,
		BundlePath: deploySrc.BundlePath,
		Bundle:     parsedBundle,
		Source:     deploySrc,
		Streams:    o.IOStreams,
	})
	if err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	return o.Printer.PrintObj(result, o.Out())
}
