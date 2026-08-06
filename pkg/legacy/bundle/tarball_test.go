// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

func TestForceUploadRepoAlwaysReportsContentMissing(t *testing.T) {
	exists, err := (&forceUploadRepo{}).Exists(context.Background(), ocispec.Descriptor{})

	require.NoError(t, err)
	require.False(t, exists)
}
