# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

# This bundle is a spec-compliant example that uses ONLY the attributes
# defined in the Better Bundles Technical Design (docs/specs/1_Better_Bundles.md).
# It serves as the baseline for testing HCL parsing and the inspect command.
#
# This example demonstrates:
# - Bundle definition with locals, metadata, and packages
# - Deploy-time variable templating in values files using {{ .vars.* }} syntax
# - Package dependencies with depends_on
# - Namespace overrides

uds {
  bundle_api_version = "uds.dev/v1alpha1"
}

metadata {
  name        = "uds-core-example"
  description = "A spec-compliant UDS Core bundle example with base, logging, and monitoring packages"
  version     = "0.1.0"
}

locals {
  repo    = "ghcr.io/defenseunicorns/packages/uds"
  // renovate: datasource=docker depName=ghcr.io/defenseunicorns/packages/uds/core versioning=docker
  version = "1.9.0-upstream"

  pkgs = {
    base       = "core-base"
    logging    = "core-logging"
    monitoring = "core-monitoring"
  }
}

# Base package - no dependencies, deployed first
package "core_base" {
  source = "oci://${local.repo}/${local.pkgs.base}:${local.version}"
  signature_verification { verify = false }
  optional_components = [
    "istio-passthrough-gateway",
    "istio-egress-gateway",
  ]
}

# Logging package - depends on base being deployed first
package "core_logging" {
  source     = "oci://${local.repo}/${local.pkgs.logging}:${local.version}"
  signature_verification { verify = false }
  depends_on = [package.core_base]
  values_files = [
    "values/loki.yaml",
    "values/vector.yaml",
  ]
}

# Monitoring package - depends on both base and logging
package "core_monitoring" {
  source     = "oci://${local.repo}/${local.pkgs.monitoring}:${local.version}"
  signature_verification { verify = false }
  depends_on = [package.core_base, package.core_logging]
  namespace  = "monitoring"
  values_files = [
    "values/monitoring.yaml",
  ]
}
