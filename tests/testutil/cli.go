// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package testutil

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"golang.org/x/mod/modfile"
	"helm.sh/helm/v4/pkg/kube"

	"github.com/defenseunicorns/uds-cli/internal/cli"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// ponytail: commands share process state; remove this lock only when logging,
// Helm settings, and working directories are isolated.
var cliMu sync.Mutex

// ExecuteCLI executes a fresh Next Cobra command with the supplied streams.
func ExecuteCLI(ctx context.Context, streams iostreams.IOStreams, args ...string) error {
	return executeCLI(ctx, streams, "", args...)
}

func executeCLI(ctx context.Context, streams iostreams.IOStreams, directory string, args ...string) (err error) {
	cliMu.Lock()
	defer cliMu.Unlock()
	if directory != "" {
		previous, getErr := os.Getwd()
		if getErr != nil {
			return getErr
		}
		if err := os.Chdir(directory); err != nil {
			return err
		}
		defer func() { err = errors.Join(err, os.Chdir(previous)) }()
	}
	fieldManager := kube.ManagedFieldsManager
	kube.ManagedFieldsManager = cluster.FieldManagerName
	defer func() { kube.ManagedFieldsManager = fieldManager }()
	logger := slog.Default()
	defer slog.SetDefault(logger)
	root := cli.NewRootCommand(streams)
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}

// RequireZarfVersion checks the standalone tool used by Zarf package action callbacks.
func RequireZarfVersion(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(TestDataPath(""), "..", "..", "go.mod"))
	require.NoError(t, err)
	module, err := modfile.ParseLax("go.mod", data, nil)
	require.NoError(t, err)
	for _, dependency := range module.Require {
		if dependency.Mod.Path != "github.com/zarf-dev/zarf" {
			continue
		}
		output, err := exec.CommandContext(t.Context(), "zarf", "version").CombinedOutput()
		require.NoError(t, err, "install repository tools with mise: %s", output)
		require.Equal(t, dependency.Mod.Version, strings.TrimSpace(string(output)), "Zarf callbacks must match the linked library; use mise exec with the linked version")
		return
	}
	t.Fatal("Zarf dependency is missing from go.mod")
}
