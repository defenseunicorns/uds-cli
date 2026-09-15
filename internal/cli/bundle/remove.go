// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// RemoveOptions holds options for the remove command.
type RemoveOptions struct {
	BundlePath   string
	Packages     []string
	Force        bool
	Prompt       bool
	Config       *bundle.UDSBundleConfig
	Verification VerifyOptions
	Printer      printer.ResourcePrinter
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
  - A .tar.zst artifact containing bundle.uds.hcl
  - An OCI artifact containing bundle.uds.hcl

Packages are removed in reverse order (last deployed first) to respect
dependency ordering. Use --packages to remove only specific packages.

When --packages targets a package that other bundle packages depend on,
removal is blocked. Pass --force to override the check.

The CLI is non-interactive by default (suitable for CI/CD pipelines).
Use --prompt to enable interactive confirmation before removal.

Examples:
  # Remove packages using a bundle in an OCI repository
  uds bundle remove oci://registry.example.com/my-org/my-bundle:1.0.0

  # Remove only specific packages from a local artifact
  uds bundle remove ./my-bundle.tar.zst --packages nginx,podinfo

  # Force-remove a package even if other packages depend on it
  uds bundle remove ./my-bundle.tar.zst --packages core --force

  # Remove with an interactive confirmation prompt
  uds bundle remove ./my-bundle.tar.zst --prompt

  # Remove using an unsigned local alpha artifact
  uds bundle remove ./my-bundle.tar.zst --skip-signature-verification`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run(cmd.Context()))
		},
	}

	cmd.Flags().StringSliceVarP(&o.Packages, "packages", "p", nil, "specific packages to remove (comma-separated)")
	cmd.Flags().BoolVarP(&o.Force, "force", "f", false, "remove packages even if other bundle packages depend on them")
	addVerificationFlags(cmd, &o.Verification, true)

	return cmd
}

// Complete fills in options from command line args.
func (o *RemoveOptions) Complete(cmd *cobra.Command, args []string) error {
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
	o.Verification.Config = cfg

	p, err := ResolvePrinter(cmd)
	if err != nil {
		return err
	}
	o.Printer = p

	return nil
}

// Validate checks argument shape, local source existence, and verification
// policy without parsing bundle content or contacting a registry.
func (o *RemoveOptions) Validate() error {
	if err := ValidateBundlePath(o.BundlePath, AllowArtifactBundlePath(), AllowOCIReferenceBundlePath()); err != nil {
		return err
	}

	if !isOCIReference(o.BundlePath) && !isTarZst(o.BundlePath) {
		return fmt.Errorf("bundle path %q is not a valid oci reference or .tar.zst \nUse \"uds bundle dev remove\" if the bundle path is an hcl file or directory", o.BundlePath)
	}

	if !o.Verification.SkipSignatureVerification && (isOCIReference(o.BundlePath) || isTarZst(o.BundlePath)) {
		_, err := o.Verification.policy()
		return err
	}

	return nil
}

// Run performs the metadata-only bundle preflight, prompts the user, and then
// delegates authoritative verification and removal to the library.
func (o *RemoveOptions) Run(ctx context.Context) error {
	bundlePath := resolveBundlePath(o.BundlePath)
	s := logger.Bind(o.IOStreams, o.Config.Options.LogLevel)

	var parsedBundle *spec.UDSBundle
	var err error

	inspection, err := artifact.InspectBundleDefinition(ctx, artifact.InspectOptions{
		Source:  o.BundlePath,
		Config:  toInternalConfig(o.Config),
		Streams: s,
	})
	if err != nil {
		return fmt.Errorf("%w %q: %w", ErrParseBundle, o.BundlePath, err)
	}
	parsedBundle = inspection.Bundle

	return runRemove(ctx, s, o.Printer, o.Config, bundlePath, parsedBundle, o.Packages, o.Force, o.Prompt, o.Verification)
}

func runRemove(ctx context.Context, s iostreams.IOStreams, printer printer.ResourcePrinter, cfg *bundle.UDSBundleConfig, bundlePath string, parsedBundle *spec.UDSBundle, packages []string, force bool, prompt bool, verification VerifyOptions) error {
	if err := parsedBundle.Validate(); err != nil {
		return fmt.Errorf("%w %q: %w", ErrInvalidBundle, parsedBundle.Metadata.Name, err)
	}
	if err := bundleinternal.ValidatePackageNames(packages, parsedBundle.Packages); err != nil {
		return err
	}
	if !force {
		violations, err := bundleinternal.RemovalViolations(ctx, s, parsedBundle, packages)
		if err != nil {
			return err
		}
		if len(violations) > 0 {
			return fmt.Errorf("%w\nre-run with --force to override: %w", formatDependencyError("cannot remove package(s) with bundle dependents", "is required by", violations), ErrForceRequired)
		}
	}

	s.Info("bundle to remove", "name", parsedBundle.Metadata.Name, "packages", len(parsedBundle.Packages))

	if prompt {
		confirmed, err := PromptConfirmation(s, "Remove this bundle?")
		if err != nil {
			return err
		}
		if !confirmed {
			s.Info("removal cancelled")
			return nil
		}
	}
	s.Info("removing bundle", "source", bundlePath)
	s.Debug("removing bundle", "path", bundlePath, "prompt", prompt)

	policy := bundle.VerificationPolicy{}
	if !verification.SkipSignatureVerification && (isOCIReference(bundlePath) || isTarZst(bundlePath)) {
		var err error
		policy, err = verification.policy()
		if err != nil {
			return err
		}
	}
	removeOpts := bundle.RemoveOptions{
		Config:                    cfg,
		Packages:                  packages,
		Verification:              policy,
		SkipSignatureVerification: verification.SkipSignatureVerification,
		Force:                     force,
		Streams:                   s,
	}

	// Zarf package APIs read their process-global temp setting instead of UDS config.
	configureZarfTempDir(cfg.Options.TmpDir)
	result, err := bundle.Remove(ctx, &bundle.DeploySource{
		BundlePath: bundlePath,
		Bundle:     parsedBundle,
	}, removeOpts)
	if err != nil {
		return err
	}

	return printer.PrintObj(result, s.Out())
}
