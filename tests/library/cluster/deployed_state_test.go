// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build library && cluster_integration && !owned_cluster

package cluster_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/state"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// decodeDeployedPackage reads the plain or gzip-compressed state stored by Zarf.
func decodeDeployedPackage(secret *corev1.Secret) (*state.DeployedPackage, error) {
	data := secret.Data["data"]
	switch format := secret.Annotations["zarf.dev/compression-format"]; format {
	case "":
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("open deployed package gzip state: %w", err)
		}
		defer reader.Close()
		data, err = io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("read deployed package gzip state: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported deployed package compression format %q", format)
	}
	var deployed state.DeployedPackage
	if err := json.Unmarshal(data, &deployed); err != nil {
		return nil, fmt.Errorf("decode deployed package state: %w", err)
	}
	return &deployed, nil
}

func TestDecodeDeployedPackage(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"name":"test-package","digest":"sha256:test-digest"}`)
	compress := func(data []byte) []byte {
		var buffer bytes.Buffer
		writer := gzip.NewWriter(&buffer)
		_, err := writer.Write(data)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		return buffer.Bytes()
	}
	compressed := compress(payload)
	badChecksum := bytes.Clone(compressed)
	badChecksum[len(badChecksum)-8] ^= 0xff

	for _, tt := range []struct {
		name    string
		labels  map[string]string
		data    []byte
		wantErr bool
	}{
		{name: "plain JSON", data: payload},
		{name: "empty compression annotation", labels: map[string]string{"zarf.dev/compression-format": ""}, data: payload},
		{name: "gzip JSON", labels: map[string]string{"zarf.dev/compression-format": "gzip"}, data: compressed},
		{name: "unsupported compression", labels: map[string]string{"zarf.dev/compression-format": "zstd"}, data: payload, wantErr: true},
		{name: "invalid gzip header", labels: map[string]string{"zarf.dev/compression-format": "gzip"}, data: payload, wantErr: true},
		{name: "truncated gzip", labels: map[string]string{"zarf.dev/compression-format": "gzip"}, data: compressed[:len(compressed)-4], wantErr: true},
		{name: "corrupt gzip checksum", labels: map[string]string{"zarf.dev/compression-format": "gzip"}, data: badChecksum, wantErr: true},
		{name: "invalid plain JSON", data: []byte(`{`), wantErr: true},
		{name: "invalid gzip JSON", labels: map[string]string{"zarf.dev/compression-format": "gzip"}, data: compress([]byte(`{`)), wantErr: true},
		{name: "missing data", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Annotations: tt.labels},
				Data:       map[string][]byte{"data": tt.data},
			}
			deployed, err := decodeDeployedPackage(secret)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, deployed)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, deployed)
			assert.Equal(t, "test-package", deployed.Name)
			assert.Equal(t, "sha256:test-digest", deployed.Digest)
		})
	}
}
