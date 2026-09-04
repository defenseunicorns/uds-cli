package bundle

import (
	"context"
	"fmt"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

const bundleDefinitionRemoveDiagnostic = "WARNING: removing directly from a bundle definition; bundle provenance and bundle-signature verification are unavailable"

// DevRemoveOptions holds options for the remove command.
type DevRemoveOptions struct {
	BundlePath string // Path to bundle file or directory (user input, resolved in Run)
	Packages   []string
	Force      bool
	Prompt     bool
	Config     *bundle.UDSBundleConfig
	Printer    printer.ResourcePrinter
	iostreams.IOStreams
}

func NewDevRemoveOptions(streams iostreams.IOStreams) *DevRemoveOptions {
	return &DevRemoveOptions{
		IOStreams: streams,
	}
}

func NewDevRemoveCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewDevRemoveOptions(streams)

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
  # Remove all packages located in current directory bundle
  uds bundle remove

  # Remove packages with a bundle in a specific directory
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
			util.CheckErr(o.Run(cmd.Context()))
		},
	}

	cmd.Flags().StringSliceVarP(&o.Packages, "packages", "p", nil, "specific packages to remove (comma-separated)")
	cmd.Flags().BoolVarP(&o.Force, "force", "f", false, "remove packages even if other bundle packages depend on them")

	return cmd
}

// Complete fills in options from command line args.
func (o *DevRemoveOptions) Complete(cmd *cobra.Command, args []string) error {
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

	p, err := ResolvePrinter(cmd)
	if err != nil {
		return err
	}
	o.Printer = p

	return nil
}

// Validate checks argument shape, local source existence, and verification
// policy without parsing bundle content or contacting a registry.
func (o *DevRemoveOptions) Validate() error {
	if err := ValidateBundlePath(o.BundlePath, AllowArtifactBundlePath(), AllowOCIReferenceBundlePath()); err != nil {
		return err
	}

	if isOCIReference(o.BundlePath) || isTarZst(o.BundlePath) {
		return fmt.Errorf("bundle path %q is not a valid hcl file or directory \nUse \"uds bundle remove\" if the bundle path is an oci reference or tar.zst", o.BundlePath)
	}

	return nil
}

// Run performs the metadata-only bundle preflight, prompts the user, and then
// delegates authoritative verification and removal to the library.
func (o *DevRemoveOptions) Run(ctx context.Context) error {
	bundlePath := resolveBundlePath(o.BundlePath)
	s := logger.Bind(o.IOStreams, o.Config.Options.LogLevel)

	if _, err := fmt.Fprintln(o.ErrOut(), bundleDefinitionRemoveDiagnostic); err != nil {
		return fmt.Errorf("%w for bundle definition diagnostic: %w", ErrWriteDefinitionNotice, err)
	}

	parsedBundle, err := bundleinternal.NewHCLParser(o.Config.Options.Architecture, s).ParseBundleFile(ctx, bundlePath)
	if err != nil {
		return fmt.Errorf("%w %q: %w", ErrParseBundle, bundlePath, err)
	}

	if err = parsedBundle.Validate(); err != nil {
		return fmt.Errorf("%w %q: %w", ErrInvalidBundle, parsedBundle.Metadata.Name, err)
	}
	if err = bundleinternal.ValidatePackageNames(o.Packages, parsedBundle.Packages); err != nil {
		return err
	}
	if !o.Force {
		violations, err := bundleinternal.RemovalViolations(ctx, s, parsedBundle, o.Packages)
		if err != nil {
			return err
		}
		if len(violations) > 0 {
			return fmt.Errorf("%w\nre-run with --force to override: %w", formatDependencyError("cannot remove package(s) with bundle dependents", "is required by", violations), ErrForceRequired)
		}
	}

	s.Info("bundle to remove", "name", parsedBundle.Metadata.Name, "packages", len(parsedBundle.Packages))

	if o.Prompt {
		confirmed, err := PromptConfirmation(o.IOStreams, "Remove this bundle?")
		if err != nil {
			return err
		}
		if !confirmed {
			s.Info("removal cancelled")
			return nil
		}
	}
	s.Info("removing bundle", "source", bundlePath)
	s.Debug("removing bundle", "path", bundlePath, "prompt", o.Prompt)

	policy := bundle.VerificationPolicy{}
	removeOpts := bundle.RemoveOptions{
		Config:                    o.Config,
		Packages:                  o.Packages,
		Verification:              policy,
		SkipSignatureVerification: true,
		Force:                     o.Force,
		Streams:                   o.IOStreams,
	}

	result, err := bundle.Remove(ctx, &bundle.DeploySource{
		BundlePath: bundlePath,
		Bundle:     parsedBundle,
	}, removeOpts)
	if err != nil {
		return err
	}

	return o.Printer.PrintObj(result, o.Out())
}
