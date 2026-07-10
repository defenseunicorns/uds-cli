// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package utils

import (
	"context"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/zarf-dev/zarf/src/pkg/ocischeme"
	"oras.land/oras-go/v2/registry"
)

// NegotiatePlainHTTP returns the concrete transport scheme for an OCI registry.
// UDS's --insecure option permits plain HTTP, but Zarf v0.81 negotiates the
// actual scheme per registry host instead of forcing plain HTTP globally.
func NegotiatePlainHTTP(ctx context.Context, ref string) (bool, error) {
	if !config.CommonOptions.Insecure {
		return false, nil
	}
	parsed, err := registry.ParseReference(strings.TrimPrefix(ref, helpers.OCIURLPrefix))
	if err != nil {
		return false, err
	}
	return ocischeme.From(ctx).UsePlainHTTP(ctx, parsed.Registry, ocischeme.ProbeOptions{InsecureSkipTLSVerify: true})
}
