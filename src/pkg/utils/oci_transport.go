// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package utils

import (
	"context"
	"net/url"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/zarf-dev/zarf/src/pkg/ocischeme"
	"oras.land/oras-go/v2/registry"
)

// NegotiatePlainHTTPForOCIRef returns whether an OCI registry reference should use plain HTTP.
// UDS's --insecure option permits plain HTTP, but transport still needs to be
// negotiated per registry host instead of forced globally.
func NegotiatePlainHTTPForOCIRef(ctx context.Context, ref string) (bool, error) {
	if !config.CommonOptions.Insecure {
		return false, nil
	}
	parsed, err := registry.ParseReference(strings.TrimPrefix(ref, helpers.OCIURLPrefix))
	if err != nil {
		return false, err
	}
	return NegotiatePlainHTTPForRegistry(ctx, parsed.Registry)
}

// NegotiatePlainHTTPForRegistry returns whether a registry host should use plain HTTP.
func NegotiatePlainHTTPForRegistry(ctx context.Context, registryAddress string) (bool, error) {
	if !config.CommonOptions.Insecure {
		return false, nil
	}
	host, err := registryHost(registryAddress)
	if err != nil {
		return false, err
	}
	return ocischeme.From(ctx).UsePlainHTTP(ctx, host, ocischeme.ProbeOptions{InsecureSkipTLSVerify: true})
}

func registryHost(registryAddress string) (string, error) {
	address := strings.TrimPrefix(registryAddress, helpers.OCIURLPrefix)
	if strings.Contains(address, "://") {
		parsed, err := url.Parse(address)
		if err != nil {
			return "", err
		}
		return parsed.Host, nil
	}
	return strings.SplitN(address, "/", 2)[0], nil
}
