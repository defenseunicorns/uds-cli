// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package modify

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptionsValidate(t *testing.T) {
	validPackageOptions := PackageOptions{TmpDir: t.TempDir(), Concurrency: 1}
	tests := []struct {
		name      string
		opts      Options
		wantError string
	}{
		{name: "valid", opts: Options{PackageOptions: validPackageOptions, Source: "package.tar.zst", OutputDir: "source"}},
		{name: "source", opts: Options{PackageOptions: validPackageOptions, OutputDir: "source"}, wantError: "source is required"},
		{name: "output", opts: Options{PackageOptions: validPackageOptions, Source: "package.tar.zst"}, wantError: "output directory is required"},
		{name: "temp directory", opts: Options{PackageOptions: PackageOptions{Concurrency: 1}, Source: "package.tar.zst", OutputDir: "source"}, wantError: "temporary directory is required"},
		{name: "concurrency", opts: Options{PackageOptions: PackageOptions{TmpDir: t.TempDir()}, Source: "package.tar.zst", OutputDir: "source"}, wantError: "concurrency must be greater than zero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.validate()
			if tt.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantError)
		})
	}
}

func TestReassembleOptionsValidate(t *testing.T) {
	validPackageOptions := PackageOptions{TmpDir: t.TempDir(), Concurrency: 1}
	tests := []struct {
		name      string
		opts      ReassembleOptions
		wantError string
	}{
		{name: "valid", opts: ReassembleOptions{PackageOptions: validPackageOptions, SourceDir: "source"}},
		{name: "source", opts: ReassembleOptions{PackageOptions: validPackageOptions}, wantError: "source directory is required"},
		{name: "temp directory", opts: ReassembleOptions{PackageOptions: PackageOptions{Concurrency: 1}, SourceDir: "source"}, wantError: "temporary directory is required"},
		{name: "package size", opts: ReassembleOptions{PackageOptions: validPackageOptions, SourceDir: "source", MaxPackageSizeMB: -1}, wantError: "maximum package size must not be negative"},
		{name: "concurrency", opts: ReassembleOptions{PackageOptions: PackageOptions{TmpDir: t.TempDir()}, SourceDir: "source"}, wantError: "concurrency must be greater than zero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.validate()
			if tt.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantError)
		})
	}
}
