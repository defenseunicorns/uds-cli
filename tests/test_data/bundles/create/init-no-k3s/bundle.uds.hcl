# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

# Bundle for testing create with optional component exclusion.
# The init package's k3s component is optional; omitting optional_components
# means all optional components (including k3s) are excluded from the bundle.

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name        = "init-no-k3s"
  description = "Bundle for testing create with k3s optional component excluded"
  version     = "0.1.0"
}

package "init" {
  source     = "oci://ghcr.io/zarf-dev/packages/init:v0.77.0"
}
