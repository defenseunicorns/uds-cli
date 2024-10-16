#!/bin/bash

OWNER="defenseunicorns"
REPO="uds-runtime"

BASE_PATH="./src/cmd/bin"
CURRENT_VERSION="v0.6.1"

# Get the latest release version from GitHub API
LATEST_VERSION=$(curl -s "https://api.github.com/repos/$OWNER/$REPO/releases/latest" | jq -r .tag_name)

# List of binaries and their paths
BINARIES=("uds-runtime-darwin-amd64" "uds-runtime-darwin-arm64" "uds-runtime-linux-amd64" "uds-runtime-linux-arm64")

# Create the base path directory if it doesn't exist
mkdir -p "$BASE_PATH"

# Update a specific binary
update_binary() {
    local binary=$1
    echo "Downloading $binary"
    curl -L "https://github.com/$OWNER/$REPO/releases/download/${LATEST_VERSION}/${binary}" -o "${BASE_PATH}/${binary}"

    # Make the binary executable
    chmod +x "${BASE_PATH}/${binary}"
}

# Check if a binary exists in the base path
binary_exists() {
    local binary=$1
    [[ -f "${BASE_PATH}/${binary}" ]]
}

# Remove all binaries except the specified one
remove_other_binaries() {
    local keep_binary=$1
    for binary in "${BINARIES[@]}"; do
        if [[ "$binary" != "$keep_binary" ]]; then
            rm -f "${BASE_PATH}/${binary}"
        fi
    done
}

# Ensure a binary name is passed in
if [ -z "$1" ]; then
    echo "Error: A binary name must be provided."
    echo "Usage: $0 <binary-name>"
    exit 1
fi

# Remove all other binaries
remove_other_binaries "$1"

# If the current version is different from the latest or a binary name is passed in and the binary doesn't exist
# or no binary name is passed in and none of the binaries exist then update the binary/binaries
if [[ "$LATEST_VERSION" != "$CURRENT_VERSION" ]] || ! binary_exists "$1"; then
    echo "Updating UDS Runtime binaries to version $LATEST_VERSION"

    # Update the specified binary
    update_binary "$1"

    # Update the current version variable
    CURRENT_VERSION="$LATEST_VERSION"
    echo "Updated current version to $LATEST_VERSION"

else
    echo "Binaries are up to date."
fi
