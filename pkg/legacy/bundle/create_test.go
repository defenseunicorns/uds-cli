// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/stretchr/testify/require"
)

func TestGetSigCreatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		confirm  bool
		want     []byte
	}{
		{name: "password flag is used", password: "hunter2", want: []byte("hunter2")},
		{name: "password flag wins over confirm", password: "hunter2", confirm: true, want: []byte("hunter2")},
		{name: "confirm with no password uses an empty password", confirm: true, want: []byte{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := config.CommonOptions.Confirm
			t.Cleanup(func() { config.CommonOptions.Confirm = prev })
			config.CommonOptions.Confirm = tt.confirm

			b := &Bundle{cfg: &types.BundleConfig{CreateOpts: types.BundleCreateOptions{SigningKeyPassword: tt.password}}}
			got, err := b.getSigCreatePassword(false)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
