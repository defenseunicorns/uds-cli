// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

type deployRunnerFunc func(
	ctx context.Context,
	streams iostreams.IOStreams,
	config *bundlepkg.UDSBundleConfig,
	bundlePath string,
	packages []string,
	force bool,
	prompt bool,
) (*bundlepkg.DeployResult, error)

type prepareDeploySourceFunc func(
	ctx context.Context,
	streams iostreams.IOStreams,
	path string,
	tmpDir string,
	architecture string,
) (*preparedDeploySource, error)

type deployBundleFunc func(ctx context.Context, source *bundlepkg.DeploySource, opts bundlepkg.DeployOptions) (*bundlepkg.DeployResult, error)

type deployRunnerDependencies struct {
	prepare prepareDeploySourceFunc
	deploy  deployBundleFunc
}

func runDeploy(
	ctx context.Context,
	streams iostreams.IOStreams,
	baseConfig *bundlepkg.UDSBundleConfig,
	bundlePath string,
	packages []string,
	force bool,
	prompt bool,
) (*bundlepkg.DeployResult, error) {
	return runDeployWith(ctx, streams, baseConfig, bundlePath, packages, force, prompt, deployRunnerDependencies{
		prepare: prepareDeploySource,
		deploy:  bundlepkg.Deploy,
	})
}

func runDeployWith(
	ctx context.Context,
	streams iostreams.IOStreams,
	baseConfig *bundlepkg.UDSBundleConfig,
	bundlePath string,
	packages []string,
	force bool,
	prompt bool,
	deps deployRunnerDependencies,
) (*bundlepkg.DeployResult, error) {
	prepared, err := deps.prepare(ctx, streams, bundlePath, baseConfig.Options.TmpDir, baseConfig.Options.Architecture)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := prepared.close(); err != nil {
			streams.Warn("failed to close deploy source", "error", err)
		}
	}()
	deploySrc := prepared.source

	config := baseConfig
	streams = logger.Bind(streams, config.Options.LogLevel)
	streams.Debug("prepared bundle deployment source", "path", deploySrc.BundlePath, "prompt", prompt)

	parsedBundle, err := parseDeployBundle(ctx, streams, config.Options.Architecture, deploySrc)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrParseBundle, deploySrc.BundlePath, err)
	}
	if err := parsedBundle.Validate(); err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrInvalidBundle, parsedBundle.Metadata.Name, err)
	}
	deploySrc.Bundle = parsedBundle

	streams.Info("bundle to deploy", "name", parsedBundle.Metadata.Name, "packages", len(parsedBundle.Packages))

	if err := bundleinternal.ValidatePackageNames(packages, parsedBundle.Packages); err != nil {
		return nil, err
	}
	if !force {
		violations, err := bundleinternal.DeployViolations(ctx, streams, parsedBundle, packages)
		if err != nil {
			return nil, err
		}
		if len(violations) > 0 {
			return nil, fmt.Errorf("%w\nre-run with --force to override: %w", formatDependencyError("cannot deploy package(s) with unselected dependencies", "requires", violations), ErrForceRequired)
		}
	}

	if prompt {
		confirmed, err := PromptConfirmation(streams, "Deploy this bundle?")
		if err != nil {
			return nil, err
		}
		if !confirmed {
			streams.Info("deployment cancelled")
			return nil, nil
		}
	}

	result, err := deps.deploy(ctx, deploySrc, bundlepkg.DeployOptions{
		Config:   config,
		Packages: packages,
		Force:    force,
		Streams:  streams,
	})
	if err != nil {
		return result, err
	}

	return result, nil
}

func parseDeployBundle(ctx context.Context, streams iostreams.IOStreams, arch string, source *bundlepkg.DeploySource) (*spec.UDSBundle, error) {
	if source.Bundle != nil {
		return source.Bundle, nil
	}
	parser := bundleinternal.NewHCLParser(arch, streams)
	if source.Loader == nil {
		return parser.ParseBundleFile(ctx, source.BundlePath)
	}
	src, err := os.ReadFile(source.BundlePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read bundle artifact definition: %w", err)
	}
	return parser.ParseBundleBytes(ctx, src)
}
