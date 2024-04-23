// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/defenseunicorns/uds-cli/src/config"

	zarfCLI "github.com/defenseunicorns/zarf/src/cmd"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
)

// CreateZarfPkgs creates a zarf package if its missing when in dev mode
func (b *Bundle) CreateZarfPkgs() {
	srcDir := b.cfg.CreateOpts.SourceDirectory
	bundleYAMLPath := filepath.Join(srcDir, b.cfg.CreateOpts.BundleFile)
	if err := zarfUtils.ReadYaml(bundleYAMLPath, &b.bundle); err != nil {
		message.Fatalf(err, "Failed to read bundle.yaml: %s", err.Error())
	}

	zarfPackagePattern := `^zarf-.*\.tar\.zst$`
	for _, pkg := range b.bundle.Packages {
		// if pkg is a local zarf package, attempt to create it if it doesn't exist
		if pkg.Path != "" {
			path := getPkgPath(pkg, config.GetArch(b.bundle.Metadata.Architecture), srcDir)
			pkgDir := filepath.Dir(path)
			// get files in directory
			files, err := os.ReadDir(pkgDir)
			if err != nil {
				message.Fatalf(err, "Failed to obtain package %s: %s", pkg.Name, err.Error())
			}
			regex := regexp.MustCompile(zarfPackagePattern)

			// check if package exists
			packageFound := false
			for _, file := range files {
				if regex.MatchString(file.Name()) {
					packageFound = true
					break
				}
			}
			// create local zarf package if it doesn't exist
			if !packageFound {
				os.Args = []string{"zarf", "package", "create", pkgDir, "--confirm", "-o", pkgDir, "--skip-sbom"}
				zarfCLI.Execute()
				if err != nil {
					message.Fatalf(err, "Failed to create package %s: %s", pkg.Name, err.Error())
				}
			}
		}
	}
}

// SetDevSource sets the source for the bundle when in dev mode
func (b *Bundle) SetDevSource(srcDir string) {
	srcDir = filepath.Dir(srcDir)
	// Add a trailing slash if it's missing
	if len(srcDir) != 0 && srcDir[len(srcDir)-1] != '/' {
		srcDir = srcDir + "/"
	}
	filename := fmt.Sprintf("%s%s-%s-%s.tar.zst", config.BundlePrefix, b.bundle.Metadata.Name, b.bundle.Metadata.Architecture, b.bundle.Metadata.Version)
	b.cfg.DeployOpts.Source = filepath.Join(srcDir, filename)
}
