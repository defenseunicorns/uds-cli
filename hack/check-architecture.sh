#!/usr/bin/env bash

# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

set -euo pipefail

if git grep -n "github.com/defenseunicorns/uds-cli/internal/next" -- internal/legacy pkg/legacy; then
  echo "Legacy packages must not import Next packages" >&2
  exit 1
fi
