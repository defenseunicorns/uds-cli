#!/bin/bash

OWNER="defenseunicorns"
REPO="uds-runtime"

BASE_PATH="./src/cmd/bin"
VERSION_FILE="${BASE_PATH}/version.txt"

# Get the latest release version from GitHub API
LATEST_VERSION=$(curl -s "https://api.github.com/repos/$OWNER/$REPO/releases/latest" | jq -r .tag_name)

# Check if the version file exists and read the current version
if [[ -f "$VERSION_FILE" ]]; then
    CURRENT_VERSION=$(cat "$VERSION_FILE")
else
    CURRENT_VERSION=""
fi

# If the current version is different from the latest, update binaries
if [[ "$LATEST_VERSION" != "$CURRENT_VERSION" ]]; then
    echo "Updating UDS Runtime binaries to version $LATEST_VERSION"

    # List of binaries and their paths
    BINARIES=("uds-runtime-darwin-amd64" "uds-runtime-darwin-arm64" "uds-runtime-linux-amd64" "uds-runtime-linux-arm64")

    # Download each binary
    for BINARY in "${BINARIES[@]}"; do
        echo "Downloading $BINARY"
        curl -L "https://github.com/$OWNER/$REPO/releases/download/${LATEST_VERSION}/${BINARY}" -o "${BASE_PATH}/${BINARY}"

        # Make the binary executable
        chmod +x "${BASE_PATH}/${BINARY}"
    done

    # Update the version file
    echo $LATEST_VERSION > "$VERSION_FILE"
    echo "Updated version file to $LATEST_VERSION"

else
    echo "Binaries are already up to date with version $LATEST_VERSION"
fi
