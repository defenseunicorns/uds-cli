// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package bundle provides functionality for managing UDS bundles.
//
// When a diagnostic contains a known sensitive value, its [MaskedError] masks
// the value in the text returned by Error. Use [errors.As] to retrieve a
// specific underlying type. Dependency violations and unsupported bundle API
// version errors retain a masked Error message and their original structured
// fields. Those fields and other recovered error types can contain sensitive data.
// For logs and user-facing output, format the error returned by the operation.
package bundle
