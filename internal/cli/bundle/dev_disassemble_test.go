// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"testing"

	"github.com/defenseunicorns/uds-cli/internal/mode/disassemble"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	bundlepkg "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zarfconfig "github.com/zarf-dev/zarf/src/config"
)

func TestDevDisassembleOptionsRunPassesConfigAndPrintsResult(t *testing.T) {
	previousTmpDir := zarfconfig.CommonOptions.TempDirectory
	zarfconfig.CommonOptions.TempDirectory = "/tmp/original"
	t.Cleanup(func() { zarfconfig.CommonOptions.TempDirectory = previousTmpDir })

	streams, _, out, _ := iostreams.NewTestIOStreams()
	textPrinter, err := printer.NewPrinter(printer.FormatText)
	require.NoError(t, err)
	var got disassemble.Options
	var gotZarfTmpDir string
	o := &DevDisassembleOptions{
		Source: "oci://example.test/package:1.0.0", OutputDir: "source",
		Config: &bundlepkg.UDSBundleConfig{Options: &bundlepkg.ConfigOptions{
			Architecture: "arm64", PlainHTTP: true, SkipTLSVerify: true, TmpDir: "/tmp/work", Concurrency: 3,
		}},
		Printer: textPrinter, IOStreams: streams,
		run: func(_ context.Context, opts disassemble.Options) (*disassemble.Result, error) {
			got = opts
			gotZarfTmpDir = zarfconfig.CommonOptions.TempDirectory
			return &disassemble.Result{Source: opts.Source, OutputDir: opts.OutputDir}, nil
		},
	}
	require.NoError(t, o.Run(t.Context()))
	assert.Equal(t, o.Config.Options.Architecture, got.Architecture)
	assert.Equal(t, o.Config.Options.PlainHTTP, got.PlainHTTP)
	assert.Equal(t, o.Config.Options.SkipTLSVerify, got.SkipTLSVerify)
	assert.Equal(t, o.Config.Options.TmpDir, got.TmpDir)
	assert.Equal(t, o.Config.Options.TmpDir, gotZarfTmpDir)
	assert.Equal(t, o.Config.Options.TmpDir, zarfconfig.CommonOptions.TempDirectory)
	assert.Equal(t, o.Config.Options.Concurrency, got.Concurrency)
	assert.Contains(t, out.String(), "oci://example.test/package:1.0.0")
}

func TestNewDevCommandContainsDisassemble(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	cmd := NewDevCommand(streams)
	found, _, err := cmd.Find([]string{"disassemble"})
	require.NoError(t, err)
	assert.Equal(t, "disassemble", found.Name())
	assert.Equal(t, "disassemble <source> <output-dir>", found.Use)
}
