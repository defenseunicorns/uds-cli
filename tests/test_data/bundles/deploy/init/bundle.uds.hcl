# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

# Minimal bundle for testing the deploy command.
# This bundle includes two packages with a dependency relationship:
# - uds_k3d_dev: Creates a local k3d cluster
# - init: The Zarf init package (depends on k3d cluster)

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name        = "k3d-core-init"
  description = "Minimal bundle for testing deploy command"
  version     = "0.1.0"
}

package "uds_k3d_dev" {
  source = "oci://ghcr.io/defenseunicorns/packages/uds-k3d:0.19.5"
}

package "init" {
  source     = "oci://ghcr.io/zarf-dev/packages/init:v0.74.2"
  depends_on = [package.uds_k3d_dev]
}

