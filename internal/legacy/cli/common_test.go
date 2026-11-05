// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/utils"
	"github.com/mholt/archives"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/require"
)

func TestLegacyCommandsRejectNextLocalBundles(t *testing.T) {
	t.Setenv("UDS_CONFIG", "")
	dir := t.TempDir()
	t.Chdir(dir)
	originalDev := config.Dev
	t.Cleanup(func() { config.Dev = originalDev })
	hcl := filepath.Join(dir, "bundle.uds.hcl")
	require.NoError(t, os.WriteFile(hcl, nil, 0o600))
	index := filepath.Join(t.TempDir(), "index.json")
	require.NoError(t, os.WriteFile(index, []byte(`{}`), 0o600))
	files, err := archives.FilesFromDisk(t.Context(), nil, map[string]string{index: "oci/index.json"})
	require.NoError(t, err)
	archivePath := filepath.Join(dir, "uds-bundle-next-amd64-1.0.0.tar.zst")
	renamed := filepath.Join(dir, "renamed.tar.zst")
	legacyTar := filepath.Join(dir, "uds-bundle-next-amd64-1.0.0.tar")
	for _, name := range []string{archivePath, renamed, legacyTar} {
		out, err := os.Create(name)
		require.NoError(t, err)
		require.NoError(t, config.BundleArchiveFormat.Archive(context.Background(), out, files))
		require.NoError(t, out.Close())
	}

	for _, source := range []string{dir, hcl, archivePath, renamed, legacyTar} {
		for _, command := range [][]string{{"create"}, {"deploy"}, {"inspect"}, {"remove"}, {"publish"}, {"pull"}, {"dev", "deploy"}} {
			t.Run(filepath.Base(source)+"/"+command[0], func(t *testing.T) {
				root := NewRootCommand()
				args := append(append([]string{}, command...), source)
				if command[0] == "publish" {
					args = append(args, "oci://example.com/bundle:1.0.0")
				}
				if command[0] == "remove" {
					args = append(args, "--confirm")
				}
				root.SetArgs(append(args, "--no-log-file", "--no-progress", "--no-color"))
				require.ErrorContains(t, root.Execute(), utils.ErrNextBundle.Error())
			})
		}
	}
	for _, command := range [][]string{{"create"}, {"dev", "deploy"}} {
		root := NewRootCommand()
		root.SetArgs(append(command, "--no-log-file", "--no-progress"))
		require.ErrorContains(t, root.Execute(), utils.ErrNextBundle.Error())
	}
}

func TestLegacyCommandsRejectNextOCIIndexes(t *testing.T) {
	t.Setenv("UDS_CONFIG", "")
	t.Chdir(t.TempDir())
	originalDev := config.Dev
	t.Cleanup(func() { config.Dev = originalDev })
	for _, index := range []string{
		`{"schemaVersion":2,"artifactType":"application/vnd.defenseunicorns.uds.bundle.v1","manifests":[]}`,
		`{"schemaVersion":2,"manifests":[{"artifactType":"application/vnd.defenseunicorns.uds.bundle.v1","platform":{"os":"multi","architecture":"arm64"}}]}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v2/" {
				return
			}
			w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
			w.Header().Set("Docker-Content-Digest", digest.FromString(index).String())
			w.Header().Set("Content-Length", strconv.Itoa(len(index)))
			if r.Method != http.MethodHead {
				_, _ = fmt.Fprint(w, index)
			}
		}))
		t.Cleanup(server.Close)
		for _, reference := range []string{":next", "@" + digest.FromString(index).String()} {
			source := "oci://" + strings.TrimPrefix(server.URL, "http://") + "/bundle" + reference
			for _, command := range [][]string{{"deploy"}, {"inspect"}, {"remove", "--confirm"}, {"pull"}, {"dev", "deploy"}} {
				root := NewRootCommand()
				args := append(append([]string{}, command...), source)
				root.SetArgs(append(args, "--insecure", "--no-log-file", "--no-progress"))
				require.ErrorContains(t, root.Execute(), utils.ErrNextBundle.Error())
			}
		}
	}
}

func TestSetBundleFilePrefersLegacyDefinition(t *testing.T) {
	for _, name := range []string{"uds-bundle.yaml", "uds-bundle.yml"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "bundle.uds.hcl"), nil, 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
			require.NoError(t, setBundleFile([]string{dir}))
			require.Equal(t, name, bundleCfg.CreateOpts.BundleFile)
		})
	}
}

func TestValidConfigOptions(t *testing.T) {
	options := []string{
		"confirm",
		"insecure",
		"uds_cache",
		"tmp_dir",
		"log_level",
		"architecture",
		"no_log_file",
		"no_progress",
		"oci_concurrency",
		"no_color",
	}

	for _, option := range options {
		t.Run("test-"+option, func(t *testing.T) {
			res := isValidConfigOption(option)
			require.True(t, res)
		})
	}

	t.Run("test-invalid-option", func(t *testing.T) {
		res := isValidConfigOption("invalid")
		require.False(t, res)
	})
}
