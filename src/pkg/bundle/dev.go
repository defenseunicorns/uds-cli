// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"

	zarfCLI "github.com/defenseunicorns/zarf/src/cmd"
	"github.com/defenseunicorns/zarf/src/pkg/message"
)

// CreateZarfPkgs creates a zarf package if its missing when in dev mode
func (b *Bundle) CreateZarfPkgs() {
	srcDir := b.cfg.CreateOpts.SourceDirectory
	bundleYAMLPath := filepath.Join(srcDir, b.cfg.CreateOpts.BundleFile)
	if err := utils.ReadYAMLStrict(bundleYAMLPath, &b.bundle); err != nil {
		message.Fatalf(err, "Failed to read %s, error in YAML: %s", b.cfg.CreateOpts.BundleFile, err.Error())
	}

	zarfPackagePattern := `^zarf-.*\.tar\.zst$`
	for _, pkg := range b.bundle.Packages {
		// Can only set flavors for local packages
		if pkg.Path == "" {
			// check if attempting to apply flavor to remote package
			if (len(b.cfg.DevDeployOpts.Flavor) == 1 && b.cfg.DevDeployOpts.Flavor[""] != "") ||
				(b.cfg.DevDeployOpts.Flavor[pkg.Name] != "") {
				message.Fatalf(errors.New("Invalid input"), "Cannot set flavor for remote packages: %s", pkg.Name)
			}
		}

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
			if !packageFound || b.cfg.DevDeployOpts.ForceCreate {
				if len(b.cfg.DevDeployOpts.Flavor) != 0 {
					pkg = b.setPackageFlavor(pkg)
					os.Args = []string{"zarf", "package", "create", pkgDir, "--confirm", "-o", pkgDir, "--skip-sbom", "--flavor", pkg.Flavor}
				} else {
					os.Args = []string{"zarf", "package", "create", pkgDir, "--confirm", "-o", pkgDir, "--skip-sbom"}
				}
				zarfCLI.Execute()
				if err != nil {
					message.Fatalf(err, "Failed to create package %s: %s", pkg.Name, err.Error())
				}
			}
		}
	}
}

func (b *Bundle) setPackageFlavor(pkg types.Package) types.Package {
	// handle case when --flavor flag applies to all packages
	// empty key references a value that is applied to all package flavors
	if len(b.cfg.DevDeployOpts.Flavor) == 1 && b.cfg.DevDeployOpts.Flavor[""] != "" {
		pkg.Flavor = b.cfg.DevDeployOpts.Flavor[""]
	} else if flavor, ok := b.cfg.DevDeployOpts.Flavor[pkg.Name]; ok {
		pkg.Flavor = flavor
	}
	return pkg
}

// SetDeploySource sets the source for the bundle when in dev mode
func (b *Bundle) SetDeploySource(srcDir string) {
	filename := fmt.Sprintf("%s%s-%s-%s.tar.zst", config.BundlePrefix, b.bundle.Metadata.Name, b.bundle.Metadata.Architecture, b.bundle.Metadata.Version)
	b.cfg.DeployOpts.Source = filepath.Join(srcDir, filename)
}
