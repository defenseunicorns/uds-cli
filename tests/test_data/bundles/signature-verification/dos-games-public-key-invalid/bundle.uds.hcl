# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name        = "signature-verification-dos-games-public-key-invalid"
  description = "Zarf dos-games package with a non-matching public-key verification policy"
  version     = "0.1.0"
}

package "dos_games" {
  source = "oci://ghcr.io/zarf-dev/packages/dos-games:1.3.0"

  signature_verification {
    public_key = file("wrong-cosign.pub")
  }
}
