// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package redact

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
)

type scopeKey struct{}

// Scope tracks sensitive values discovered during one operation.
type Scope struct {
	mu     sync.Mutex
	values map[string]bool
	mask   *strings.Replacer
}

func WithScope(ctx context.Context) (context.Context, *Scope) {
	s := &Scope{values: make(map[string]bool)}
	return context.WithValue(ctx, scopeKey{}, s), s
}

func FromContext(ctx context.Context) *Scope {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(scopeKey{}).(*Scope)
	return s
}

func (s *Scope) Add(values ...string) {
	if s == nil || len(values) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.values)
	for _, v := range values {
		if v != "" {
			s.values[v] = true
		}
	}
	if n != len(s.values) {
		s.mask = nil
	}
}

func (s *Scope) Values() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Collect(maps.Keys(s.values))
}

func (s *Scope) replacer() *strings.Replacer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mask == nil && len(s.values) > 0 {
		s.mask = Text(slices.Collect(maps.Keys(s.values)))
	}
	return s.mask
}

func (s *Scope) Replace(text string) string {
	if mask := s.replacer(); mask != nil {
		return mask.Replace(text)
	}
	return text
}
