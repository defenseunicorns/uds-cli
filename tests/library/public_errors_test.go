// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library

package bundle_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	bundle "github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/stretchr/testify/require"
)

func TestCreateErrorUsesPublicContract(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.hcl")
	_, err := bundle.Create(t.Context(), missing, bundle.CreateOptions{
		Config:  newTestConfig(),
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
	})
	require.ErrorIs(t, err, bundle.ErrCreateBundle)

	var pathErr *fs.PathError
	require.ErrorAs(t, err, &pathErr)
	require.Equal(t, missing, pathErr.Path)
}

func TestPushErrorUsesPublicContract(t *testing.T) {
	t.Parallel()

	artifactPath := filepath.Join(t.TempDir(), "invalid.tar.zst")
	require.NoError(t, os.WriteFile(artifactPath, []byte("not an archive"), 0o600))

	_, err := bundle.Push(t.Context(), artifactPath, "example.com/test/bundle:v1", bundle.PushOptions{Config: newTestConfig()})
	require.ErrorIs(t, err, bundle.ErrPushBundle)
}

func TestSourceValidationUsesPublicContracts(t *testing.T) {
	t.Parallel()

	_, err := bundle.Deploy(t.Context(), nil, bundle.DeployOptions{Config: newTestConfig()})
	require.ErrorIs(t, err, bundle.ErrSourceRequired)

	_, err = bundle.Remove(t.Context(), nil, bundle.RemoveOptions{Config: newTestConfig()})
	require.ErrorIs(t, err, bundle.ErrSourceRequired)

	_, err = bundle.Reconfigure(t.Context(), "", "defaults.uds.hcl", bundle.ReconfigureOptions{
		Config:                    newTestConfig(),
		Suffix:                    "-test",
		Signing:                   bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
		SkipSignatureVerification: true,
	})
	require.ErrorIs(t, err, bundle.ErrSourceRequired)
}

func TestOCIReferenceErrorsUsePublicContracts(t *testing.T) {
	t.Parallel()

	config := newTestConfig()
	malformed := "example.com//repo:tag"
	checks := []struct {
		name      string
		operation error
		run       func() error
	}{
		{name: "pull", operation: bundle.ErrPullBundle, run: func() error {
			_, err := bundle.Pull(t.Context(), malformed, t.TempDir(), bundle.PullOptions{Config: config, SkipSignatureVerification: true})
			return err
		}},
		{name: "push", operation: bundle.ErrPushBundle, run: func() error {
			_, err := bundle.Push(t.Context(), "bundle.tar.zst", malformed, bundle.PushOptions{Config: config})
			return err
		}},
		{name: "inspect", operation: bundle.ErrInspectBundle, run: func() error {
			_, err := bundle.Inspect(t.Context(), bundle.InspectOptions{Source: malformed, Config: config})
			return err
		}},
		{name: "reconfigure", operation: bundle.ErrReconfigureBundle, run: func() error {
			_, err := bundle.Reconfigure(t.Context(), malformed, "defaults.uds.hcl", bundle.ReconfigureOptions{
				Config:                    config,
				Suffix:                    "-test",
				Signing:                   bundle.SigningOptions{Mode: bundle.SigningModeUnsigned},
				SkipSignatureVerification: true,
			})
			return err
		}},
		{name: "sign", operation: bundle.ErrSignBundle, run: func() error {
			return bundle.Sign(t.Context(), bundle.SignOptions{Source: malformed, Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: "unused"}})
		}},
		{name: "verify", operation: bundle.ErrVerifyBundle, run: func() error {
			return bundle.Verify(t.Context(), bundle.VerifyOptions{Source: malformed, Policy: bundle.VerificationPolicy{PublicKey: "unused"}})
		}},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := check.run()
			require.ErrorIs(t, err, check.operation)
			require.ErrorIs(t, err, bundle.ErrInvalidOCIReference)
		})
	}
}

func TestPullRejectsConflictingVerificationPolicy(t *testing.T) {
	t.Parallel()

	opts := bundle.PullOptions{
		Config: newTestConfig(),
		Verification: bundle.VerificationPolicy{
			PublicKey: "key.pem",
			Keyless:   &bundle.KeylessVerification{},
		},
	}
	_, err := bundle.Pull(t.Context(), "", "", opts)
	require.ErrorIs(t, err, bundle.ErrInvalidVerificationPolicy)
}

func TestPullAndPushRequiredInputsUsePublicContracts(t *testing.T) {
	t.Parallel()

	config := newTestConfig()
	validRef := "example.com/test/bundle:v1"

	_, err := bundle.Pull(t.Context(), "", t.TempDir(), bundle.PullOptions{Config: config, SkipSignatureVerification: true})
	require.ErrorIs(t, err, bundle.ErrSourceRequired)

	_, err = bundle.Inspect(t.Context(), bundle.InspectOptions{Config: config})
	require.ErrorIs(t, err, bundle.ErrSourceRequired)

	_, err = bundle.Pull(t.Context(), validRef, "", bundle.PullOptions{Config: config, SkipSignatureVerification: true})
	require.ErrorIs(t, err, bundle.ErrTargetDirRequired)

	_, err = bundle.Push(t.Context(), "", validRef, bundle.PushOptions{Config: config})
	require.ErrorIs(t, err, bundle.ErrSourceRequired)
}

func TestSigningErrorsUsePublicContracts(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.tar.zst")
	err := bundle.Sign(t.Context(), bundle.SignOptions{
		Source:  missing,
		Signing: bundle.SigningOptions{Mode: bundle.SigningModeKey, Key: "unused"},
	})
	require.ErrorIs(t, err, bundle.ErrSignBundle)
	require.ErrorIs(t, err, os.ErrNotExist)

	err = bundle.Verify(t.Context(), bundle.VerifyOptions{
		Source: missing,
		Policy: bundle.VerificationPolicy{PublicKey: "unused"},
	})
	require.ErrorIs(t, err, bundle.ErrVerifyBundle)
	require.ErrorIs(t, err, os.ErrNotExist)
}
