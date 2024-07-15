#!/usr/bin/env sh

# Create the CLI docs for UDS CLI
# set UDS_NO_PROGRESS to empty string to override uds run default behavior
UDS_NO_PROGRESS="" go run main.go internal gen-cli-docs
