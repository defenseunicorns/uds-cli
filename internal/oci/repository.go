// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/zarf-dev/zarf/src/pkg/ocischeme"
	"oras.land/oras-go/v2/registry"
	orasregistry "oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// NewRemoteRepository creates an ORAS remote repository configured with registry
// transport settings and credentials loaded from the Docker credential store.
func NewRemoteRepository(ctx context.Context, ref string, opts bundleinternal.ConfigOptions) (*orasregistry.Repository, error) {
	repo, err := orasregistry.NewRepository(ref)
	if err != nil {
		return nil, err
	}

	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t = &http.Transport{}
	}
	transport := t.Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: opts.SkipTLSVerify} //nolint:gosec // user-controlled via --skip-tls-verify
	plainHTTP, err := ResolvePlainHTTP(ctx, ref, opts, transport)
	if err != nil {
		return nil, err
	}
	repo.PlainHTTP = plainHTTP

	credStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err != nil {
		return nil, fmt.Errorf("loading docker credentials: %w", err)
	}

	repo.Client = &auth.Client{
		Client:     &http.Client{Transport: retry.NewTransport(transport)},
		Cache:      auth.DefaultCache,
		Credential: credentials.Credential(credStore),
	}

	return repo, nil
}

// ResolvePlainHTTP determines whether an OCI reference should use plain HTTP.
// Plain HTTP is only considered when the user explicitly enables it; HTTPS remains
// the default and is preferred whenever the registry supports it.
func ResolvePlainHTTP(ctx context.Context, ref string, opts bundleinternal.ConfigOptions, transport http.RoundTripper) (bool, error) {
	if !opts.PlainHTTP {
		return false, nil
	}

	parsed, err := registry.ParseReference(ref)
	if err != nil {
		return false, fmt.Errorf("parsing OCI reference %q: %w", ref, err)
	}

	plainHTTP, err := ocischeme.From(ctx).UsePlainHTTP(ctx, parsed.Registry, ocischeme.ProbeOptions{
		InsecureSkipTLSVerify: opts.SkipTLSVerify,
		Transport:             transport,
	})
	if err != nil {
		return false, fmt.Errorf("determining registry transport for %q: %w", ref, err)
	}
	return plainHTTP, nil
}
