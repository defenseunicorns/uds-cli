// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"embed"
	"log/slog"

	ui "github.com/defenseunicorns/uds-runtime/pkg/api"
)

//go:embed ui/build/*
var uiBuild embed.FS

//go:embed certs/cert.pem
var localCert []byte

//go:embed certs/key.pem
var localKey []byte

func startUI() error {
	r, incluster, err := ui.Setup(&uiBuild)
	if err != nil {
		slog.Error("Failed to setup UI server", err)
		return err
	}
	err = ui.Serve(r, localCert, localKey, incluster)
	if err != nil {
		slog.Error("failed to serve UI", "error", err)
		return err
	}
	return nil
}
