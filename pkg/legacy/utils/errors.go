// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package utils

import "errors"

// ErrNextBundle indicates that a Legacy operation received a Next bundle.
var ErrNextBundle = errors.New("bundle uses the Next format; use Next mode: CLI_FEATURES=NextMode=true uds bundle --help")
