// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package interactive contains functions for interacting with the user via STDIN.
package interactive

import (
	"github.com/AlecAivazis/survey/v2"
)

// PromptSigPassword prompts the user for the password to their private key
func PromptSigPassword() ([]byte, error) {
	var password string

	prompt := &survey.Password{
		Message: "Private key password (empty for no password): ",
	}
	err := survey.AskOne(prompt, &password)
	if err != nil {
		return []byte{}, err
	}
	return []byte(password), nil
}
