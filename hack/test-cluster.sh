#!/usr/bin/env bash
# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

# Provision infrastructure outside the supplied-cluster test suites.
set -euo pipefail
umask 077

if [[ $# != 2 || ( "$1" != setup && "$1" != cleanup ) ]]; then
  echo "usage: $0 setup|cleanup <isolated-directory>" >&2
  exit 1
fi

directory=$(cd "$2" && pwd)
name_file="$directory/cluster-name"
name="uds-tests-$(basename "$directory" | tr '[:upper:]' '[:lower:]')"

if [[ "$1" == cleanup ]]; then
  if [[ -f "$name_file" ]]; then
    [[ "$(cat "$name_file")" == "$name" ]] || { echo "cluster ownership mismatch" >&2; exit 1; }
    k3d cluster delete "$name"
    rm "$name_file"
  fi
  exit 0
fi

clusters=$(k3d cluster list --output json)
exists=$(jq -r --arg name "$name" 'any(.[]; .name == $name)' <<< "$clusters")
if [[ -e "$name_file" || "$exists" == true ]]; then
  echo "test cluster already exists: $name" >&2
  exit 1
fi
# Record ownership before creation so a failed setup can also be cleaned up.
printf '%s\n' "$name" > "$name_file"
cat > "$directory/k3d.yaml" <<'CONFIG'
apiVersion: k3d.io/v1alpha5
kind: Simple
kubeAPI:
  host: 127.0.0.1
  hostIP: 127.0.0.1
CONFIG
k3d cluster create "$name" --config "$directory/k3d.yaml" \
  --kubeconfig-update-default=false --kubeconfig-switch-context=false \
  --timeout 10m --wait
k3d kubeconfig get "$name" > "$directory/kubeconfig.yaml"

# Use the installed tool's matching init version, outside the repository so a
# local archive cannot override it. UDS bundle bootstrap is tested via the API.
(
  cd "$directory"
  export KUBECONFIG="$directory/kubeconfig.yaml"
  zarf_version=$(zarf version)
  if [[ ! "$zarf_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "unexpected installed Zarf version: $zarf_version" >&2
    exit 1
  fi
  zarf init "oci://ghcr.io/zarf-dev/packages/init:${zarf_version}" \
    --confirm --no-color
)
