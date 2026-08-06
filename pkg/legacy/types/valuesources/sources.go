// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package valuesources

type Source string

const (
	Config Source = "config"
	Env    Source = "env"
	CLI    Source = "cli"
	Bundle Source = "bundle"
)
