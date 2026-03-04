// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package config

import (
	"os"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestDefaultBundleConfig(t *testing.T) {
	cfg := DefaultBundleConfig()

	require.Equal(t, runtime.GOARCH, cfg.Architecture)
	require.False(t, cfg.PlainHTTP)
	require.False(t, cfg.SkipTLSVerify)
	require.Equal(t, 10, cfg.Concurrency)
	require.Equal(t, os.TempDir(), cfg.TmpDir)
}

func TestBuildBundleConfig_Defaults(t *testing.T) {
	cmd := &cobra.Command{}
	registerBundleFlags(cmd)

	cfg := BuildBundleConfig(cmd)
	defaults := DefaultBundleConfig()

	require.Equal(t, defaults, cfg)
}

func TestBuildBundleConfig_ExplicitFlags(t *testing.T) {
	cmd := &cobra.Command{}
	registerBundleFlags(cmd)

	// Simulate explicit flag setting
	require.NoError(t, cmd.Flags().Set("architecture", "arm64"))
	require.NoError(t, cmd.Flags().Set("plain-http", "true"))
	require.NoError(t, cmd.Flags().Set("skip-tls-verify", "true"))
	require.NoError(t, cmd.Flags().Set("tmp-dir", "/custom/tmp"))
	require.NoError(t, cmd.Flags().Set("concurrency", "20"))

	cfg := BuildBundleConfig(cmd)

	require.Equal(t, "arm64", cfg.Architecture)
	require.True(t, cfg.PlainHTTP)
	require.True(t, cfg.SkipTLSVerify)
	require.Equal(t, "/custom/tmp", cfg.TmpDir)
	require.Equal(t, 20, cfg.Concurrency)
}

func TestBuildBundleConfig_PartialFlags(t *testing.T) {
	cmd := &cobra.Command{}
	registerBundleFlags(cmd)

	// Only set architecture
	require.NoError(t, cmd.Flags().Set("architecture", "arm64"))

	cfg := BuildBundleConfig(cmd)

	require.Equal(t, "arm64", cfg.Architecture)
	// Everything else should be defaults
	require.False(t, cfg.PlainHTTP)
	require.Equal(t, 10, cfg.Concurrency)
}

// registerBundleFlags mirrors the flags registered in pkg/cmd/bundle/bundle.go
// for testing BuildBundleConfig.
func registerBundleFlags(cmd *cobra.Command) {
	defaults := DefaultBundleConfig()
	cmd.Flags().StringP("architecture", "a", defaults.Architecture, "target architecture")
	cmd.Flags().Bool("plain-http", defaults.PlainHTTP, "use plain HTTP")
	cmd.Flags().Bool("skip-tls-verify", defaults.SkipTLSVerify, "skip TLS verification")
	cmd.Flags().String("tmp-dir", defaults.TmpDir, "temp directory")
	cmd.Flags().Int("concurrency", defaults.Concurrency, "concurrency")
}
