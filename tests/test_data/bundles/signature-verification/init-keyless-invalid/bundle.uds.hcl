# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name        = "signature-verification-init-keyless-invalid"
  description = "Zarf init package with a non-matching keyless identity policy"
  version     = "0.1.0"
}

package "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.85.0"

  signature_verification {
    keyless {
      certificate_identity = "https://github.com/zarf-dev/zarf/.github/workflows/release.yml@refs/tags/not-a-real-release"
      certificate_oidc_issuer = "https://token.actions.githubusercontent.com"
    }
  }
}
