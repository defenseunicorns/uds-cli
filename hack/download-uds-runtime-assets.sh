#!/bin/bash

OWNER="defenseunicorns"
REPO="uds-runtime"
BASE_PATH="./src/cmd/"
CERTS_PATH="./src/cmd/ui/certs"
ARCHIVE_NAME="uds-runtime-ui.tar.gz"
CURRENT_VERSION="v0.6.0"

# Get the latest release version from GitHub API
LATEST_VERSION=$(curl -s "https://api.github.com/repos/$OWNER/$REPO/releases/latest" | jq -r .tag_name)

# Create the base path directory if it doesn't exist
mkdir -p "$BASE_PATH"
mkdir -p "$CERTS_PATH"

# Download the latest release archive
download_release() {
    echo "Downloading $ARCHIVE_NAME for version $LATEST_VERSION"
    curl -L "https://github.com/$OWNER/$REPO/releases/download/${LATEST_VERSION}/${ARCHIVE_NAME}" -o "${BASE_PATH}/${ARCHIVE_NAME}"
}

# Extract the archive into the base path
extract_release() {
    echo "Extracting $ARCHIVE_NAME"
    tar -xzf "${BASE_PATH}/${ARCHIVE_NAME}" -C "$BASE_PATH"
}

# Remove old files in the base path
clean_old_files() {
    echo "Cleaning up old files"
    rm -rf "${BASE_PATH:?}/*"
}

# Download raw certs files from the repository's main branch
download_certs() {
    echo "Downloading certificates from hack/certs"
    FILES=("cert.pem" "key.pem")
    for file in "${FILES[@]}"; do
        echo "Downloading $file"
        curl -L "https://raw.githubusercontent.com/$OWNER/$REPO/main/hack/certs/$file" -o "${CERTS_PATH}/$file"
    done
}

# Check if the current version is different from the latest or the archive doesn't exist
if [[ "$LATEST_VERSION" != "$CURRENT_VERSION" ]] || [[ ! -f "${BASE_PATH}/${ARCHIVE_NAME}" ]]; then
    echo "Updating UDS Runtime UI to version $LATEST_VERSION"

    # Clean up old files before downloading the new release
    clean_old_files

    # Download and extract the latest release archive
    download_release
    extract_release

    # Update the current version
    CURRENT_VERSION="$LATEST_VERSION"
    echo "Updated to version $LATEST_VERSION"
else
    echo "UDS Runtime UI is up to date."
fi

# Download certs files
download_certs
