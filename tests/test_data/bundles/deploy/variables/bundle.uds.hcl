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
  source       = "oci://ghcr.io/defenseunicorns/packages/uds-k3d:0.20.0"
  values_files = ["values/k3d.yaml"]
}

package "init" {
  source     = "oci://ghcr.io/zarf-dev/packages/init:v0.75.1"
  depends_on = [package.uds_k3d_dev]
}

package "podinfo" {
  source       = "./zarf-package-podinfo-${sys.arch}-0.1.0.tar.zst"
  values_files = ["values/podinfo.yaml"]
  depends_on   = [package.init]
}
