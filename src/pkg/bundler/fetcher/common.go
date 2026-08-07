// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package fetcher

import (
	"context"

	"github.com/defenseunicorns/pkg/oci"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/zoci"
)

func NewZarfOCIRemote(ctx context.Context, url string, platform ocispec.Platform, mods ...oci.Modifier) (*zoci.Remote, error) {
	plainHTTP, err := utils.NegotiatePlainHTTPForOCIRef(ctx, url, config.CommonOptions.Insecure)
	if err != nil {
		return nil, err
	}
	modifiers := append([]oci.Modifier{
		oci.WithUserAgent("uds-cli/" + config.CLIVersion),
		oci.WithInsecureSkipVerify(config.CommonOptions.Insecure),
		oci.WithPlainHTTP(plainHTTP),
	}, mods...)
	return zoci.NewRemote(ctx, url, platform, modifiers...)
}

func packageManifestLayerDescriptor(sourceDesc ocispec.Descriptor) ocispec.Descriptor {
	return ocispec.Descriptor{
		MediaType: layout.ZarfLayerMediaTypeBlob,
		Digest:    sourceDesc.Digest,
		Size:      sourceDesc.Size,
	}
}
