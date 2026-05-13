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

// RemoveOptions holds options for the remove command.
type RemoveOptions struct {
	BundlePath string // Path to bundle file or directory (user input, resolved in Run)
	Packages   []string
	Force      bool
	Config     *bundle.UDSBundleConfig
	Printer    printer.ResourcePrinter

	// parsedBundle is populated by Validate() after a successful parse and
	// is consumed by Run(). Centralizing parsing in Validate() lets the
	// dependency-safety check (ValidateRemovalSafety) run there without
	// re-parsing in Run().
	parsedBundle *bundle.UDSBundle

	iostreams.IOStreams
}

// NewRemoveOptions returns a RemoveOptions with default values.
func NewRemoveOptions(streams iostreams.IOStreams) *RemoveOptions {
	return &RemoveOptions{
		IOStreams: streams,
	}
}

// NewRemoveCommand creates the remove command.
func NewRemoveCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewRemoveOptions(streams)

	cmd := &cobra.Command{
		Use:   "remove [bundle-path]",
		Short: "Remove a bundle from a Kubernetes cluster",
		Long: `Remove a UDS bundle from a Kubernetes cluster.

The bundle-path can be:
  - A directory containing bundle.uds.hcl
  - A path to a bundle.uds.hcl file
  - If omitted, uses the bundle.uds.hcl file in current directory

Packages are removed in reverse order (last deployed first) to respect
dependency ordering. Use --packages to remove only specific packages.

When --packages targets a package that other bundle packages depend on,
removal is blocked. Pass --force to override the check.

The CLI is non-interactive by default (suitable for CI/CD pipelines).
Use --prompt to enable interactive confirmation before removal.

Examples:
  # Remove all packages in current directory bundle
  uds bundle remove

  # Remove bundle from specific directory
  uds bundle remove ./my-bundle

  # Remove only specific packages
  uds bundle remove --packages nginx,podinfo

  # Force-remove a package even if other packages depend on it
  uds bundle remove --packages core --force

  # Remove with interactive confirmation prompt
  uds bundle remove --prompt`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	cmd.Flags().StringSliceVarP(&o.Packages, "packages", "p", nil, "specific packages to remove (comma-separated)")
	cmd.Flags().BoolVar(&o.Force, "force", false, "remove packages even if other bundle packages depend on them")

	return cmd
}

// Complete fills in options from command line args.
func (o *RemoveOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundlePath = args[0]
	} else {
		o.BundlePath = "."
	}

	cfg, _, err := NewConfigResolver().Resolve(cmd, o.BundlePath)
	if err != nil {
		return err
	}
	o.Config = cfg

	p, err := ResolvePrinter(cmd)
	if err != nil {
		return err
	}
	o.Printer = p

	return nil
}

// Validate validates the options. It parses the bundle so that bundle-content
// checks (parsedBundle.Validate, ValidatePackageNames, and the dependency
// safety check) can run before Run(). The parsed bundle is cached on
// o.parsedBundle for Run() to consume.
func (o *RemoveOptions) Validate() error {
	if err := bundle.ValidateConfig(o.Config); err != nil {
		return err
	}
	if err := ValidateBundlePath(o.BundlePath); err != nil {
		return err
	}

	bundlePath := ResolveBundlePath(o.BundlePath)
	parsedBundle, err := bundle.NewHCLParser(o.Config.Options.Architecture).ParseBundleFile(context.Background(), bundlePath)
	if err != nil {
		return fmt.Errorf("failed to parse bundle: %w", err)
	}
	if err := parsedBundle.Validate(); err != nil {
		return fmt.Errorf("invalid bundle: %w", err)
	}
	if err := bundle.ValidatePackageNames(o.Packages, parsedBundle.Packages); err != nil {
		return err
	}
	if !o.Force {
		if err := bundle.ValidateRemovalSafety(parsedBundle, o.Packages); err != nil {
			return err
		}
	}

	o.parsedBundle = parsedBundle
	return nil
}

// Run executes the remove command. Validate() must have populated
// o.parsedBundle.
func (o *RemoveOptions) Run() error {
	ctx := context.Background()

	bundlePath := ResolveBundlePath(o.BundlePath)
	slog.Debug("removing bundle", "path", bundlePath, "prompt", o.Config.Global.Prompt)

	slog.Info("bundle to remove", "name", o.parsedBundle.Metadata.Name, "packages", len(o.parsedBundle.Packages))

	if o.Config.Global.Prompt {
		confirmed, err := o.promptConfirmation()
		if err != nil {
			return err
		}
		if !confirmed {
			slog.Info("removal cancelled")
			return nil
		}
	}

	removeOpts := bundle.RemoveOptions{
		Config:     o.Config,
		BundlePath: bundlePath,
		Bundle:     o.parsedBundle,
		Packages:   o.Packages,
		Out:        o.ErrOut,
	}

	result, err := bundle.Remove(ctx, removeOpts)
	if err != nil {
		return fmt.Errorf("removal failed: %w", err)
	}

	return o.Printer.PrintObj(result, o.Out)
}

// promptConfirmation asks the user to confirm removal.
func (o *RemoveOptions) promptConfirmation() (bool, error) {
	_, _ = fmt.Fprint(o.ErrOut, "\nRemove this bundle? [y/N]: ")

	var response string
	_, err := fmt.Fscanln(o.In, &response)
	if err != nil {
		return false, nil
	}

	return strings.ToLower(response) == "y" || strings.ToLower(response) == "yes", nil
}
