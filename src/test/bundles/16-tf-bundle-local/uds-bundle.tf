terraform {
  required_providers {
    uds = {
      source = "defenseunicorns/uds"
    }
  }
}

provider "uds" {}

resource "uds_bundle_metadata" "tf-demo-bundle-local" {
  version = "0.0.1"
  kind = "UDSBundle"
  description = "A demo bundle for the podinfo and nginx packages"
  architecture = "arm64"
}

resource "uds_package" "podinfo" {
  path = "../../packages/podinfo/zarf-package-podinfo-arm64-0.0.1.tar.zst"
  ref = "0.0.1"
  architecture = "arm64"
}

resource "uds_package" "nginx" {
  path = "../../packages/nginx/zarf-package-nginx-arm64-0.0.1.tar.zst"
  ref = "0.0.1"
  architecture = "arm64"
}
