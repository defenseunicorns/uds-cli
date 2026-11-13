// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"io"
	"log"
	"sync"

	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/hashicorp/hcl/v2"
	zarfconfig "github.com/zarf-dev/zarf/src/config"
)

var zarfTempDir = newTempDirConfig()

var suppressedLogs struct {
	sync.Mutex
	active int
	writer io.Writer
}

// Helm uses the standard logger; keep it muted until all sensitive deployments finish.
func withSensitiveLogging(sensitive bool, run func() error) error {
	if !sensitive {
		return run()
	}
	suppressedLogs.Lock()
	if suppressedLogs.active == 0 {
		suppressedLogs.writer = log.Writer()
		log.SetOutput(io.Discard)
	}
	suppressedLogs.active++
	suppressedLogs.Unlock()
	defer func() {
		suppressedLogs.Lock()
		defer suppressedLogs.Unlock()
		suppressedLogs.active--
		if suppressedLogs.active == 0 {
			log.SetOutput(suppressedLogs.writer)
			suppressedLogs.writer = nil
		}
	}()
	return run()
}

type tempDirConfig struct {
	mu     sync.Mutex
	idle   *sync.Cond
	value  string
	active int
}

func newTempDirConfig() *tempDirConfig {
	c := &tempDirConfig{}
	c.idle = sync.NewCond(&c.mu)
	return c
}

// UDSBundleConfig is the private resolved deployment configuration.
type UDSBundleConfig struct {
	Options            *bundleinternal.ConfigOptions `hcl:"options,block"`
	Variables          bundleinternal.Variables
	SensitiveVariables map[string]bool
	Remain             hcl.Body `hcl:",remain"`
}

// WithTempDir runs an operation while Zarf's process-global temporary directory
// matches the process configuration. Operations using the same directory may
// overlap; an operation using another directory waits for them to finish.
func WithTempDir[T any](tmpDir string, run func() (T, error)) (T, error) {
	zarfTempDir.mu.Lock()
	for zarfTempDir.active > 0 && zarfTempDir.value != tmpDir {
		zarfTempDir.idle.Wait()
	}
	if zarfTempDir.active == 0 {
		zarfTempDir.value = tmpDir
		zarfconfig.CommonOptions.TempDirectory = tmpDir
	}
	zarfTempDir.active++
	zarfTempDir.mu.Unlock()

	defer func() {
		zarfTempDir.mu.Lock()
		zarfTempDir.active--
		if zarfTempDir.active == 0 {
			zarfTempDir.idle.Broadcast()
		}
		zarfTempDir.mu.Unlock()
	}()

	return run()
}

func withTempDir(tmpDir string, run func() error) error {
	_, err := WithTempDir(tmpDir, func() (struct{}, error) {
		return struct{}{}, run()
	})
	return err
}
