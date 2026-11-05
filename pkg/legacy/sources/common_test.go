// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package sources

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

func TestLoadPackageFromDirPreservesManifestDigest(t *testing.T) {
	ctx := context.Background()
	packagePath, err := filepath.Abs(filepath.Join("..", "testdata", "zarf-package-real-simple-amd64-0.0.1.tar.zst"))
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

func TestHandleFilterPreservesBetaFields(t *testing.T) {
	component := api.Component{
		Name:        "required",
		StateAccess: []api.StateAccessKey{api.StateAccessRegistryCredentials},
		Files:       []api.File{{Source: "file", EnableTemplating: true, Checksum: api.ParseFileChecksum("sha512:abc")}},
		Actions: api.ComponentActions{OnDeploy: api.ActionSet{Before: []api.Action{{
			Cmd: "echo '{{ .Values.message }}'", EnableTemplating: true,
			SetValues: []api.SetValue{{Key: "result", Type: api.SetValueString}},
		}}}},
	}
	pkg := api.Package{APIVersion: "zarf.dev/v1beta1", Components: []api.Component{
		component, {Name: "optional", Optional: true},
	}}
	components, partial, err := handleFilter(pkg, filters.ForDeploy("", false))
	require.NoError(t, err)
	require.True(t, partial)
	require.Equal(t, []api.Component{component}, components)
}
