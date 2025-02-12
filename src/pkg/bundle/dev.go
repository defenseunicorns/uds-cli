// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	"github.com/defenseunicorns/uds-cli/src/types"
	zarfCmd "github.com/zarf-dev/zarf/src/cmd"

	// zarfTools "github.com/zarf-dev/zarf/src/cmd/tools"
	"github.com/zarf-dev/zarf/src/pkg/layout"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	zarfTypes "github.com/zarf-dev/zarf/src/types"

	zarfCLI "github.com/zarf-dev/zarf/src/cmd"
)

// CreateZarfPkgs creates a zarf package if its missing when in dev mode
func (b *Bundle) CreateZarfPkgs() error {
	srcDir := b.cfg.CreateOpts.SourceDirectory
	bundleYAMLPath := filepath.Join(srcDir, b.cfg.CreateOpts.BundleFile)
	if err := utils.ReadYAMLStrict(bundleYAMLPath, &b.bundle); err != nil {
		return fmt.Errorf("failed to read %s, error in YAML: %s", b.cfg.CreateOpts.BundleFile, err.Error())
	}

	zarfPackagePattern := `^zarf-.*\.tar\.zst$`
	for _, pkg := range b.bundle.Packages {
		// Can only set flavors for local packages
		if pkg.Path == "" {
			// check if attempting to apply flavor to remote package
			if (len(b.cfg.DevDeployOpts.Flavor) == 1 && b.cfg.DevDeployOpts.Flavor[""] != "") ||
				(b.cfg.DevDeployOpts.Flavor[pkg.Name] != "") {
				return fmt.Errorf("cannot set flavor for remote packages: %s", pkg.Name)
			}
		}

		// if pkg is a local zarf package, attempt to create it if it doesn't exist
		if pkg.Path != "" {
			path := getPkgPath(pkg, config.GetArch(b.bundle.Metadata.Architecture), srcDir)
			pkgDir := filepath.Dir(path)
			// get files in directory
			files, err := os.ReadDir(pkgDir)
			if err != nil {
				return fmt.Errorf("failed to obtain package %s: %s", pkg.Name, err.Error())
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
				zarfCLI.Execute(context.TODO())
			}
		}
	}
	return nil
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

func (b *Bundle) extractPackage(path string, pkg types.Package) error {
	pkgTmp, err := zarfUtils.MakeTempDir("")
	if err != nil {
		return err
	}
	defer os.RemoveAll(pkgTmp)

	nsOverrides := sources.NamespaceOverrideMap{}
	sha := strings.Split(pkg.Ref, "@sha256:")[1] // using appended SHA from create!
	opts := zarfTypes.ZarfPackageOptions{PackageSource: pkgTmp}

	// NOTE: The source.Collect() methods are not implemented for these 'sources'. It would be more convient if they were.
	// NOTE: The below code would simplfy down to `source.Collect(...)`
	source, err := sources.NewFromLocation(*b.cfg, pkg, opts, sha, nsOverrides)
	if err != nil {
		return err
	}

	packagePath := layout.New(pkgTmp)
	filters := filters.Empty()
	loadedPkg, _, err := source.LoadPackage(context.TODO(), packagePath, filters, false)
	if err != nil {
		return err
	}

	// NOTE: filepath.Join() strips the trailing '/' and we need that for this command
	archiveFilePath := pkgTmp + string(filepath.Separator)

	tarballNameTemplate := "zarf-package-%s-%s-%s.tar.zst"
	if pkg.Name == "init" {
		// zarf-init packages are 'special' and don't have the 'zarf-package' prefix
		tarballNameTemplate = "zarf-%s-%s-%s.tar.zst"
	}
	tarballName := fmt.Sprintf(tarballNameTemplate, pkg.Name, loadedPkg.Metadata.Architecture, loadedPkg.Metadata.Version)

	zarfCmd := zarfCmd.NewZarfCommand()
	zarfCmd.SetArgs([]string{"tools", "archiver", "compress", archiveFilePath, filepath.Join(path, tarballName)})
	err = zarfCmd.Execute()
	if err != nil {
		return err
	}

	return nil
}

func (b *Bundle) GetDefaultExtractPath() string {
	return filepath.Join(b.tmp, "extracted-packages")
}

func (b *Bundle) Extract(path string) error {
	// Create the directory that we will store the extract tarballs into
	if err := helpers.CreateDirectory(path, helpers.ReadWriteExecuteUser); err != nil {
		return err
	}

	// Extract each Zarf Package that is within the bundle
	for _, pkg := range b.bundle.Packages {
		if err := b.extractPackage(path, pkg); err != nil {
			return err
		}
	}

	return nil
}
