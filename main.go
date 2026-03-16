// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package main is the entrypoint for the uds binary.
package main

import (
	"github.com/defenseunicorns/uds-cli/src/cmd"
)

func main() {
	// Decision point here for the uds-cli team, with ssa deploys what do you want your field manager to be named
	// You could name it "zarf" to be consistent with the name set by Zarf, this will allow users to deploy the same resources with packages and bundles interchangeably
	// Alternatively, you could name it uds, if you want to introduce friction if a user deploys a package previously owned by uds-cli with Zarf or vice versa
	// kube.ManagedFieldsManager = "uds|zarf" (there is a constant cluster.FieldManagerName if using zarf)
	cmd.Execute()
}
