# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name    = "variables-test"
  version = "0.1.0"
}

package "uds_k3d_dev" {
  source       = "oci://ghcr.io/defenseunicorns/packages/uds-k3d:0.19.4"
  values_files = ["values/k3d.yaml"]
}
