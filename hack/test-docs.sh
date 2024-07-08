#!/usr/bin/env sh

check_git_status() {
    if [ -z "$(git status -s "$1")" ]; then
        echo "Success!"
    else
        echo "Schema changes found, please regenerate $1"
        git status "$1"
        exit 1
    fi
}

check_git_status docs/

exit 0
