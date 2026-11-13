// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"sync"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
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
	secrets []string
}

type secretLogWriter struct {
	next     io.Writer
	replacer *strings.Replacer
}

func (w secretLogWriter) Write(p []byte) (int, error) {
	if _, err := io.WriteString(w.next, w.replacer.Replace(string(p))); err != nil {
		return 0, err
	}
	// io.Writer counts consumed input, even when redaction shortens the output.
	return len(p), nil
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
	}
	standardLogRedaction.active++
	standardLogRedaction.secrets = append(standardLogRedaction.secrets, secrets...)
	// Match longer secrets first so a shared prefix does not expose their suffixes.
	sort.Slice(standardLogRedaction.secrets, func(i, j int) bool {
		return len(standardLogRedaction.secrets[i]) > len(standardLogRedaction.secrets[j])
	})
	pairs := make([]string, 0, 2*len(standardLogRedaction.secrets))
	for _, secret := range standardLogRedaction.secrets {
		pairs = append(pairs, secret, "***")
	}
	// Write must not take our mutex: log holds its output lock while calling it,
	// and SetOutput takes that same lock while we hold this mutex.
	log.SetOutput(secretLogWriter{standardLogRedaction.writer, strings.NewReplacer(pairs...)})
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

func newBackendLogRedactor(cfg *UDSBundleConfig) backendLogRedactor {
	var secrets []string
	var collect func(any)
	collect = func(value any) {
		switch v := value.(type) {
		case bundleinternal.Variables:
			for _, child := range v {
				collect(child)
			}
		case []any:
			for _, child := range v {
				collect(child)
			}
		case nil:
		default:
			// An empty search pattern would match every position in a message.
			if text := fmt.Sprint(v); text != "" {
				secrets = append(secrets, text)
			}
		}
	}
	for name, sensitive := range cfg.SensitiveVariables {
		if sensitive {
			collect(cfg.Variables[name])
		}
	}
	return backendLogRedactor{secrets: secrets}
}
