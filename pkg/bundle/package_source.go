// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

// NewPackageSource returns a PackageSource for the given source string.
// OCI references (detected by IsOCIReference) use zoci.NewRemote;
// everything else is treated as a local path resolved against bundleDir.
func NewPackageSource(source string, opts ConfigOptions, bundleDir string) PackageSource {
	if IsOCIReference(source) {
		return &remoteSource{
			ref:  TrimScheme(source),
			arch: opts.Architecture,
			opts: opts,
		}
	}
	return &localSource{
		path:      source,
		arch:      opts.Architecture,
		bundleDir: bundleDir,
		tmpDir:    opts.TmpDir,
	}
}
