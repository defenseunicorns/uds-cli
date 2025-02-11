# Copyright 2025 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

terraform {
  required_providers {
    uds = {
      source = "defenseunicorns/uds"
    }
  }
}

provider "uds" {}

resource "uds_bundle_metadata" "tf-demo-bundle-remote" {
  version = "0.0.1"
  kind = "UDSBundle"
  description = "A demo bundle for the podinfo and nginx packages"
  architecture = "arm64"
}

resource "uds_package" "podinfo" {
  repository = "ghcr.io/defenseunicorns/uds-cli/podinfo"
  ref = "0.0.1"
  architecture = "arm64"
}

resource "uds_package" "nginx" {
  repository = "ghcr.io/defenseunicorns/uds-cli/nginx"
  ref = "0.0.1"
  architecture = "arm64"
}
