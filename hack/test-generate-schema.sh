#!/usr/bin/env sh
# Copyright 2024 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial


check_git_status() {
    if [ -z "$(git status -s "$1")" ]; then
        echo "Success!"
    else
        echo "Schema changes found, please regenerate $1"
        git status "$1"
        exit 1
    fi
}

check_git_status uds.schema.json
check_git_status zarf.schema.json
check_git_status tasks.schema.json

exit 0
