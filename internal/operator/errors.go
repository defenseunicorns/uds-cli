// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package operator

import "errors"

// ErrNoTargetsFound indicates that no Pepr log targets were discovered.
var ErrNoTargetsFound = errors.New("no potential targets for monitoring found")
