#!/usr/bin/env sh

# This script runs as part of the nightly UDS Core smoke tests and validates the state of the UDS Core bundle after deployment

set -e

echo "Validating UDS Core state"

get_secret_data() {
  kubectl get secret uds-bundle-k3d-core-slim-dev -n uds -o json | \
    jq -r '.data.data' | \
    base64 -d
}

secret_data=$(get_secret_data) || {
  echo "Error: Failed to retrieve secret data"
  exit 1
}

echo "State data:"
echo "$secret_data" | jq '.' || {
  echo "Error: Failed to parse secret data as JSON"
  exit 1
}

all_statuses_success=$(echo "$secret_data" | \
  jq -r '
    if .status == "success" and (.packages | all(.status == "success")) then
      "true"
    else
      "false"
    end
  ') || {
  echo "Error: Failed to check statuses"
  exit 1
}

if [ "$all_statuses_success" != "true" ]; then
  echo "Error: Not all statuses are successful"
  echo "Issues:"
  echo "$secret_data" | jq -r '
    [
      if .status != "success" then "Top-level status is not success" else empty end,
      (.packages[] | select(.status != "success") | "Package \(.name) status is not success")
    ] | .[]
  ' || echo "Error: Failed to list issues"
  exit 1
else
  echo "All statuses are successful"
fi
