// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/state"
)

func TestMapPackagesToBundles(t *testing.T) {
	t.Run("groups packages by bundle name and version", func(t *testing.T) {
		deployedPackages := []state.DeployedPackage{
			{
				Name: "podinfo",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "podinfo",
						Version: "6.4.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "demo-bundle",
							AnnotationBundleVersion: "1.0.0",
						},
					},
				},
			},
			{
				Name: "prometheus",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "prometheus",
						Version: "2.45.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "demo-bundle",
							AnnotationBundleVersion: "1.0.0",
						},
					},
				},
			},
		}

		bundles := mapPackagesToBundles(deployedPackages)

		require.Equal(t, 1, len(bundles))
		require.Equal(t, "demo-bundle", bundles[0].Name)
		require.Equal(t, "1.0.0", bundles[0].Version)
		require.Equal(t, 2, len(bundles[0].Packages))
		require.Contains(t, bundles[0].Packages, "podinfo:6.4.0")
		require.Contains(t, bundles[0].Packages, "prometheus:2.45.0")
	})

	t.Run("filters out packages without bundle annotations", func(t *testing.T) {
		deployedPackages := []state.DeployedPackage{
			{
				Name: "init",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "init",
						Version: "0.38.2",
						// No annotations - standalone Zarf package
					},
				},
			},
			{
				Name: "podinfo",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "podinfo",
						Version: "6.4.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "demo-bundle",
							AnnotationBundleVersion: "1.0.0",
						},
					},
				},
			},
		}

		bundles := mapPackagesToBundles(deployedPackages)

		require.Equal(t, 1, len(bundles))
		require.Equal(t, "demo-bundle", bundles[0].Name)
		require.Equal(t, 1, len(bundles[0].Packages))
	})

	t.Run("handles multiple bundles", func(t *testing.T) {
		deployedPackages := []state.DeployedPackage{
			{
				Name: "podinfo",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "podinfo",
						Version: "6.4.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "demo-bundle",
							AnnotationBundleVersion: "1.0.0",
						},
					},
				},
			},
			{
				Name: "nginx",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "nginx",
						Version: "1.25.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "web-bundle",
							AnnotationBundleVersion: "2.1.0",
						},
					},
				},
			},
		}

		bundles := mapPackagesToBundles(deployedPackages)

		require.Equal(t, 2, len(bundles))
		// Bundles should be sorted alphabetically by name
		require.Equal(t, "demo-bundle", bundles[0].Name)
		require.Equal(t, "web-bundle", bundles[1].Name)
	})

	t.Run("handles same bundle name with different versions", func(t *testing.T) {
		deployedPackages := []state.DeployedPackage{
			{
				Name: "pkg-v1",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "pkg-v1",
						Version: "1.0.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "my-bundle",
							AnnotationBundleVersion: "1.0.0",
						},
					},
				},
			},
			{
				Name: "pkg-v2",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "pkg-v2",
						Version: "2.0.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "my-bundle",
							AnnotationBundleVersion: "2.0.0",
						},
					},
				},
			},
		}

		bundles := mapPackagesToBundles(deployedPackages)

		require.Equal(t, 2, len(bundles))
		// Should be sorted by version within the same bundle name
		require.Equal(t, "1.0.0", bundles[0].Version)
		require.Equal(t, "2.0.0", bundles[1].Version)
		require.Equal(t, 1, len(bundles[0].Packages))
		require.Equal(t, 1, len(bundles[1].Packages))
	})

	t.Run("filters packages with incomplete annotations", func(t *testing.T) {
		deployedPackages := []state.DeployedPackage{
			{
				Name: "pkg-no-version",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "pkg-no-version",
						Version: "1.0.0",
						Annotations: map[string]string{
							AnnotationBundleName: "incomplete-bundle",
							// Missing bundle version
						},
					},
				},
			},
			{
				Name: "pkg-no-name",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "pkg-no-name",
						Version: "1.0.0",
						Annotations: map[string]string{
							AnnotationBundleVersion: "1.0.0",
							// Missing bundle name
						},
					},
				},
			},
			{
				Name: "pkg-complete",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "pkg-complete",
						Version: "1.0.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "complete-bundle",
							AnnotationBundleVersion: "1.0.0",
						},
					},
				},
			},
		}

		bundles := mapPackagesToBundles(deployedPackages)

		require.Equal(t, 1, len(bundles))
		require.Equal(t, "complete-bundle", bundles[0].Name)
		require.Equal(t, 1, len(bundles[0].Packages))
	})

	t.Run("handles empty package list", func(t *testing.T) {
		deployedPackages := []state.DeployedPackage{}

		bundles := mapPackagesToBundles(deployedPackages)

		require.Equal(t, 0, len(bundles))
	})

	t.Run("handles nil annotations", func(t *testing.T) {
		deployedPackages := []state.DeployedPackage{
			{
				Name: "pkg-nil-annotations",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:        "pkg-nil-annotations",
						Version:     "1.0.0",
						Annotations: nil,
					},
				},
			},
		}

		bundles := mapPackagesToBundles(deployedPackages)

		require.Equal(t, 0, len(bundles))
	})

	t.Run("sorts packages within each bundle", func(t *testing.T) {
		deployedPackages := []state.DeployedPackage{
			{
				Name: "zebra",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "zebra",
						Version: "1.0.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "sorted-bundle",
							AnnotationBundleVersion: "1.0.0",
						},
					},
				},
			},
			{
				Name: "alpha",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "alpha",
						Version: "1.0.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "sorted-bundle",
							AnnotationBundleVersion: "1.0.0",
						},
					},
				},
			},
			{
				Name: "beta",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "beta",
						Version: "1.0.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "sorted-bundle",
							AnnotationBundleVersion: "1.0.0",
						},
					},
				},
			},
		}

		bundles := mapPackagesToBundles(deployedPackages)

		require.Equal(t, 1, len(bundles))
		require.Equal(t, 3, len(bundles[0].Packages))
		// Packages should be sorted alphabetically
		require.Equal(t, "alpha:1.0.0", bundles[0].Packages[0])
		require.Equal(t, "beta:1.0.0", bundles[0].Packages[1])
		require.Equal(t, "zebra:1.0.0", bundles[0].Packages[2])
	})

	t.Run("complex scenario with multiple bundles and versions", func(t *testing.T) {
		deployedPackages := []state.DeployedPackage{
			// init package without bundle annotations
			{
				Name: "init",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "init",
						Version: "0.38.2",
					},
				},
			},
			// demo-bundle v1.0.0
			{
				Name: "podinfo",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "podinfo",
						Version: "6.4.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "demo-bundle",
							AnnotationBundleVersion: "1.0.0",
						},
					},
				},
			},
			{
				Name: "prometheus",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "prometheus",
						Version: "2.45.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "demo-bundle",
							AnnotationBundleVersion: "1.0.0",
						},
					},
				},
			},
			// demo-bundle v2.0.0
			{
				Name: "podinfo",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "podinfo",
						Version: "6.5.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "demo-bundle",
							AnnotationBundleVersion: "2.0.0",
						},
					},
				},
			},
			// web-bundle
			{
				Name: "nginx",
				Data: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:    "nginx",
						Version: "1.25.0",
						Annotations: map[string]string{
							AnnotationBundleName:    "web-bundle",
							AnnotationBundleVersion: "1.0.0",
						},
					},
				},
			},
		}

		bundles := mapPackagesToBundles(deployedPackages)

		require.Equal(t, 3, len(bundles))

		// Verify sorting: demo-bundle 1.0.0, demo-bundle 2.0.0, web-bundle 1.0.0
		require.Equal(t, "demo-bundle", bundles[0].Name)
		require.Equal(t, "1.0.0", bundles[0].Version)
		require.Equal(t, 2, len(bundles[0].Packages))

		require.Equal(t, "demo-bundle", bundles[1].Name)
		require.Equal(t, "2.0.0", bundles[1].Version)
		require.Equal(t, 1, len(bundles[1].Packages))

		require.Equal(t, "web-bundle", bundles[2].Name)
		require.Equal(t, "1.0.0", bundles[2].Version)
		require.Equal(t, 1, len(bundles[2].Packages))
	})
}

func TestPrintBundleList(t *testing.T) {
	t.Run("handles empty bundle list", func(t *testing.T) {
		// This test just ensures the function doesn't panic with empty input
		bundles := []BundleDeployment{}
		// Should output a warning message, but not panic
		PrintBundleList(bundles)
	})

	t.Run("handles bundle list with data", func(t *testing.T) {
		bundles := []BundleDeployment{
			{
				Name:     "test-bundle",
				Version:  "1.0.0",
				Packages: []string{"pkg1:1.0.0", "pkg2:2.0.0"},
			},
		}
		// Should output formatted data, but not panic
		PrintBundleList(bundles)
	})
}
