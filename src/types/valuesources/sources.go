// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package valuesources

type Source string

const (
	Config Source = "config"
	Env    Source = "env"
	CLI    Source = "cli"
	Bundle Source = "bundle"
)
