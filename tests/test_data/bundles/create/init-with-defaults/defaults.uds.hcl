# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

# Default bundle configuration for testing.
# The integration test verifies that variables from defaults.uds.hcl are applied during create.

variables = {
  a = file("default-a.txt")
  b = 0
  c = {
    d = true
    e = false
  }
}
