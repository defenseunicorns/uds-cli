// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package consumer_test

import (
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/legacy/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/bundler"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/bundler/fetcher"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/bundler/pusher"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/cache"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/config"
	enginek8s "github.com/defenseunicorns/uds-cli/pkg/legacy/engine/k8s"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/engine/pepr"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/engine/stream"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/interactive"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/message"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/sources"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/style"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types/chartvariable"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/types/valuesources"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/utils"
	"github.com/defenseunicorns/uds-cli/pkg/legacy/utils/boci"
)

var (
	_ = bundle.New
	_ = bundler.NewBundler
	_ fetcher.Fetcher
	_ = pusher.NewPkgPusher
	_ = cache.Exists
	_ = config.GetArch
	_ = enginek8s.NewClient
	_ pepr.StreamKind
	_ stream.Reader
	_ = interactive.PromptSigPassword
	_ message.LogLevel
	_ sources.PackageSource
	_ = style.RenderFmt
	_ types.BundleConfig
	_ chartvariable.Type
	_ valuesources.Source
	_ = utils.IsValidTarballPath
	_ = boci.EnsureOCIPrefix
)

func TestMutableConfigurationGlobals(t *testing.T) {
	originalVersion := config.CLIVersion
	originalOptions := config.CommonOptions
	t.Cleanup(func() {
		config.CLIVersion = originalVersion
		config.CommonOptions = originalOptions
	})

	config.CLIVersion = "consumer version"
	config.CommonOptions.Confirm = true

	if config.CLIVersion != "consumer version" || !config.CommonOptions.Confirm {
		t.Fatal("Legacy mutable configuration API is unavailable")
	}
}
