// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

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
		slog.Error("Failed to serve UI", err)
		return err
	}
	return nil
}
