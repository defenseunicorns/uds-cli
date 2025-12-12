// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"sort"

	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/state"
)

const (
	// Annotation keys for bundle metadata (following OCI image spec)
	AnnotationBundleName    = "dev.uds.bundle.name"
	AnnotationBundleVersion = "dev.uds.bundle.version"
)

// BundleDeployment represents a deployed bundle with its packages
type BundleDeployment struct {
	Name     string
	Version  string
	Packages []string
}

// ListDeployedBundles retrieves all deployed Zarf packages and maps them to bundles
// This is an entrypoint to extension if required (beyond zarf package annotation mapping)
func ListDeployedBundles(ctx context.Context) ([]BundleDeployment, error) {
	c, err := cluster.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to cluster: %w", err)
	}

	// Get all deployed Zarf packages
	deployedPackages, err := c.GetDeployedZarfPackages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployed packages: %w", err)
	}

	return mapPackagesToBundles(deployedPackages), nil
}

// mapPackagesToBundles maps deployed packages to bundles based on annotations
func mapPackagesToBundles(deployedPackages []state.DeployedPackage) []BundleDeployment {
	// Map packages to bundles based on annotations
	bundleMap := make(map[string]*BundleDeployment)

	for _, pkg := range deployedPackages {
		// Check if package has bundle annotations
		annotations := pkg.Data.Metadata.Annotations
		if annotations == nil {
			continue
		}

		bundleName, hasBundleName := annotations[AnnotationBundleName]
		bundleVersion, hasBundleVersion := annotations[AnnotationBundleVersion]

		// Only include packages that have both bundle name and version annotations
		if !hasBundleName || !hasBundleVersion {
			continue
		}

		bundleKey := fmt.Sprintf("%s:%s", bundleName, bundleVersion)
		pkgIdentifier := fmt.Sprintf("%s:%s", pkg.Name, pkg.Data.Metadata.Version)

		if bundle, exists := bundleMap[bundleKey]; exists {
			bundle.Packages = append(bundle.Packages, pkgIdentifier)
		} else {
			bundleMap[bundleKey] = &BundleDeployment{
				Name:     bundleName,
				Version:  bundleVersion,
				Packages: []string{pkgIdentifier},
			}
		}
	}

	// Convert map to sorted slice
	bundles := make([]BundleDeployment, 0, len(bundleMap))
	for _, bundle := range bundleMap {
		// Sort packages within each bundle
		sort.Strings(bundle.Packages)
		bundles = append(bundles, *bundle)
	}

	// Sort bundles by name, then version
	sort.Slice(bundles, func(i, j int) bool {
		if bundles[i].Name != bundles[j].Name {
			return bundles[i].Name < bundles[j].Name
		}
		return bundles[i].Version < bundles[j].Version
	})

	return bundles
}

// PrintBundleList prints the deployed bundles in a formatted table to stdout
func PrintBundleList(bundles []BundleDeployment) {
	if len(bundles) == 0 {
		fmt.Fprintln(message.OutputWriter, "No deployed bundles found in the cluster")
		return
	}

	fmt.Fprintln(message.OutputWriter, "BUNDLE NAME      VERSION    PACKAGES")
	fmt.Fprintln(message.OutputWriter, "───────────────────────────────────────────────────────────────────────")

	for _, bundle := range bundles {
		fmt.Fprintf(message.OutputWriter, "%-16s %-10s %d\n", bundle.Name, bundle.Version, len(bundle.Packages))
		for _, pkg := range bundle.Packages {
			fmt.Fprintf(message.OutputWriter, "  └─ %s\n", pkg)
		}
	}
}
