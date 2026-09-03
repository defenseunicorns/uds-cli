// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package core

import "errors"

var (
	ErrReadLogLevel      = errors.New("reading log-level flag")
	ErrInvalidStreamKind = errors.New("invalid stream kind")
	ErrInvalidSince      = errors.New("since must not be negative")
)
