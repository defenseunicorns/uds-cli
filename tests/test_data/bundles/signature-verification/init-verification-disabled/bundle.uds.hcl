# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name        = "signature-verification-init-disabled"
  description = "Zarf init package with signature verification explicitly disabled"
  version     = "0.1.0"
}

package "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.82.0"

  signature_verification {
    verify = false
  }
}
