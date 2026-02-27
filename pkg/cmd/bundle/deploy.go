// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// DeployOptions holds options for the deploy command.
type DeployOptions struct {
	BundlePath string // Path to bundle file or directory (user input, resolved in Run)
	Confirm    bool   // Auto-confirm deployment (skip confirmation prompt)

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

Examples:
  # Deploy bundle in current directory
  uds bundle deploy

  # Deploy bundle from specific directory
  uds bundle deploy ./my-bundle

  # Deploy with auto-confirmation (for CI/CD)
  uds bundle deploy --confirm`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	cmd.Flags().BoolVarP(&o.Confirm, "confirm", "y", false, "Auto-confirm deployment (skip confirmation prompt)")

	return cmd
}

// Complete fills in options from command line args.
func (o *DeployOptions) Complete(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundlePath = args[0]
	} else {
		// Default to looking for bundle.uds.hcl in current directory
		o.BundlePath = "."
	}
	return nil
}

// Validate validates the options without modifying state.
func (o *DeployOptions) Validate() error {
	return ValidateBundlePath(o.BundlePath)
}

// Run executes the deploy command.
func (o *DeployOptions) Run() error {
	ctx := context.Background()

	// Resolve the bundle path
	bundlePath := ResolveBundlePath(o.BundlePath)

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
	out, err := parsedBundle.BufferString()
	if err != nil {
		return err
	}
	_, err = o.Out.Write(out.Bytes())
	if err != nil {
		return err
	}

	if !o.Confirm {
		confirmed, err := o.promptConfirmation()
		if err != nil {
			return err
		}
		if !confirmed {
			_, _ = fmt.Fprintln(o.Out, "Deployment cancelled")
			return nil
		}
		o.Confirm = true
	}

	_, _ = fmt.Fprintf(o.Out, "\nStarting deployment of bundle: %s\n", parsedBundle.Metadata.Name)

	// Deploy the bundle
	deployOpts := bundle.DeployOptions{
		BundlePath: bundlePath,
		Confirm:    o.Confirm,
		Out:        o.Out,
	}

	if err := bundle.Deploy(ctx, deployOpts); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

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
