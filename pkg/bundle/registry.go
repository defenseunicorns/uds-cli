// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"crypto/tls"
	"fmt"
	"net/http"

	orasregistry "oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// newRemoteRepository creates an ORAS remote repository configured with TLS
// settings and credentials loaded from the Docker credential store.
func newRemoteRepository(ref string, opts ConfigOptions) (*orasregistry.Repository, error) {
	repo, err := orasregistry.NewRepository(ref)
	if err != nil {
		return nil, err
	}
	repo.PlainHTTP = opts.PlainHTTP

	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t = &http.Transport{}
	}
	transport := t.Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: opts.SkipTLSVerify} //nolint:gosec // user-controlled via --skip-tls-verify

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
