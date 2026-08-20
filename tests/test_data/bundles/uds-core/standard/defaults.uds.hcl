# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

variables = {
  PEPR_WATCHER_MEMORY_REQUEST   = "256Mi"
  PEPR_ADMISSION_MEMORY_REQUEST = "256Mi"
  PEPR_WATCHER_CPU_REQUEST      = "200m"
  PEPR_ADMISSION_CPU_REQUEST    = "200m"

  LOKI_WRITE_REPLICAS   = "1"
  LOKI_READ_REPLICAS    = "1"
  LOKI_BACKEND_REPLICAS = "1"

  VELERO_BUCKET_PROVIDER_URL    = "http://minio.uds-dev-stack.svc.cluster.local:9000"
  VELERO_BUCKET                 = "uds"
  VELERO_BUCKET_REGION          = "uds-dev-stack"
  VELERO_BUCKET_KEY             = "uds"
  VELERO_BUCKET_KEY_SECRET      = "uds-secret"
  VELERO_BUCKET_CREDENTIAL_NAME = "velero-bucket-credentials"
  VELERO_BUCKET_CREDENTIAL_KEY  = "cloud"

  AUTHSERVICE_REPLICA_COUNT = 1

  KEYCLOAK_CUSTOM_TERMS_AND_CONDITIONS = ""
}
