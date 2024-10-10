// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"embed"

	ui "github.com/defenseunicorns/uds-runtime/pkg/api"
)

//go:embed assets/ui/build/*
var assets embed.FS

//go:embed assets/certs/cert.pem
var localCert []byte

//go:embed assets/certs/key.pem
var localKey []byte

func startUI() error {
	r, incluster, err := ui.Setup(&assets)
	if err != nil {
		return err
	}
	ui.Serve(r, localCert, localKey, incluster)

	return nil
}
