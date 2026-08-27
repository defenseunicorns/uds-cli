#!/usr/bin/env sh
# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

set -eu

module_pattern='github\.com/defenseunicorns/uds-cli'
patterns_file=$(mktemp)
trap 'rm -f "$patterns_file"' EXIT

for path in internal/legacy pkg/legacy; do
	if [ ! -d "$path" ]; then
		echo "Architecture path does not exist: $path" >&2
		exit 1
	fi
done

for dir in internal/*; do
	[ -d "$dir" ] || continue
	name=${dir#internal/}
	case "$name" in
		legacy|mode)
			continue
			;;
	esac
	printf '%s/internal/%s(/|")\n' "$module_pattern" "$name" >>"$patterns_file"
done

for pkg in pkg/bundle pkg/iostreams; do
	[ -d "$pkg" ] || continue
	printf '%s/%s(/|")\n' "$module_pattern" "$pkg" >>"$patterns_file"
done

if matches=$(grep -R -n -E -f "$patterns_file" internal/legacy pkg/legacy); then
	printf '%s\n' "$matches"
	echo "Legacy implementation must not depend on canonical Next packages" >&2
	exit 1
else
	status=$?
	if [ "$status" -ne 1 ]; then
		exit "$status"
	fi
fi
