// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package modify

import (
	"context"
	"fmt"
	"strings"

	internalzarf "github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
	"github.com/zarf-dev/zarf/src/pkg/signing"
	zarftypes "github.com/zarf-dev/zarf/src/types"
)

// Reassemble recreates a Zarf package from source produced by Disassemble.
func Reassemble(ctx context.Context, opts ReassembleOptions) (string, error) {
	if err := opts.validate(); err != nil {
		return "", err
	}
	return internalzarf.WithTempDir(opts.TmpDir, func() (string, error) {
		return reassemble(ctx, opts)
	})
}

func reassemble(ctx context.Context, opts ReassembleOptions) (string, error) {
	metadata, err := readDisassemblyMetadata(opts.SourceDir)
	if err != nil {
		return "", err
	}
	remoteOptions := zarftypes.RemoteOptions{
		PlainHTTP:             opts.PlainHTTP,
		InsecureSkipTLSVerify: opts.SkipTLSVerify,
	}
	definition, err := load.PackageDefinition(ctx, opts.SourceDir, load.DefinitionOptions{
		Flavor:        metadata.Flavor,
		RemoteOptions: remoteOptions,
	})
	if err != nil {
		return "", fmt.Errorf("loading disassembled package definition: %w", err)
	}
	if !strings.HasSuffix(definition.Metadata.Version, disassembleVersionSuffix) {
		return "", fmt.Errorf("package version %q must end with %q", definition.Metadata.Version, disassembleVersionSuffix)
	}
	if definition.Metadata.Architecture != metadata.Architecture {
		return "", fmt.Errorf("package architecture %q does not match disassembly metadata architecture %q", definition.Metadata.Architecture, metadata.Architecture)
	}
	var signOpts *signing.SignBlobOptions
	if opts.SigningKeyPath != "" {
		defaults := signing.DefaultSignBlobOptions()
		defaults.Key = opts.SigningKeyPath
		defaults.Password = opts.SigningKeyPassword
		signOpts = &defaults
	}

	created, err := packager.Create(ctx, opts.SourceDir, opts.Output, packager.CreateOptions{
		Flavor:               metadata.Flavor,
		MaxPackageSizeMB:     opts.MaxPackageSizeMB,
		SignBlobOptions:      signOpts,
		WithBuildMachineInfo: opts.WithBuildMachineInfo,
		OCIConcurrency:       opts.Concurrency,
		CachePath:            opts.CachePath,
		RemoteOptions:        remoteOptions,
	})
	if err != nil {
		return "", fmt.Errorf("creating package from disassembled source: %w", err)
	}
	return created, nil
}
