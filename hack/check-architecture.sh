#!/usr/bin/env sh
# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

set -eu

for path in internal/legacy pkg/legacy; do
	if [ ! -d "$path" ]; then
		echo "Architecture path does not exist: $path" >&2
		exit 1
	fi
	if matches=$(grep -R -n -E 'github\.com/defenseunicorns/uds-cli/internal/(artifact|bundle|cli|logger|oci|printer|testutil|version|zarf)(/|\")' "$path"); then
		printf '%s\n' "$matches"
		echo "Legacy implementation must not depend on the canonical implementation: $path" >&2
		exit 1
	else
		status=$?
		if [ "$status" -ne 1 ]; then
			exit "$status"
		fi
	fi
done
