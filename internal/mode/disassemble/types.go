// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import "github.com/zarf-dev/zarf/src/pkg/packager/layout"

// Options holds inputs for disassembling an artifact into local source.
type Options struct {
	Source               string
	OutputDir            string
	PlainHTTP            bool
	SkipTLSVerify        bool
	TmpDir               string
	CachePath            string
	Concurrency          int
	VerificationStrategy layout.VerificationStrategy
	Warn                 func(string, ...any)
}

// Result describes source emitted by a successful disassembly.
type Result struct {
	Source    string `json:"source" yaml:"source" text:"Source"`
	OutputDir string `json:"outputDir" yaml:"outputDir" text:"Output Directory"`
}

// ReassembleOptions holds inputs for recreating a package from disassembled source.
type ReassembleOptions struct {
	SourceDir            string
	Output               string
	PlainHTTP            bool
	SkipTLSVerify        bool
	CachePath            string
	MaxPackageSizeMB     int
	SigningKeyPath       string
	SigningKeyPassword   string
	WithBuildMachineInfo bool
	Concurrency          int
}

// ReassembleResult describes a package created from disassembled source.
type ReassembleResult struct {
	SourceDir  string `json:"sourceDir" yaml:"sourceDir" text:"Source Directory"`
	OutputPath string `json:"outputPath" yaml:"outputPath" text:"Output Path"`
}
