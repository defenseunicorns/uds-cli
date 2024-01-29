#!/usr/bin/env sh

# script to push the UDS bundle and Zarf artifacts that we use for testing GHCR
# ensure you have the proper creds and are logged in to GHCR for defenseunicorns

set -e

# used when pushing to GHCR using a local version of UDS; useful for testing changes to OCI
#alias uds=<path>/build/uds-mac-apple

# create the nginx and podinfo Zarf packages
cd ./../src/test/packages/nginx
zarf package create -o oci://ghcr.io/defenseunicorns/uds-cli --confirm -a amd64
zarf package create -o oci://ghcr.io/defenseunicorns/uds-cli --confirm -a arm64

cd ../podinfo
zarf package create -o oci://ghcr.io/defenseunicorns/uds-cli --confirm -a amd64
zarf package create -o oci://ghcr.io/defenseunicorns/uds-cli --confirm -a arm64

# create ghcr-test bundle
cd ../../bundles/06-ghcr
uds create . -o ghcr.io/defenseunicorns/packages/uds-cli/test/create-remote --confirm -a amd64
uds create . -o ghcr.io/defenseunicorns/packages/uds-cli/test/create-remote --confirm -a arm64

uds create . -o ghcr.io/defenseunicorns/packages/uds-cli/test/publish --confirm -a amd64
uds create . -o ghcr.io/defenseunicorns/packages/uds-cli/test/publish --confirm -a arm64

uds create . -o ghcr.io/defenseunicorns/packages/uds/bundles --confirm -a amd64
uds create . -o ghcr.io/defenseunicorns/packages/uds/bundles --confirm -a arm64

uds create . -o ghcr.io/defenseunicorns/packages/delivery --confirm -a amd64
uds create . -o ghcr.io/defenseunicorns/packages/delivery --confirm -a arm64

# change name of bundle for testing purposes
sed -i '' -e 's/ghcr-test/ghcr-delivery-test/g' uds-bundle.yaml
uds create . -o ghcr.io/defenseunicorns/packages/delivery --confirm -a amd64
uds create . -o ghcr.io/defenseunicorns/packages/delivery --confirm -a arm64
sed -i '' -e 's/ghcr-delivery-test/ghcr-test/g' uds-bundle.yaml

printf "\nSuccessfully pushed all test artifacts to GHCR."
