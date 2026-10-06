// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"errors"
	"strings"

	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
)

// PackageOptions holds Zarf package settings shared by disassembly and reassembly.
type PackageOptions struct {
	PlainHTTP     bool
	SkipTLSVerify bool
	TmpDir        string
	CachePath     string
	Concurrency   int
}

// Options holds inputs for disassembling an artifact into local source.
type Options struct {
	PackageOptions
	Source               string
	OutputDir            string
	VerificationStrategy layout.VerificationStrategy
	Warn                 func(string, ...any)
}

func (o Options) validate() error {
	switch {
	case strings.TrimSpace(o.Source) == "":
		return errors.New("source is required")
	case strings.TrimSpace(o.OutputDir) == "":
		return errors.New("output directory is required")
	case strings.TrimSpace(o.TmpDir) == "":
		return errors.New("temporary directory is required")
	case o.Concurrency < 1:
		return errors.New("concurrency must be greater than zero")
	default:
		return nil
	}
}

// ReassembleOptions holds inputs for recreating a package from disassembled source.
type ReassembleOptions struct {
	PackageOptions
	SourceDir            string
	Output               string
	MaxPackageSizeMB     int
	SigningKeyPath       string
	SigningKeyPassword   string
	WithBuildMachineInfo bool
}

func (o ReassembleOptions) validate() error {
	switch {
	case strings.TrimSpace(o.SourceDir) == "":
		return errors.New("source directory is required")
	case strings.TrimSpace(o.TmpDir) == "":
		return errors.New("temporary directory is required")
	case o.MaxPackageSizeMB < 0:
		return errors.New("maximum package size must not be negative")
	case o.Concurrency < 1:
		return errors.New("concurrency must be greater than zero")
	default:
		return nil
	}
}
