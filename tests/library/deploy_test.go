// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"context"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeployRejectsInvalidZarfSettingsBeforeHooks(t *testing.T) {
	tests := []struct {
		name        string
		zarfTimeout time.Duration
		zarfRetries int
	}{
		{name: "negative timeout", zarfTimeout: -time.Second},
		{name: "negative retries", zarfRetries: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			_, err := bundle.Deploy(t.Context(), deploySource(singlePkgBundle()), bundle.DeployOptions{
				Config:      newTestConfig(),
				ZarfTimeout: tt.zarfTimeout,
				ZarfRetries: tt.zarfRetries,
				BundleDeployHooks: bundle.BundleDeployHooks{PreDeploy: func(context.Context, *spec.UDSBundle, *bundle.DeployOptions) error {
					called = true
					return nil
				}},
			})

			require.ErrorIs(t, err, bundle.ErrInvalidDeployOptions)
			assert.False(t, called)
		})
	}
}
