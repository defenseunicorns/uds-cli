// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import "log/slog"

// Remove removes a UDS bundle from a Kubernetes cluster.
func Remove(ociRef string) error {
	slog.Info("removing bundle from cluster", "ref", ociRef)
	// TODO: Implement bundle remove logic
	return nil
}
