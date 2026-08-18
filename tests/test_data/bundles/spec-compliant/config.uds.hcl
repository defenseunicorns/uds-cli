# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

# Deploy-time configuration for the UDS Core example bundle
#
# This file demonstrates how to provide deploy-time variables that are used
# to template values files. Variables use the {{ .vars.* }} syntax in YAML files.
#
# Reference: docs/specs/1_Better_Bundles.md

# CLI options for bundle deployment
options {
  log_level       = "info"
  architecture    = "amd64"
  plain_http      = false
  skip_tls_verify = false
  tmp_dir         = "/tmp/uds-tmp"
  concurrency     = 10
}

# Variables used to template out values files AND passed to any matching Zarf variables
# These variables are referenced in values/*.yaml files using {{ .vars.<key> }} syntax
variables = {
  # Shared domain for all packages
  domain = "uds.dev"

  # Logging package configuration
  logging = {
    vectorEnabled          = true
    vectorRole             = "collector"
    vectorAcknowledgements = "auto"
  }

  # Monitoring package configuration
  monitoring = {
    retentionDays         = "15d"
    prometheusStorageSize = "50Gi"
    grafanaAdminPassword  = "change-me-in-production"
    grafanaStorageSize    = "5Gi"
  }
}
