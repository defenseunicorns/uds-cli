// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"io"
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
	BundlePath   string
	Packages     []string
	Force        bool
	Config       *bundle.UDSBundleConfig
	Printer      printer.ResourcePrinter
	Verification VerifyOptions

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
	addVerificationFlags(cmd, &o.Verification, true)

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
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	config, _, err := NewConfigResolver().resolveBase(ctx, o.IOStreams, o.flags)
	if err != nil {
		return err
	}
	o.Config = config
	o.Verification.Config = config

	p, err := ResolvePrinter(cmd)
	if err != nil {
		return err
	}
	o.Printer = p
	return nil
}

// Validate validates artifact deploy options without modifying state.
func (o *DeployOptions) Validate() error {
	if err := ValidateArtifactReference(o.BundlePath); err != nil {
		return err
	}
	if !o.Verification.SkipSignatureVerification {
		if _, err := o.Verification.policy(); err != nil {
			return err
		}
	}
	return nil
}

// Run executes local or OCI artifact deployment.
func (o *DeployOptions) Run(ctx context.Context) error {
	if o.Config == nil {
		config, _, err := NewConfigResolver().resolveBase(ctx, o.IOStreams, o.flags)
		if err != nil {
			return err
		}
		o.Config = config
	}
	if o.Verification.Config == nil {
		o.Verification.Config = o.Config
	}
	o.IOStreams = logger.Bind(o.IOStreams, o.Config.Global.LogLevel)

	policy := bundle.VerificationPolicy{}
	var err error
	if !o.Verification.SkipSignatureVerification {
		policy, err = o.Verification.policy()
		if err != nil {
			return err
		}
	}
	artifactPath, cleanup, err := o.stageArtifact(ctx, policy)
	if err != nil {
		return err
	}
	defer cleanup()
	runner := o.runDeploy
	if runner == nil {
		runner = runDeploy
	}
	result, err := runner(ctx, o.IOStreams, o.Config, artifactPath, o.Packages, o.Force, policy, o.Verification.SkipSignatureVerification)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return o.Printer.PrintObj(result, o.Out())
}

// stageArtifact places the artifact in a private workspace before verification
// and deployment so both operations consume the same immutable copy.
func (o *DeployOptions) stageArtifact(ctx context.Context, policy bundle.VerificationPolicy) (string, func(), error) {
	if bundle.IsOCIReference(o.BundlePath) {
		return o.pullOCIArtifact(ctx, policy)
	}
	if o.Verification.SkipSignatureVerification {
		return o.BundlePath, func() {}, nil
	}
	return o.stageLocalArtifact()
}

func (o *DeployOptions) stageLocalArtifact() (string, func(), error) {
	workspace, cleanup, err := o.newArtifactWorkspace("uds-bundle-local-deploy-*")
	if err != nil {
		return "", nil, err
	}

	source, err := os.Open(o.BundlePath)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("opening local bundle artifact: %w", err)
	}
	defer func() { _ = source.Close() }()

	artifactPath := filepath.Join(workspace, "bundle.tar.zst")
	destination, err := os.OpenFile(artifactPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("creating staged bundle artifact: %w", err)
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("staging local bundle artifact: %w", copyErr)
	}
	if closeErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("closing staged bundle artifact: %w", closeErr)
	}
	return artifactPath, cleanup, nil
}

func (o *DeployOptions) pullOCIArtifact(ctx context.Context, policy bundle.VerificationPolicy) (string, func(), error) {
	outputDir, cleanup, err := o.newArtifactWorkspace("uds-bundle-oci-deploy-*")
	if err != nil {
		return "", nil, err
	}

	puller := o.puller
	if puller == nil {
		puller = bundle.NewDefaultPuller()
	}
	result, err := puller.PullBundle(ctx, o.BundlePath, outputDir, bundle.PullOptions{
		Config:                    o.Config,
		Verification:              policy,
		SkipSignatureVerification: o.Verification.SkipSignatureVerification,
		Streams:                   o.IOStreams,
	})
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("pulling bundle for deploy: %w", err)
	}
	if result == nil {
		cleanup()
		return "", nil, fmt.Errorf("pulling bundle for deploy: puller returned no result")
	}
	if result.OutputPath == "" {
		cleanup()
		return "", nil, fmt.Errorf("pulling bundle for deploy: puller returned an empty output path")
	}
	artifactPath, err := validatePulledArtifact(outputDir, result.OutputPath)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("pulling bundle for deploy: %w", err)
	}

	return artifactPath, cleanup, nil
}

func (o *DeployOptions) newArtifactWorkspace(pattern string) (string, func(), error) {
	workspace, err := os.MkdirTemp(o.Config.Options.TmpDir, pattern)
	if err != nil {
		return "", nil, fmt.Errorf("creating workspace for bundle deploy: %w", err)
	}
	cleanup := func() {
		if cleanupErr := os.RemoveAll(workspace); cleanupErr != nil {
			o.Warn("failed to remove deploy workspace", "path", workspace, "error", cleanupErr)
		}
	}
	return workspace, cleanup, nil
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
