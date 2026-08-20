# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

variables = {
  service_enabled = false
  replica_count   = 2
  log_level       = "info"
  annotations = {
    "app.kubernetes.io/managed-by" = "uds"
    "team"                          = "platform"
  }
  tolerations = [
    { key = "node.kubernetes.io/not-ready", operator = "Exists", effect = "NoExecute" },
  ]
}
