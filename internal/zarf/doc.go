// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package zarf deploys, removes, verifies, and stages Zarf packages used by UDS
// bundles.
//
// It loads package metadata from bundle artifacts or source declarations,
// resolves local and remote package sources, applies component filters from the
// bundle configuration, rebuilds package layouts from stored OCI descriptors,
// and calls the upstream Zarf APIs that perform package-level operations. When
// upstream Zarf exposes a higher-level package-layout method, prefer that over
// reading the layout through ORAS directly.
package zarf
