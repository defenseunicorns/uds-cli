# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

# Test public-key verification and HCL file() materialization. The key is the
# Zarf-published verification key for the dos-games package.
uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name        = "signature-verification-dos-games-public-key"
  description = "Zarf dos-games package with file-backed public-key verification"
  version     = "0.1.0"
}

package "dos_games" {
  source = "oci://ghcr.io/zarf-dev/packages/dos-games:1.3.0"

  signature_verification {
    public_key = file("cosign.pub")
  }
}
