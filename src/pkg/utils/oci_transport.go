// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package utils

import (
	"context"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/zarf-dev/zarf/src/pkg/ocischeme"
	"oras.land/oras-go/v2/registry"
)

// NegotiatePlainHTTP returns whether an OCI registry reference should use plain HTTP.
// UDS's --insecure option permits plain HTTP, but transport still needs to be
// negotiated per registry host instead of forced globally.
func NegotiatePlainHTTP(ctx context.Context, ref registry.Reference) (bool, error) {
	if !config.CommonOptions.Insecure {
		return false, nil
	}
	return ocischeme.From(ctx).UsePlainHTTP(ctx, ref.Registry, ocischeme.ProbeOptions{InsecureSkipTLSVerify: true})
}
