#!/usr/bin/env bash
# Copyright 2024 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial


# Parse command-line arguments for debug flag
DEBUG=false
while [[ "$#" -gt 0 ]]; do
  case $1 in
    --debug) DEBUG=true ;;
  esac
  shift
done

# Function to print debug messages
debug() {
  if [ "${DEBUG}" = true ]; then
    echo "$1"
  fi
}

# Determine OS type (linux or macos)
OS=$(uname | tr '[:upper:]' '[:lower:]')
if [[ "${OS}" == "darwin" ]]; then
  OS="Darwin"
elif [[ "${OS}" == "linux" ]]; then
  OS="Linux"
else
  debug "Unsupported OS: ${OS}"
  exit 1
fi

# Determine architecture (amd64 or arm64)
ARCH=$(uname -m)
if [[ "${ARCH}" == "x86_64" ]]; then
  ARCH="amd64"
elif [[ "${ARCH}" == "arm64" || "${ARCH}" == "aarch64" ]]; then
  ARCH="arm64"
else
  debug "Unsupported architecture: ${ARCH}"
  exit 1
fi

# Construct the download URL
URL="https://github.com/defenseunicorns/uds-cli"
RELEASE_URL="${URL}/releases/latest"


# Define the version to install
VERSION=$(curl --tlsv1.2 --proto "=https" --fail --show-error -Ls -o /dev/null -w %{url_effective} ${RELEASE_URL} | grep -oE "[^/]+$" )
debug "Version set to ${VERSION}"

# Set binary name
BINARY_NAME="uds"

# Set full path to binary download
FULL_PATH="releases/download/${VERSION}/uds-cli_${VERSION}_${OS}_${ARCH}"

UDS_TMP_DIR="$(mktemp -d -t uds-binary-XXXXXXXX)"
UDS_TMP_FILE="${UDS_TMP_DIR}/${BINARY_NAME}"

# Download the binary
debug "Downloading uds-cli for ${OS}-${ARCH}..."
curl --tlsv1.2 --proto "=https" --fail --show-error -L -o "${UDS_TMP_FILE}" "${URL}/${FULL_PATH}"
if [[ $? -ne 0 ]]; then
  debug "Failed to download uds-cli from ${URL}/${FULL_PATH}"
  exit 1
fi

# Make the binary executable
chmod +x "${UDS_TMP_FILE}"

# Define the installation directory and binary name
INSTALL_DIR="/usr/local/bin"

# Move the binary to the installation directory
debug "Installing uds-cli to ${INSTALL_DIR}..."
sudo mv "${UDS_TMP_FILE}" "${INSTALL_DIR}/"
if [[ $? -ne 0 ]]; then
  echo "Failed to move uds-cli to ${INSTALL_DIR}"
  exit 1
fi

# Verify the installation
debug "Verifying uds-cli installation..."
"${INSTALL_DIR}/${BINARY_NAME}" version > /dev/null 2>&1
if [[ $? -eq 0 ]]; then
  debug "uds-cli ${VERSION} installed successfully!"
else
  debug "Installation failed."
  exit 1
fi
