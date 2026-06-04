// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import "github.com/defenseunicorns/uds-cli/pkg/iostreams"

// NewPackageSource returns a PackageSource for the given source string.
// OCI references (detected by IsOCIReference) use zoci.NewRemote;
// everything else is treated as a local path resolved against bundleDir.
// streams carries the leveled logger used for ingest/pull diagnostics.
func NewPackageSource(source string, opts ConfigOptions, bundleDir string, streams iostreams.IOStreams) PackageSource {
	if IsOCIReference(source) {
		return &remoteSource{
			ref:     TrimScheme(source),
			arch:    opts.Architecture,
			opts:    opts,
			streams: streams,
		}
	}
	return &localSource{
		path:      source,
		arch:      opts.Architecture,
		bundleDir: bundleDir,
		tmpDir:    opts.TmpDir,
		streams:   streams,
	}
}
