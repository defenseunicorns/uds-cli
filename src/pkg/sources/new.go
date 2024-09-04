// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package sources contains Zarf packager sources
package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	zarfSources "github.com/zarf-dev/zarf/src/pkg/packager/sources"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// NewFromLocation creates a new package source based on pkgLocation
func NewFromLocation(bundleCfg types.BundleConfig, pkg types.Package, opts zarfTypes.ZarfPackageOptions, sha string, nsOverrides NamespaceOverrideMap) (zarfSources.PackageSource, error) {
	var source zarfSources.PackageSource
	var pkgLocation string
	if bundleCfg.DeployOpts.Source != "" {
		pkgLocation = bundleCfg.DeployOpts.Source
	} else if bundleCfg.RemoveOpts.Source != "" {
		pkgLocation = bundleCfg.RemoveOpts.Source
	} else if bundleCfg.InspectOpts.Source != "" {
		pkgLocation = bundleCfg.InspectOpts.Source
	} else {
		return nil, fmt.Errorf("no source provided for package %s", pkg.Name)
	}

	if strings.Contains(pkgLocation, "tar.zst") {
		source = &TarballBundle{
			Pkg:            pkg,
			PkgOpts:        &opts,
			PkgManifestSHA: sha,
			TmpDir:         opts.PackageSource,
			BundleLocation: pkgLocation,
			nsOverrides:    nsOverrides,
		}
	} else {
		platform := ocispec.Platform{
			Architecture: config.GetArch(),
			OS:           oci.MultiOS,
		}
		remote, err := zoci.NewRemote(pkgLocation, platform)
		if err != nil {
			return nil, err
		}
		source = &RemoteBundle{
			Pkg:            pkg,
			PkgOpts:        &opts,
			PkgManifestSHA: sha,
			TmpDir:         opts.PackageSource,
			Remote:         remote.OrasRemote,
			nsOverrides:    nsOverrides,
			bundleCfg:      bundleCfg,
		}
	}
	return source, nil
}

// NewFromZarfState creates a new ZarfState package source
// only used for remove operations on prune
func NewFromZarfState(client kubernetes.Interface, pkgName string) (*ZarfState, error) {
	// get secret from K8s
	secretName := fmt.Sprintf("zarf-package-%s", pkgName)
	sec, err := client.CoreV1().Secrets("zarf").Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return nil, nil
	}

	// marshal secret to state
	var state zarfTypes.DeployedPackage
	err = json.Unmarshal(sec.Data["data"], &state)
	if err != nil {
		return nil, err
	}

	return &ZarfState{state: state}, nil
}
