// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import "github.com/defenseunicorns/uds-cli/pkg/iostreams"

// Options holds inputs for disassembling an artifact into local source.
type Options struct {
	Source        string
	OutputDir     string
	Architecture  string
	PlainHTTP     bool
	SkipTLSVerify bool
	TmpDir        string
	Concurrency   int
	Streams       iostreams.IOStreams
}

// Result describes source emitted by a successful disassembly.
type Result struct {
	Source    string `json:"source" yaml:"source" text:"Source"`
	OutputDir string `json:"outputDir" yaml:"outputDir" text:"Output Directory"`
}
