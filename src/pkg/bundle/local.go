// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"fmt"
	"os"
	"path/filepath"
)

// CheckYAMLSourcePath checks if the provided YAML source path is valid
func CheckYAMLSourcePath(source string) (string, error) {
	// Check if the file exists
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return "", fmt.Errorf("file does not exist: %s", source)
	}

	// Check if the file has a .yaml extension
	if filepath.Ext(source) != ".yaml" {
		return "", fmt.Errorf("file is not a YAML file: %s", source)
	}

	return source, nil
}
