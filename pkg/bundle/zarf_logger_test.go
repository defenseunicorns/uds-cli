// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	zarflogger "github.com/zarf-dev/zarf/src/pkg/logger"
)

func TestNewZarfLoggerContext(t *testing.T) {
	t.Run("uses the caller-bound logger so Zarf output joins the caller's stream", func(t *testing.T) {
		var caller, console bytes.Buffer
		streams := iostreams.New(nil, nil, &console).WithLogger(slog.New(slog.NewJSONHandler(&caller, nil)), nil)

		ctx := newZarfLoggerContext(t.Context(), streams)
		zarflogger.From(ctx).Info("zarf-log-line")

		assert.Contains(t, caller.String(), "zarf-log-line")
		assert.Empty(t, console.String())
	})

	t.Run("leaves ctx unset when no logger is bound so Zarf discards rather than panicking", func(t *testing.T) {
		var console bytes.Buffer
		streams := iostreams.New(nil, nil, &console)

		ctx := newZarfLoggerContext(t.Context(), streams)
		// From returns a discard logger when the key is unset; using it must not panic.
		assert.NotPanics(t, func() { zarflogger.From(ctx).Info("zarf-log-line") })
		assert.Empty(t, console.String())
	})
}
