// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package message provides a rich set of functions for displaying messages to the user.
package message

import (
	"fmt"

	"github.com/zarf-dev/zarf/src/pkg/state"
)

// PrintConnectStringTable prints a table of connect strings.
func PrintConnectStringTable(connectStrings state.ConnectStrings) {
	if len(connectStrings) > 0 {
		connectData := [][]string{}
		// Loop over each connectStrings and convert to a string matrix
		for name, connect := range connectStrings {
			name = fmt.Sprintf("zarf connect %s", name)
			connectData = append(connectData, []string{name, connect.Description})
		}

		// Create the table output with the data
		header := []string{"Connect Command", "Description"}
		TableWithWriter(OutputWriter, header, connectData)
	}
}
