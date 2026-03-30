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
	"github.com/spf13/cobra"
)

// DeployOptions holds options for the deploy command.
type DeployOptions struct {
	BundlePath string // Path to bundle file or directory (user input, resolved in Run)
	Config     *bundle.UDSBundleConfig

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

The CLI is non-interactive by default (suitable for CI/CD pipelines).
Use --prompt to enable interactive confirmation before deployment.

Examples:
  # Deploy bundle in current directory (non-interactive)
  uds bundle deploy

  # Deploy bundle from specific directory
  uds bundle deploy ./my-bundle

  # Deploy with interactive confirmation prompt
  uds bundle deploy --prompt`,
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
	return nil
}

// Validate validates the options without modifying state.
func (o *DeployOptions) Validate() error {
	if err := bundle.ValidateConfig(o.Config); err != nil {
		return err
	}
	if err := ValidateConfigOptions(*o.Config.Options); err != nil {
		return err
	}
	if err := ValidateBundlePath(o.BundlePath); err != nil {
		return err
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
	parsedBundle, err := bundle.NewHCLParser().ParseBundleFile(ctx, bundlePath)
	if err != nil {
		return fmt.Errorf("failed to parse bundle: %w", err)
	}

	// Validate the bundle
	if err := parsedBundle.Validate(); err != nil {
		return fmt.Errorf("invalid bundle: %w", err)
	}

	// Display bundle information before deployment
	_, err = o.Out.Write(parsedBundle.BufferString().Bytes())
	if err != nil {
		return err
	}

	if o.Config.Global.Prompt {
		confirmed, err := o.promptConfirmation()
		if err != nil {
			return err
		}
		if !confirmed {
			_, _ = fmt.Fprintln(o.Out, "Deployment cancelled")
			return nil
		}
	}

	slog.Info("starting deployment", "name", parsedBundle.Metadata.Name)

	deployOpts := bundle.DeployOptions{
		Config:     o.Config,
		BundlePath: bundlePath,
		Bundle:     parsedBundle,
		Prompt:     o.Config.Global.Prompt,
		Out:        o.Out,
	}

	if err := bundle.Deploy(ctx, deployOpts); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	slog.Info("bundle deployed successfully", "name", parsedBundle.Metadata.Name)
	_, _ = fmt.Fprintln(o.Out, "\n✓ Bundle deployed successfully")
	return nil
}

// promptConfirmation asks the user to confirm deployment.
func (o *DeployOptions) promptConfirmation() (bool, error) {
	_, _ = fmt.Fprint(o.Out, "\nDeploy this bundle? [y/N]: ")

	var response string
	_, err := fmt.Fscanln(o.In, &response)
	if err != nil {
		// Treat empty/EOF as "no"
		return false, nil
	}

	return strings.ToLower(response) == "y" || strings.ToLower(response) == "yes", nil
}
