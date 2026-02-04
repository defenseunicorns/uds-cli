// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"
)

// Remove removes a UDS bundle from a Kubernetes cluster.
func Remove(ociRef string) error {
	fmt.Printf("Removing bundle from Kubernetes cluster: %s\n", ociRef)
	// TODO: Implement bundle remove logic
	return nil
}
