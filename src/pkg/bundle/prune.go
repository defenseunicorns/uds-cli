// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/sources"
	"github.com/defenseunicorns/uds-cli/src/pkg/state"
	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/fatih/color"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/message"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
	"k8s.io/apimachinery/pkg/api/errors"
)

func (b *Bundle) handlePrune(sc *state.Client, kc *cluster.Cluster) error {
	// get any unreferenced pkgs
	unreferencedPkgs, err := sc.GetUnreferencedPackages(&b.bundle)
	if err != nil {
		return err
	}
	if len(unreferencedPkgs) > 0 {
		fmt.Println("\n", message.RuleLine)
		message.HeaderInfof("🪓 PRUNING UNREFERENCED PACKAGES")

		// prompt user if no --confirm (noting dev deploy confirms automatically)
		if !config.CommonOptions.Confirm {
			err, cancel := b.prunePrompt(unreferencedPkgs)
			if err != nil {
				return err
			} else if cancel {
				return nil
			}
		}

		// remove unreferenced packages
		for _, pkg := range unreferencedPkgs {
			message.Infof("Removing unreferenced package: %v", pkg.Name)

			// set up Zarf pkg client
			opts := zarfTypes.ZarfPackageOptions{
				PackageSource: b.cfg.RemoveOpts.Source,
			}
			pkgCfg := zarfTypes.PackagerConfig{
				PkgOpts: opts,
			}
			pkgTmp, err := zarfUtils.MakeTempDir(config.CommonOptions.TempDirectory)
			if err != nil {
				return err
			}

			source, err := sources.NewFromZarfState(kc.Clientset, pkg.Name)
			if err != nil {
				if errors.IsNotFound(err) {
					// handles case where Zarf pkg is not found in cluster, but exists in UDS state (ie. pkgs with only actions)
					message.Debugf("Package %s state secret not found in cluster, updating UDS state", pkg.Name)
					err = sc.RemovePackageFromState(&b.bundle, pkg.Name)
					if err != nil {
						return err
					}
					continue
				}
				return err
			}

			pkgClient, err := packager.New(&pkgCfg, packager.WithSource(source), packager.WithTemp(pkgTmp))
			if err != nil {
				return err
			}
			defer pkgClient.ClearTempPaths()

			// remove package
			if removeErr := pkgClient.Remove(context.TODO()); removeErr != nil {
				err = sc.UpdateBundlePkgState(&b.bundle, pkg, state.FailedRemove)
				if err != nil {
					return err
				}
				return removeErr
			}

			err = sc.RemovePackageFromState(&b.bundle, pkg.Name)
			if err != nil {
				return err
			}
			message.Success("Package removed")
		}
	}
	return nil
}

func (b *Bundle) prunePrompt(unreferencedPkgs []types.Package) (error, bool) {
	// format a list of pkg names and print prompt
	unreferencedPkgNames := make([]string, 0)
	for _, pkg := range unreferencedPkgs {
		unreferencedPkgNames = append(unreferencedPkgNames, pkg.Name)
	}
	pkgList := strings.Join(unreferencedPkgNames, "\n  - ")
	cyan := color.New(color.FgCyan).SprintFunc()
	styledBundleName := cyan(b.bundle.Metadata.Name)
	promptMessage := fmt.Sprintf("The following packages are no longer referenced by the bundle %s:\n  - %s\n\nAttempt removal of these packages?", styledBundleName, pkgList)
	prompt := &survey.Confirm{
		Message: promptMessage,
	}
	if err := survey.AskOne(prompt, &config.CommonOptions.Confirm); err != nil {
		return fmt.Errorf("failed to prompt user: %w", err), true
	}

	if !config.CommonOptions.Confirm {
		message.Info("Canceled prune operation")
		return nil, true
	}

	return nil, false
}
