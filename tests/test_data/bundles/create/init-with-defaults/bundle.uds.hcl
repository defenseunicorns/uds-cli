# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

# Bundle for testing defaults.uds.hcl integration.
# The defaults.uds.hcl in this directory sets architecture = "amd64",
# which should be applied during create when no CLI flag overrides it.

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name        = "defaults-test"
  description = file("description.txt")
  version     = "0.1.0"
}

package "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.83.0"
  signature_verification {
    keyless {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer      = "https://token.actions.githubusercontent.com"
    }
  }
}
