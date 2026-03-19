// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package main is the entrypoint for the uds binary.
package main

import (
	"github.com/defenseunicorns/uds-cli/src/cmd"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"helm.sh/helm/v3/pkg/kube"
)

func main() {
	// Set the Helm field manager name to match Zarf's so that resources deployed via UDS bundles
	// and resources deployed directly via Zarf are interchangeable without requiring --force-conflicts.
	kube.ManagedFieldsManager = cluster.FieldManagerName
	cmd.Execute()
}
