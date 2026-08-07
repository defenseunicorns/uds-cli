// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// DeployOptions holds options for artifact deployment.
type DeployOptions struct {
	BundlePath string
	Packages   []string
	Force      bool
	Config     *bundle.UDSBundleConfig
	Printer    printer.ResourcePrinter

	flags     CLIFlags
	puller    bundle.Puller
	runDeploy deployRunnerFunc

	iostreams.IOStreams
}

// NewDeployOptions returns artifact deploy options with default values.
func NewDeployOptions(streams iostreams.IOStreams) *DeployOptions {
	return &DeployOptions{
		IOStreams: streams,
		puller:    bundle.NewDefaultPuller(),
		runDeploy: runDeploy,
	}
}

// NewDeployCommand creates the artifact deploy command.
func NewDeployCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewDeployOptions(streams)

	cmd := &cobra.Command{
		Use:   "deploy <bundle-artifact>",
		Short: "Deploy a created bundle artifact to Kubernetes",
		Long: `Deploy a created UDS bundle artifact to a Kubernetes cluster.

The required bundle-artifact can be a local .tar.zst file or an OCI reference.
Local and pulled OCI artifacts are integrity-verified before package deployment.

Bundle directories and bundle.uds.hcl files are development
inputs and must use uds bundle dev deploy instead.`,
		Example: `  # Deploy a local bundle artifact
  uds bundle deploy uds-bundle-example-amd64-0.1.0.tar.zst

  # Pull and deploy an OCI bundle artifact
  uds bundle deploy oci://ghcr.io/example/bundle:1.0.0

  # Deploy selected packages with confirmation
  uds bundle deploy bundle.tar.zst --packages nginx,podinfo --prompt`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run(cmd.Context()))
		},
	}

	addDeployFlags(cmd, &o.Packages, &o.Force)

	return cmd
}

func addDeployFlags(cmd *cobra.Command, packages *[]string, force *bool) {
	cmd.Flags().StringSliceVarP(packages, "packages", "p", nil, "specific packages to deploy (comma-separated)")
	cmd.Flags().BoolVarP(force, "force", "f", false, "deploy packages even if their dependencies are not selected")
}

// Complete fills artifact deploy options from command-line arguments.
func (o *DeployOptions) Complete(cmd *cobra.Command, args []string) error {
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

// Validate validates artifact deploy options without modifying state.
func (o *DeployOptions) Validate() error {
	return ValidateArtifactReference(o.BundlePath)
}

// Run executes local or OCI artifact deployment.
func (o *DeployOptions) Run(ctx context.Context) error {
	o.IOStreams = logger.Bind(o.IOStreams, o.flags.LogLevel)

	baseConfig, _, err := NewConfigResolver().resolveBase(ctx, o.IOStreams, o.flags)
	if err != nil {
		return err
	}
	o.Config = baseConfig
	o.IOStreams = logger.Bind(o.IOStreams, baseConfig.Global.LogLevel)

	var result *bundle.DeployResult
	runner := o.runDeploy
	if runner == nil {
		runner = runDeploy
	}
	if bundle.IsOCIReference(o.BundlePath) {
		result, err = o.runOCIArtifact(ctx, runner)
	} else {
		result, err = runner(ctx, o.IOStreams, baseConfig, o.BundlePath, o.Packages, o.Force)
	}
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return o.Printer.PrintObj(result, o.Out())
}

func (o *DeployOptions) runOCIArtifact(ctx context.Context, runner deployRunnerFunc) (*bundle.DeployResult, error) {
	outputDir, err := os.MkdirTemp(o.Config.Options.TmpDir, "uds-bundle-oci-deploy-*")
	if err != nil {
		return nil, fmt.Errorf("creating workspace for OCI bundle deploy: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(outputDir); cleanupErr != nil {
			o.Warn("failed to remove OCI deploy workspace", "path", outputDir, "error", cleanupErr)
		}
	}()

	puller := o.puller
	if puller == nil {
		puller = bundle.NewDefaultPuller()
	}
	result, err := puller.PullBundle(ctx, o.BundlePath, outputDir, bundle.PullOptions{
		Config:  o.Config,
		Streams: o.IOStreams,
	})
	if err != nil {
		return nil, fmt.Errorf("pulling bundle for deploy: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("pulling bundle for deploy: puller returned no result")
	}
	if result.OutputPath == "" {
		return nil, fmt.Errorf("pulling bundle for deploy: puller returned an empty output path")
	}
	artifactPath, err := validatePulledArtifact(outputDir, result.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("pulling bundle for deploy: %w", err)
	}

	return runner(ctx, o.IOStreams, o.Config, artifactPath, o.Packages, o.Force)
}

func validatePulledArtifact(workspace, outputPath string) (string, error) {
	resolvedWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("resolving pull workspace: %w", err)
	}
	resolvedArtifact, err := filepath.EvalSymlinks(outputPath)
	if err != nil {
		return "", fmt.Errorf("resolving pulled artifact: %w", err)
	}
	rel, err := filepath.Rel(resolvedWorkspace, resolvedArtifact)
	if err != nil {
		return "", fmt.Errorf("checking pulled artifact location: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("puller returned an artifact outside its workspace: %s", outputPath)
	}
	info, err := os.Stat(resolvedArtifact)
	if err != nil {
		return "", fmt.Errorf("checking pulled artifact: %w", err)
	}
	if !info.Mode().IsRegular() || !bundle.IsTarZst(resolvedArtifact) {
		return "", fmt.Errorf("puller returned a non-artifact output path: %s", outputPath)
	}
	return resolvedArtifact, nil
}
