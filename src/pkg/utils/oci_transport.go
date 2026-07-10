// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package utils

import (
	"context"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/zarf-dev/zarf/src/pkg/ocischeme"
	"oras.land/oras-go/v2/registry"
)

// NegotiatePlainHTTPForOCIRef returns whether an OCI registry reference should use plain HTTP.
// UDS's --insecure option permits plain HTTP, but transport still needs to be
// negotiated per registry host instead of forced globally.
func NegotiatePlainHTTPForOCIRef(ctx context.Context, ref string, insecure bool) (bool, error) {
	parsed, err := registry.ParseReference(strings.TrimPrefix(ref, helpers.OCIURLPrefix))
	if err != nil {
		return false, err
	}
	return NegotiatePlainHTTPForRegistry(ctx, parsed.Registry, insecure)
}

// NegotiatePlainHTTPForRegistry returns whether a registry host should use plain HTTP.
func NegotiatePlainHTTPForRegistry(ctx context.Context, registryAddress string, insecure bool) (bool, error) {
	if !insecure {
		return false, nil
	}
	address := strings.TrimPrefix(registryAddress, helpers.OCIURLPrefix)
	if strings.HasPrefix(address, "http://") {
		return true, nil
	}
	if strings.HasPrefix(address, "https://") {
		return false, nil
	}
	host := strings.SplitN(address, "/", 2)[0]
	return ocischeme.From(ctx).UsePlainHTTP(ctx, host, ocischeme.ProbeOptions{InsecureSkipTLSVerify: true})
}
