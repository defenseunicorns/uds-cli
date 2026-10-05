// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/load"
	zarftypes "github.com/zarf-dev/zarf/src/types"
)

// Reassemble recreates a Zarf package from source produced by Disassemble.
func Reassemble(ctx context.Context, opts ReassembleOptions) (*ReassembleResult, error) {
	if strings.TrimSpace(opts.SourceDir) == "" {
		return nil, errors.New("source directory is required")
	}

	metadata, err := readDisassemblyMetadata(opts.SourceDir)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("loading disassembled package definition: %w", err)
	}
	pkg := definition.AsV1alpha1()
	if !strings.HasSuffix(pkg.Metadata.Version, disassembleVersionSuffix) {
		return nil, fmt.Errorf("package version %q must end with %q", pkg.Metadata.Version, disassembleVersionSuffix)
	}
	if pkg.Metadata.Architecture != metadata.Architecture {
		return nil, fmt.Errorf("package architecture %q does not match disassembly metadata architecture %q", pkg.Metadata.Architecture, metadata.Architecture)
	}

	created, err := packager.Create(ctx, opts.SourceDir, opts.Output, packager.CreateOptions{
		Flavor:               metadata.Flavor,
		MaxPackageSizeMB:     opts.MaxPackageSizeMB,
		SigningKeyPath:       opts.SigningKeyPath,
		SigningKeyPassword:   opts.SigningKeyPassword,
		WithBuildMachineInfo: opts.WithBuildMachineInfo,
		OCIConcurrency:       opts.Concurrency,
		CachePath:            opts.CachePath,
		RemoteOptions:        remoteOptions,
	})
	if err != nil {
		return nil, fmt.Errorf("creating package from disassembled source: %w", err)
	}
	return &ReassembleResult{SourceDir: opts.SourceDir, OutputPath: created}, nil
}
