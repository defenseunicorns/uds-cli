// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package sources

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

func TestLoadPackageFromDirPreservesManifestDigest(t *testing.T) {
	ctx := context.Background()
	packagePath, err := filepath.Abs(filepath.Join("..", "..", "test", "packages", "no-cluster", "real-simple", "zarf-package-real-simple-amd64-0.0.1.tar.zst"))
	require.NoError(t, err)

	extractedLayout, err := layout.LoadFromTar(ctx, packagePath, layout.PackageLayoutOptions{
		Filter:               filters.Empty(),
		VerificationStrategy: layout.VerifyNever,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, extractedLayout.Cleanup()) })

	manifestDigest := digest.FromString("source registry manifest")
	loadedLayout, err := loadPackageFromDir(ctx, extractedLayout.DirPath(), layout.PackageLayoutOptions{
		Filter:               filters.Empty(),
		VerificationStrategy: layout.VerifyNever,
	}, manifestDigest)
	require.NoError(t, err)
	require.Equal(t, manifestDigest.String(), loadedLayout.Digest())
	require.False(t, loadedLayout.IsPushable())
}
