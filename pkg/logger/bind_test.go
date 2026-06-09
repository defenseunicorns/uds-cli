// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package logger

import (
	"bytes"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func TestBind_RoutesToErrOutAtLevel(t *testing.T) {
	var buf bytes.Buffer
	s := iostreams.New(nil, nil, &buf)
	s = Bind(s, "warn")

	s.Info("suppressed-at-warn")
	s.Warn("visible-warn")

	out := buf.String()
	assert.NotContains(t, out, "suppressed-at-warn")
	assert.Contains(t, out, "visible-warn")
}

func TestBind_InvalidLevelFallsBackToInfo(t *testing.T) {
	// Level is validated upstream; Bind itself is lenient and defaults to info.
	var buf bytes.Buffer
	s := Bind(iostreams.New(nil, nil, &buf), "not-a-level")

	s.Debug("debug-suppressed")
	s.Info("info-visible")

	out := buf.String()
	assert.NotContains(t, out, "debug-suppressed")
	assert.Contains(t, out, "info-visible")
}
