// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package mode

import (
	"fmt"
	"strings"
)

const (
	flagName = "--cli-mode"

	// EnvName is the environment variable used to select the CLI mode.
	EnvName = "UDS_CLI_MODE"
)

// Resolve selects a mode and removes its bootstrap option from args.
func Resolve(args []string, lookupEnv func(string) (string, bool)) (Mode, []string, error) {
	cleaned := make([]string, 0, len(args))
	value := ""
	found := false

	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			cleaned = append(cleaned, args[index:]...)
			break
		}

		if argument == flagName {
			if found {
				return "", nil, fmt.Errorf("%s may only be specified once", flagName)
			}
			if index+1 >= len(args) || args[index+1] == "--" || strings.HasPrefix(args[index+1], "-") {
				return "", nil, fmt.Errorf("%s requires a value", flagName)
			}
			value = args[index+1]
			found = true
			index++
			continue
		}

		if strings.HasPrefix(argument, flagName+"=") {
			if found {
				return "", nil, fmt.Errorf("%s may only be specified once", flagName)
			}
			value = strings.TrimPrefix(argument, flagName+"=")
			if value == "" {
				return "", nil, fmt.Errorf("%s requires a value", flagName)
			}
			found = true
			continue
		}

		cleaned = append(cleaned, argument)
	}

	if !found {
		if envValue, ok := lookupEnv(EnvName); ok {
			value = envValue
		} else {
			value = Legacy.String()
		}
	}

	selected, err := parse(value)
	if err != nil {
		return "", nil, err
	}
	return selected, cleaned, nil
}
