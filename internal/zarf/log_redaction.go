// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"io"
	"log"
	"maps"
	"slices"
	"sync"

	"github.com/defenseunicorns/uds-cli/internal/redact"
)

// backendLogRedactor snapshots marked values before deployment hooks can change them.
type backendLogRedactor struct {
	secrets []string
}

// Run registers this call's secrets and releases its scope on return or panic.
func (r backendLogRedactor) Run(deploy func() error) error {
	defer redactStandardLogs(r.secrets)()
	return deploy()
}

// Registration must update the saved writer, secrets, and global output together.
// An atomic count alone cannot prevent overlapping calls from replacing each other.
var standardLogRedaction struct {
	sync.Mutex
	active  int
	writer  io.Writer
	secrets map[string]struct{}
}

// redactStandardLogs retains overlapping calls' secrets until the last call finishes.
func redactStandardLogs(secrets []string) func() {
	if len(secrets) == 0 {
		return func() {}
	}
	standardLogRedaction.Lock()
	defer standardLogRedaction.Unlock()
	if standardLogRedaction.active == 0 {
		standardLogRedaction.writer = log.Writer()
		standardLogRedaction.secrets = make(map[string]struct{})
	}
	standardLogRedaction.active++
	previous := len(standardLogRedaction.secrets)
	for _, secret := range secrets {
		standardLogRedaction.secrets[secret] = struct{}{}
	}
	// Write must not take our mutex: log holds its output lock while calling it,
	// and SetOutput takes that same lock while we hold this mutex.
	if len(standardLogRedaction.secrets) != previous {
		log.SetOutput(redact.Writer(standardLogRedaction.writer, redact.Text(slices.Collect(maps.Keys(standardLogRedaction.secrets)))))
	}
	return func() {
		standardLogRedaction.Lock()
		defer standardLogRedaction.Unlock()
		standardLogRedaction.active--
		if standardLogRedaction.active == 0 {
			log.SetOutput(standardLogRedaction.writer)
			standardLogRedaction.writer = nil
			standardLogRedaction.secrets = nil
		}
	}
}

func newBackendLogRedactor(ctx context.Context, cfg *UDSBundleConfig) backendLogRedactor {
	secrets := redact.FromContext(ctx).Values()
	secrets = append(secrets, cfg.Variables.SensitiveValues(cfg.SensitiveVariables)...)
	return backendLogRedactor{secrets: secrets}
}
