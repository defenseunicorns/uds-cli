// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import "github.com/defenseunicorns/uds-cli/pkg/bundle/spec"

const (
	tempDirPerm = 0o700
	tmpFilePerm = 0o600
)

// UDSBlock aliases the canonical constraints model for package tests.
type UDSBlock = spec.UDSBlock

// Metadata aliases the canonical metadata model for package tests.
type Metadata = spec.Metadata

// Package aliases the canonical package model for package tests.
type Package = spec.Package

// PackageRef aliases the canonical dependency reference model for package tests.
type PackageRef = spec.PackageRef

// PackageSignatureVerification aliases the canonical signature policy model for package tests.
type PackageSignatureVerification = spec.PackageSignatureVerification

// KeylessSignatureVerification aliases the canonical keyless policy model for package tests.
type KeylessSignatureVerification = spec.KeylessSignatureVerification
