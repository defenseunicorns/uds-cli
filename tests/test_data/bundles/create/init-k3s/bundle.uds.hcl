# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

# Bundle for testing create with optional component inclusion.
# The init package's k3s component is optional; listing it in
# optional_components ensures its layer blob is included in the bundle.

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name        = "init-k3s"
  description = "Bundle for testing create with k3s optional component included"
  version     = "0.1.0"
}

package "init" {
  source              = "oci://ghcr.io/zarf-dev/packages/init:v0.85.0"
  signature_verification {
    keyless {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer      = "https://token.actions.githubusercontent.com"
    }
  }
  optional_components = ["k3s"]
}
