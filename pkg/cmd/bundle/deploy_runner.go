// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/defenseunicorns/uds-cli/pkg/logger"
)

type deployRunnerFunc func(
	ctx context.Context,
	streams iostreams.IOStreams,
	config *bundlepkg.UDSBundleConfig,
	bundlePath string,
	packages []string,
	force bool,
) (*bundlepkg.DeployResult, error)

type prepareDeploySourceFunc func(
	ctx context.Context,
	streams iostreams.IOStreams,
	path string,
	tmpDir string,
) (*bundlepkg.DeploySource, error)

type closeDeploySourceFunc func(source *bundlepkg.DeploySource) error

type deployBundleFunc func(ctx context.Context, opts bundlepkg.DeployOptions) (*bundlepkg.DeployResult, error)

type deployRunnerDependencies struct {
	prepare prepareDeploySourceFunc
	close   closeDeploySourceFunc
	deploy  deployBundleFunc
}

func runDeploy(
	ctx context.Context,
	streams iostreams.IOStreams,
	baseConfig *bundlepkg.UDSBundleConfig,
	bundlePath string,
	packages []string,
	force bool,
) (*bundlepkg.DeployResult, error) {
	return runDeployWith(ctx, streams, baseConfig, bundlePath, packages, force, deployRunnerDependencies{
		prepare: bundlepkg.PrepareDeploySource,
		close: func(source *bundlepkg.DeploySource) error {
			return source.Close()
		},
		deploy: bundlepkg.Deploy,
	})
}

func runDeployWith(
	ctx context.Context,
	streams iostreams.IOStreams,
	baseConfig *bundlepkg.UDSBundleConfig,
	bundlePath string,
	packages []string,
	force bool,
	deps deployRunnerDependencies,
) (*bundlepkg.DeployResult, error) {
	deploySrc, err := deps.prepare(ctx, streams, bundlePath, baseConfig.Options.TmpDir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := deps.close(deploySrc); err != nil {
			streams.Warn("failed to close deploy source", "error", err)
		}
	}()

	config, err := NewConfigResolver().applyBundleDefaults(ctx, streams, baseConfig, deploySrc.BundlePath)
	if err != nil {
		return nil, err
	}
	streams = logger.Bind(streams, config.Global.LogLevel)
	streams.Debug("deploying bundle", "path", deploySrc.BundlePath, "prompt", config.Global.Prompt)

	parsedBundle, err := bundlepkg.NewHCLParser(config.Options.Architecture, streams).ParseBundleFile(ctx, deploySrc.BundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bundle: %w", err)
	}
	if err := parsedBundle.Validate(); err != nil {
		return nil, fmt.Errorf("invalid bundle: %w", err)
	}

	streams.Info("bundle to deploy", "name", parsedBundle.Metadata.Name, "packages", len(parsedBundle.Packages))

	if err := bundlepkg.ValidatePackageNames(packages, parsedBundle.Packages); err != nil {
		return nil, err
	}
	if !force {
		if err := bundlepkg.ValidateDeploySafety(ctx, streams, parsedBundle, packages); err != nil {
			return nil, fmt.Errorf("%w\nre-run with --force to override", err)
		}
	}

	if config.Global.Prompt {
		confirmed, err := PromptConfirmation(streams, "Deploy this bundle?")
		if err != nil {
			return nil, err
		}
		if !confirmed {
			streams.Info("deployment cancelled")
			return nil, nil
		}
	}

	result, err := deps.deploy(ctx, bundlepkg.DeployOptions{
		Config:     config,
		BundlePath: deploySrc.BundlePath,
		Bundle:     parsedBundle,
		Source:     deploySrc,
		Packages:   packages,
		Force:      force,
		Streams:    streams,
	})
	if err != nil {
		return nil, fmt.Errorf("deployment failed: %w", err)
	}

	return result, nil
}
