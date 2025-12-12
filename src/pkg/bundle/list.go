// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"sort"

	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
)

const (
	// Annotation keys for bundle metadata (following OCI image spec)
	AnnotationBundleName    = "dev.defenseunicorns.uds.bundle.name"
	AnnotationBundleVersion = "dev.defenseunicorns.uds.bundle.version"
)

// BundleDeployment represents a deployed bundle with its packages
type BundleDeployment struct {
	Name     string
	Version  string
	Packages []string
}

// ListDeployedBundles retrieves all deployed Zarf packages and maps them to bundles
func ListDeployedBundles(ctx context.Context) ([]BundleDeployment, error) {
	// Create cluster client
	c, err := cluster.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to cluster: %w", err)
	}

	// Get all deployed Zarf packages
	deployedPackages, err := c.GetDeployedZarfPackages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployed packages: %w", err)
	}

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

		// Create bundle key (name:version)
		bundleKey := fmt.Sprintf("%s:%s", bundleName, bundleVersion)

		// Add or update bundle in map
		if bundle, exists := bundleMap[bundleKey]; exists {
			bundle.Packages = append(bundle.Packages, pkg.Name)
		} else {
			bundleMap[bundleKey] = &BundleDeployment{
				Name:     bundleName,
				Version:  bundleVersion,
				Packages: []string{pkg.Name},
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

	return bundles, nil
}

// PrintBundleList prints the deployed bundles in a formatted table
func PrintBundleList(bundles []BundleDeployment) {
	if len(bundles) == 0 {
		message.Warn("No deployed bundles found in the cluster")
		return
	}

	message.Title("Deployed Bundles", "")
	message.HorizontalRule()

	for _, bundle := range bundles {
		message.Infof("Bundle: %s", bundle.Name)
		message.Infof("Version: %s", bundle.Version)
		message.Infof("Packages (%d):", len(bundle.Packages))
		for _, pkg := range bundle.Packages {
			message.Infof("  - %s", pkg)
		}
		message.HorizontalRule()
	}
}
