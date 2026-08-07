// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build ghcr_write

package ghcr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/src/test/testutil"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

const ghcrRepositoryBase = "packages/uds-cli/test"

func TestPublishAndRemoveGHCRBundle(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN is required for GHCR write tests")
	}
	requireGHCRCLI(t)

	repository := testutil.UniqueRepository(t, ghcrRepositoryBase)
	tag := testutil.UniqueTag(t, "bundle")
	bundleName := testutil.UniqueTag(t, "bundle")
	registry := "ghcr.io/defenseunicorns/" + repository
	ref := fmt.Sprintf("%s/%s:%s", registry, bundleName, tag)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := deleteGHCRReference(ctx, ref, token); err != nil {
			t.Logf("clean up GHCR reference %q: %v", ref, err)
		}
	})

	workspace := t.TempDir()
	fixture := testutil.CopyFixture(t, "bundles/06-ghcr")
	result := testutil.RunCLI(t, testutil.CommandOptions{Dir: workspace},
		"--tmpdir", filepath.Join(workspace, "tmp"),
		"create", fixture,
		"--output", "oci://"+registry,
		"--name", bundleName,
		"--version", tag,
		"--architecture", runtime.GOARCH,
		"--confirm", "--no-progress",
	)
	require.NoError(t, result.Err, result.Stderr)

	inspect := testutil.RunCLI(t, testutil.CommandOptions{Dir: workspace},
		"--tmpdir", filepath.Join(workspace, "tmp"),
		"inspect", "oci://"+ref, "--no-color", "--no-progress",
	)
	require.NoError(t, inspect.Err, inspect.Stderr)
	require.Contains(t, inspect.Stdout, bundleName)
}

func TestGHCRPathExpansion(t *testing.T) {
	requireGHCRCLI(t)

	workspace := t.TempDir()
	for _, reference := range []string{
		"ghcr-test:0.0.1",
		"delivery/ghcr-test:0.0.1",
		"ghcr-delivery-test:0.0.1",
	} {
		result := testutil.RunCLI(t, testutil.CommandOptions{Dir: workspace},
			"--tmpdir", filepath.Join(workspace, "tmp"),
			"inspect", reference, "--no-color", "--no-progress",
		)
		require.NoError(t, result.Err, result.Stderr)
	}
}

func requireGHCRCLI(t *testing.T) {
	t.Helper()
	if os.Getenv("UDS_CLI_PATH") == "" {
		t.Skip("UDS_CLI_PATH is required for GHCR tests")
	}
}

func deleteGHCRReference(ctx context.Context, reference, token string) error {
	repository, err := remote.NewRepository(reference)
	if err != nil {
		return fmt.Errorf("create remote repository: %w", err)
	}
	username := os.Getenv("GITHUB_ACTOR")
	if username == "" {
		username = "github-actions[bot]"
	}
	repository.Client = &auth.Client{Credential: auth.StaticCredential("ghcr.io", auth.Credential{
		Username: username,
		Password: token,
	})}

	tag := repository.Reference.Reference
	descriptor, err := repository.Resolve(ctx, tag)
	if errors.Is(err, errdef.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve %q: %w", reference, err)
	}
	if err := repository.Delete(ctx, descriptor); err != nil && !errors.Is(err, errdef.ErrNotFound) {
		return fmt.Errorf("delete %q: %w", reference, err)
	}
	return nil
}
