// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"

	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

type preparedDeploySource struct {
	source *bundlepkg.DeploySource
	close  func() error
}

func prepareDeploySource(ctx context.Context, streams iostreams.IOStreams, path, tmpDir, architecture string) (*preparedDeploySource, error) {
	source, err := bundlepkg.PrepareDeploySource(ctx, streams, path, tmpDir, architecture)
	if err != nil {
		return nil, err
	}
	return &preparedDeploySource{source: source, close: source.Close}, nil
}
