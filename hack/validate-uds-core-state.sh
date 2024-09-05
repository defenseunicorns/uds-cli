#!/usr/bin/env sh

# This script runs as part of the nightly UDS Core smoke tests and validates the state of the UDS Core bundle after deployment

secret_data=$(kubectl get secret uds-bundle-k3d-core-slim-dev -n uds -o json | \
  jq -r '.data.data' | \
  base64 -d)

all_statuses_success=$(echo "$secret_data" | \
  jq -r '
    .status == "success" and
    (.packages | all(.status == "success"))
  ')

if [[ "$all_statuses_success" != "true" ]]; then
  echo "Error: Not all statuses are successful"
  echo "Issues:"
  echo "$secret_data" | jq '
    [
      if .status != "success" then "Top-level status is not success" else empty end,
      (.packages[] | select(.status != "success") | "Package \(.name) status is not success")
    ] | .[]
  '
  exit 1
else
  echo "All statuses are successful"
fi
