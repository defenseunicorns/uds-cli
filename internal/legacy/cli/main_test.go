// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
)

func TestNewRootCommandIsRepeatable(t *testing.T) {
	first := NewRootCommand()
	second := NewRootCommand()
	if first == second {
		t.Fatal("NewRootCommand reused a command tree")
	}
}

func TestNewRootCommandDoesNotReadOrReuseViper(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "uds-config.yaml")
	if err := os.WriteFile(configPath, []byte("invalid: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UDS_CONFIG", configPath)
	NewRootCommand()
	first := v
	if vConfigError != nil {
		t.Fatalf("configuration read during construction: %v", vConfigError)
	}
	if got := v.GetString(V_LOG_LEVEL); got != "info" {
		t.Fatalf("log level = %q before execution, want default", got)
	}

	t.Setenv("UDS_CONFIG", "")
	t.Chdir(t.TempDir())
	NewRootCommand()
	if v == first {
		t.Fatal("NewRootCommand reused Viper state")
	}
	if got := v.ConfigFileUsed(); got != "" {
		t.Fatalf("ConfigFileUsed() = %q, want empty", got)
	}
}

func TestConfigurationIsAppliedDuringExecution(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "uds-config.yaml")
	contents := []byte(`options:
  architecture: arm64
  insecure: true
  log_level: debug
  oci_concurrency: 7
`)
	if err := os.WriteFile(configPath, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UDS_CONFIG", configPath)

	root := NewRootCommand()
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if config.CLIArch != "arm64" {
		t.Fatalf("architecture = %q, want arm64", config.CLIArch)
	}
	if !config.CommonOptions.Insecure {
		t.Fatal("insecure = false, want true")
	}
	if config.CommonOptions.OCIConcurrency != 7 {
		t.Fatalf("OCI concurrency = %d, want 7", config.CommonOptions.OCIConcurrency)
	}
	if logLevel != "debug" {
		t.Fatalf("log level = %q, want debug", logLevel)
	}
	if !config.SkipLogFile {
		t.Fatal("version enabled log file creation")
	}
}

func TestRootCommandDoesNotCreateLogFile(t *testing.T) {
	t.Setenv("UDS_CONFIG", "")
	t.Chdir(t.TempDir())

	root := NewRootCommand()
	root.SetArgs(nil)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !config.SkipLogFile {
		t.Fatal("root command enabled log file creation")
	}
}

func TestCommandLineOverridesConfiguration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "uds-config.yaml")
	if err := os.WriteFile(configPath, []byte("options:\n  architecture: arm64\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UDS_CONFIG", configPath)

	root := NewRootCommand()
	root.SetArgs([]string{"version", "--architecture", "amd64"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if config.CLIArch != "amd64" {
		t.Fatalf("architecture = %q, want amd64", config.CLIArch)
	}
}

func TestInvalidConfigurationFailsDuringExecution(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "uds-config.yaml")
	if err := os.WriteFile(configPath, []byte("invalid: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UDS_CONFIG", configPath)

	root := NewRootCommand()
	root.SetArgs([]string{"version"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "failed to load uds-config") {
		t.Fatalf("error = %v, want invalid configuration error", err)
	}
}

func TestConfigurationPreservesChangedDeployFlags(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "uds-config.yaml")
	if err := os.WriteFile(configPath, []byte("retries: 9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UDS_CONFIG", configPath)

	root := NewRootCommand()
	deploy, _, err := root.Find([]string{"deploy"})
	if err != nil {
		t.Fatal(err)
	}
	if err := deploy.Flags().Set("retries", "5"); err != nil {
		t.Fatal(err)
	}
	if err := readViperConfig(); err != nil {
		t.Fatal(err)
	}
	overrides := captureChangedFlags(deploy)
	if err := loadViperConfig(); err != nil {
		t.Fatal(err)
	}
	if err := restoreChangedFlags(overrides); err != nil {
		t.Fatal(err)
	}
	if bundleCfg.DeployOpts.Retries != 5 {
		t.Fatalf("retries = %d, want 5", bundleCfg.DeployOpts.Retries)
	}
}
