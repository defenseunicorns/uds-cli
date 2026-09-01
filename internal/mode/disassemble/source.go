// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"context"
	"fmt"

	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarftypes "github.com/zarf-dev/zarf/src/types"
)

func loadPackageSource(ctx context.Context, opts Options) (*layout.PackageLayout, error) {
	pkgLayout, err := packager.LoadPackage(ctx, opts.Source, packager.LoadOptions{
		Architecture:         opts.Architecture,
		Filter:               filters.Empty(),
		OCIConcurrency:       opts.Concurrency,
		VerificationStrategy: opts.VerificationStrategy,
		RemoteOptions: zarftypes.RemoteOptions{
			PlainHTTP:             opts.PlainHTTP,
			InsecureSkipTLSVerify: opts.SkipTLSVerify,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("loading package %q: %w", opts.Source, err)
	}
	return pkgLayout, nil
}
