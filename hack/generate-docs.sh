#!/usr/bin/env sh
# Copyright 2024 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial


# Create the CLI docs for UDS CLI
# set UDS_NO_PROGRESS to empty string to override uds run default behavior
UDS_NO_PROGRESS="" go run main.go internal gen-cli-docs
