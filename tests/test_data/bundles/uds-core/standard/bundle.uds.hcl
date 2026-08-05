# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name        = "k3d-core-demo"
  description = "A UDS bundle for deploying the standard UDS Core package on a development cluster"
  // renovate: datasource=github-tags depName=defenseunicorns/uds-core extractVersion=^v?(?<version>.*)$ versioning=semver
  version     = "1.9.0"
}

locals {
  uds_repo      = "ghcr.io/defenseunicorns/packages/uds"
  k3d_repo      = "ghcr.io/defenseunicorns/packages"
  // renovate: datasource=docker depName=ghcr.io/defenseunicorns/packages/uds/core versioning=docker
  core_version  = "1.9.0-upstream"
  // renovate: datasource=docker depName=ghcr.io/defenseunicorns/packages/uds-k3d versioning=docker
  k3d_version   = "0.20.2-airgap"
  // renovate: datasource=docker depName=ghcr.io/zarf-dev/packages/init versioning=docker
  init_version  = "v0.82.0"
}

package "uds_k3d_dev" {
  source = "oci://${local.k3d_repo}/uds-k3d:${local.k3d_version}"
  signature_verification { verify = false }
}

package "init" {
  source     = "oci://ghcr.io/zarf-dev/packages/init:${local.init_version}"
  signature_verification {
    keyless {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer      = "https://token.actions.githubusercontent.com"
    }
  }
  depends_on = [package.uds_k3d_dev]
}

package "core_base" {
  source     = "oci://${local.uds_repo}/core-base:${local.core_version}"
  signature_verification { verify = false }
  depends_on = [package.init]
  optional_components = [
    "istio-passthrough-gateway",
    "istio-egress-gateway",
    "envoy-gateway",
    "envoy-default-gateway",
  ]
  values_files = ["values/core-base.yaml"]
}

package "core_identity_authorization" {
  source       = "oci://${local.uds_repo}/core-identity-authorization:${local.core_version}"
  signature_verification { verify = false }
  depends_on   = [package.core_base]
  values_files = ["values/core-identity-authorization.yaml"]
}

package "core_logging" {
  source       = "oci://${local.uds_repo}/core-logging:${local.core_version}"
  signature_verification { verify = false }
  depends_on   = [package.core_base]
  values_files = ["values/core-logging.yaml"]
}

package "core_monitoring" {
  source       = "oci://${local.uds_repo}/core-monitoring:${local.core_version}"
  signature_verification { verify = false }
  depends_on   = [package.core_identity_authorization]
  values_files = ["values/core-monitoring.yaml"]
}

package "core_runtime_security" {
  source       = "oci://${local.uds_repo}/core-runtime-security:${local.core_version}"
  signature_verification { verify = false }
  depends_on   = [package.core_base]
  values_files = ["values/core-runtime-security.yaml"]
}

package "core_backup_restore" {
  source       = "oci://${local.uds_repo}/core-backup-restore:${local.core_version}"
  signature_verification { verify = false }
  depends_on   = [package.core_base]
  values_files = ["values/core-backup-restore.yaml"]
}

package "core_portal" {
  source     = "oci://${local.uds_repo}/core-portal:${local.core_version}"
  signature_verification { verify = false }
  depends_on = [package.core_identity_authorization]
}

package "core_metrics_server" {
  source     = "oci://${local.uds_repo}/core-metrics-server:${local.core_version}"
  signature_verification { verify = false }
  depends_on = [package.core_base]
}
