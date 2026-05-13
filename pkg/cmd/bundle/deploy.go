// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/printer"
	"github.com/spf13/cobra"
)

// DeployOptions holds options for the deploy command.
type DeployOptions struct {
	BundlePath string // Path to bundle file or directory (user input, resolved in Run)
	Config     *bundle.UDSBundleConfig
	Printer    printer.ResourcePrinter

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
  - If omitted, uses the current directory

The CLI is non-interactive by default (suitable for CI/CD pipelines) and
deploys packages within a level in parallel (see --concurrency, default 10).
Use --prompt to enable interactive confirmation before deployment.

Interactive mode is incompatible with parallel deploys: when --prompt is set
without an explicit --concurrency, concurrency is forced to 1. Passing
--prompt together with --concurrency > 1 is rejected.

Examples:
  # Deploy bundle in current directory (parallel, non-interactive)
  uds bundle deploy

  # Deploy bundle from specific directory
  uds bundle deploy ./my-bundle

  # Deploy with interactive confirmation prompt (concurrency auto-forced to 1)
  uds bundle deploy --prompt

  # Deploy serially without prompt
  uds bundle deploy --concurrency 1`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	return cmd
}

// Complete fills in options from command line args.
func (o *DeployOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundlePath = args[0]
	} else {
		// Default to looking for bundle.uds.hcl in current directory
		o.BundlePath = "."
	}

	cfg, _, err := NewConfigResolver().Resolve(cmd, o.BundlePath)
	if err != nil {
		return err
	}
	o.Config = cfg

	// Interactive prompts cannot run in parallel. When a user passes --prompt
	// without explicitly setting --concurrency, force concurrency to 1 rather
	// than failing on the default (10, per ADR-0006). If the user passes both
	// flags with incompatible values, Validate() rejects it.
	if cmd.Flags().Changed("prompt") && !cmd.Flags().Changed("concurrency") {
		o.Config.Options.Concurrency = 1
	}

	p, err := ResolvePrinter(cmd)
	if err != nil {
		return err
	}
	o.Printer = p

	return nil
}

// Validate validates the options without modifying state.
// Config validation is performed during Resolve(); this only checks
// command-specific inputs and cross-field rules involving CLI-only fields.
func (o *DeployOptions) Validate() error {
	if err := ValidateBundlePath(o.BundlePath); err != nil {
		return err
	}
	// Interactive prompts cannot run in parallel. --prompt is CLI-only (ADR-0005),
	// so this cross-field rule lives here, not in the bundle layer.
	if o.Config.Global.Prompt && o.Config.Options.Concurrency > 1 {
		return fmt.Errorf("--prompt is incompatible with concurrency > 1; interactive prompts cannot run in parallel")
	}
	return nil
}

// Run executes the deploy command.
func (o *DeployOptions) Run() error {
	ctx := context.Background()

	// Resolve the bundle path
	bundlePath := ResolveBundlePath(o.BundlePath)
	slog.Debug("deploying bundle", "path", bundlePath, "prompt", o.Config.Global.Prompt)

	// Parse bundle for display (BundlePath already validated by Validate)
	parsedBundle, err := bundle.NewHCLParser(o.Config.Options.Architecture).ParseBundleFile(ctx, bundlePath)
	if err != nil {
		return fmt.Errorf("failed to parse bundle: %w", err)
	}

	// Validate the bundle
	if err := parsedBundle.Validate(); err != nil {
		return fmt.Errorf("invalid bundle: %w", err)
	}

	// Display bundle information to stderr (diagnostic, not structured output)
	slog.Info("bundle to deploy", "name", parsedBundle.Metadata.Name, "packages", len(parsedBundle.Packages))

	if o.Config.Global.Prompt {
		confirmed, err := o.promptConfirmation()
		if err != nil {
			return err
		}
		if !confirmed {
			slog.Info("deployment cancelled")
			return nil
		}
	}

	deployOpts := bundle.DeployOptions{
		Config:     o.Config,
		BundlePath: bundlePath,
		Bundle:     parsedBundle,
		Prompt:     o.Config.Global.Prompt,
		Out:        o.ErrOut,
	}

	result, err := bundle.Deploy(ctx, deployOpts)
	if err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	return o.Printer.PrintObj(result, o.Out)
}

// promptConfirmation asks the user to confirm deployment.
func (o *DeployOptions) promptConfirmation() (bool, error) {
	_, _ = fmt.Fprint(o.ErrOut, "\nDeploy this bundle? [y/N]: ")

	var response string
	_, err := fmt.Fscanln(o.In, &response)
	if err != nil {
		// Treat empty/EOF as "no"
		return false, nil
	}

	return strings.ToLower(response) == "y" || strings.ToLower(response) == "yes", nil
}
