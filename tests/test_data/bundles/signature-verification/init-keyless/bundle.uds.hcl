# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

# Test keyless verification.
uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name        = "signature-verification-init-keyless"
  description = "Zarf init package with keyless signature verification"
  version     = "0.1.0"
}

package "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.84.0"

  signature_verification {
    keyless {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer = "https://token.actions.githubusercontent.com"
    }
  }
}
