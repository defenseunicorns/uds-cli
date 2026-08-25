// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import "errors"

var (
	ErrInvalidArgument       = errors.New("invalid command argument")
	ErrPathNotFound          = errors.New("path not found")
	ErrInvalidPath           = errors.New("invalid path")
	ErrUnsupportedSource     = errors.New("unsupported bundle source")
	ErrParseConfig           = errors.New("parsing bundle config")
	ErrAccessDefaults        = errors.New("accessing bundle defaults")
	ErrParseBundle           = errors.New("parsing bundle")
	ErrInvalidBundle         = errors.New("invalid bundle")
	ErrForceRequired         = errors.New("operation requires force")
	ErrPullBundle            = errors.New("pulling bundle")
	ErrPushBundle            = errors.New("pushing bundle")
	ErrCreateWorkspace       = errors.New("creating bundle workspace")
	ErrResolvePath           = errors.New("resolving path")
	ErrUnsafePullOutput      = errors.New("unsafe pull output path")
	ErrReadConfirmation      = errors.New("reading confirmation")
	ErrWriteDefinitionNotice = errors.New("writing bundle definition notice")
)
