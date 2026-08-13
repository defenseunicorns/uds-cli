// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package consumer_test preserves the Legacy public API used by zarf scan.
package consumer_test

import (
	"runtime"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/sources"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/zarf-dev/zarf/src/pkg/signing"
)

func TestZarfScanLegacyImportsCompile(_ *testing.T) {
	_ = zarfScanConsumerSurface
}

func zarfScanConsumerSurface(t *testing.T) {
	bundleConfig := types.BundleConfig{}
	bundleConfig.PullOpts.Source = "oci://example.invalid/bundle"
	bundleConfig.PullOpts.PublicKeyPath = "key.pem"
	bundleConfig.PullOpts.OutputDirectory = t.TempDir()
	bundleConfig.DeployOpts.Source = "bundle.tar.zst"
	config.CommonOptions.CachePath = t.TempDir()
	config.CLIArch = runtime.GOARCH

	client, err := bundle.New(&bundleConfig)
	if err != nil {
		t.Fatal(err)
	}
	_ = client.Pull
	_ = client.ClearPaths
	_ = client.GetMetadata
	_ = client.PreDeployValidation
	_ = client.GetPackages
	_ = config.BundlePrefix
	_ = types.UDSMetadata{}

	packageDefinition := types.Package{}
	verifyOptions := signing.VerifyBlobOptions{}
	source, err := sources.NewFromLocation(
		bundleConfig,
		packageDefinition,
		t.TempDir(),
		&verifyOptions,
		false,
		"",
		make(bundle.NamespaceOverrideMap),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = source.LoadPackage
}
