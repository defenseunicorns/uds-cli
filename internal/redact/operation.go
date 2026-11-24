// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package redact

import (
	"context"
	"io"
	"log/slog"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

type stderrKey struct{}

// Bind filters diagnostics written to streams and returns the scope for masking errors.
func Bind(ctx context.Context, streams *iostreams.IOStreams) (context.Context, *Scope) {
	scope := FromContext(ctx)
	if scope == nil {
		ctx, scope = WithScope(ctx)
	}
	original := *streams
	stderr, _ := ctx.Value(stderrKey{}).(io.Writer)
	if original.ErrOut() != stderr {
		*streams = iostreams.New(original.In(), original.Out(), Writer(original.ErrOut(), scope))
		ctx = context.WithValue(ctx, stderrKey{}, streams.ErrOut())
	}
	if log := original.Logger(); log != nil {
		handler := log.Handler()
		if bound, ok := handler.(maskingHandler); !ok || bound.scope != scope {
			log = slog.New(maskingHandler{Handler: handler, scope: scope})
		}
		*streams = streams.WithLogger(log, original.LogLevel())
	}
	return ctx, scope
}

// maskingHandler preserves a caller's logger, including its levels and output destination.
type maskingHandler struct {
	slog.Handler
	scope *Scope
	bound func(textReplacer) slog.Handler
}

func (h maskingHandler) handler(mask textReplacer) slog.Handler {
	if h.bound != nil {
		return h.bound(mask)
	}
	return h.Handler
}

func (h maskingHandler) Handle(ctx context.Context, r slog.Record) error {
	mask := h.scope.replacer()
	if mask == nil {
		return h.handler(nil).Handle(ctx, r)
	}
	clean := slog.NewRecord(r.Time, r.Level, mask.Replace(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(maskAttr(a, mask))
		return true
	})
	return h.handler(mask).Handle(ctx, clean)
}

func maskAttr(a slog.Attr, mask textReplacer) slog.Attr {
	a.Key = mask.Replace(a.Key)
	value := a.Value.Resolve()
	if value.Kind() == slog.KindGroup {
		group := value.Group()
		attrs := make([]slog.Attr, len(group))
		for i, child := range group {
			attrs[i] = maskAttr(child, mask)
		}
		a.Value = slog.GroupValue(attrs...)
	} else if text := value.String(); mask.Replace(text) != text {
		a.Value = slog.StringValue(mask.Replace(text))
	} else {
		a.Value = value
	}
	return a
}

func (h maskingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	parent := h
	h.bound = func(mask textReplacer) slog.Handler {
		if mask == nil {
			return parent.handler(nil).WithAttrs(attrs)
		}
		clean := make([]slog.Attr, len(attrs))
		for i, a := range attrs {
			clean[i] = maskAttr(a, mask)
		}
		return parent.handler(mask).WithAttrs(clean)
	}
	return h
}

func (h maskingHandler) WithGroup(name string) slog.Handler {
	parent := h
	h.bound = func(mask textReplacer) slog.Handler {
		if mask != nil {
			return parent.handler(mask).WithGroup(mask.Replace(name))
		}
		return parent.handler(nil).WithGroup(name)
	}
	return h
}
