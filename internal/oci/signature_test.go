// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"bytes"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	oraci "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry"
)

func TestPublishBundleSignature_IsIdempotent(t *testing.T) {
	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	subject := pushSignatureSubject(t, store)
	evidence := []byte("signature evidence")
	require.NoError(t, PublishBundleSignature(t.Context(), store, subject, evidence, false))
	require.NoError(t, PublishBundleSignature(t.Context(), store, subject, evidence, false))

	refs, err := registry.Referrers(t.Context(), store, subject, MediaTypeBundleSignature)
	require.NoError(t, err)
	require.Len(t, refs, 1)
}

func TestPublishBundleSignature_OverwriteReplacesEvidence(t *testing.T) {
	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	subject := pushSignatureSubject(t, store)
	require.NoError(t, PublishBundleSignature(t.Context(), store, subject, []byte("old evidence"), false))
	require.NoError(t, PublishBundleSignature(t.Context(), store, subject, []byte("new evidence"), true))

	refs, err := registry.Referrers(t.Context(), store, subject, MediaTypeBundleSignature)
	require.NoError(t, err)
	require.Len(t, refs, 1)
	evidence, err := FetchBundleSignature(t.Context(), store, subject)
	require.NoError(t, err)
	require.Equal(t, []byte("new evidence"), evidence)
}

func TestPublishBundleSignature_RejectsDuplicateExistingEvidence(t *testing.T) {
	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	subject := pushSignatureSubject(t, store)
	pushSignatureEvidence(t, store, subject, []byte("first evidence"))
	pushSignatureEvidence(t, store, subject, []byte("second evidence"))

	err = PublishBundleSignature(t.Context(), store, subject, []byte("first evidence"), false)
	require.ErrorContains(t, err, "bundle signature evidence already exists")
}

func TestFetchBundleSignature_MissingEvidence(t *testing.T) {
	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	_, err = FetchBundleSignature(t.Context(), store, pushSignatureSubject(t, store))
	require.ErrorIs(t, err, ErrBundleSignatureNotFound)
}

func TestFetchBundleSignature_RejectsDuplicateEvidence(t *testing.T) {
	store, err := oraci.New(t.TempDir())
	require.NoError(t, err)

	subject := pushSignatureSubject(t, store)
	pushSignatureEvidence(t, store, subject, []byte("first"))
	pushSignatureEvidence(t, store, subject, []byte("second"))

	_, err = FetchBundleSignature(t.Context(), store, subject)
	require.ErrorContains(t, err, "expected exactly one bundle signature artifact, found 2")
	require.NotErrorIs(t, err, ErrBundleSignatureNotFound)
}

func pushSignatureSubject(t *testing.T, store *oraci.Store) ocispec.Descriptor {
	t.Helper()
	subjectData := []byte(`{"schemaVersion":2,"manifests":[]}`)
	subject := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, subjectData)
	require.NoError(t, store.Push(t.Context(), subject, bytes.NewReader(subjectData)))
	return subject
}

func pushSignatureEvidence(t *testing.T, store *oraci.Store, subject ocispec.Descriptor, evidence []byte) {
	t.Helper()
	layer := content.NewDescriptorFromBytes(MediaTypeBundleSignature, evidence)
	require.NoError(t, store.Push(t.Context(), layer, bytes.NewReader(evidence)))
	_, err := oras.PackManifest(t.Context(), store, oras.PackManifestVersion1_1, MediaTypeBundleSignature, oras.PackManifestOptions{
		Subject: &subject,
		Layers:  []ocispec.Descriptor{layer},
	})
	require.NoError(t, err)
}
