// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"bytes"
	"errors"
	"io"
	"log"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zarfconfig "github.com/zarf-dev/zarf/src/config"
)

func TestWithTempDirConfiguresEachOperation(t *testing.T) {
	resetTempDirConfig(t)
	zarfconfig.CommonOptions.TempDirectory = "before"
	wantErr := errors.New("operation failed")

	result, err := WithTempDir("during", func() (string, error) {
		assert.Equal(t, "during", zarfconfig.CommonOptions.TempDirectory)
		return "result", wantErr
	})

	assert.Equal(t, "result", result)
	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, "during", zarfconfig.CommonOptions.TempDirectory)

	_, err = WithTempDir("after", func() (struct{}, error) {
		assert.Equal(t, "after", zarfconfig.CommonOptions.TempDirectory)
		return struct{}{}, nil
	})
	require.NoError(t, err)
	assert.Equal(t, "after", zarfconfig.CommonOptions.TempDirectory)
}

func TestWithTempDirAllowsConcurrentOperationsInSameDirectory(t *testing.T) {
	resetTempDirConfig(t)
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	errs := make(chan error, 2)

	go func() {
		_, err := WithTempDir("shared", func() (struct{}, error) {
			close(firstStarted)
			<-secondStarted
			return struct{}{}, nil
		})
		errs <- err
	}()
	<-firstStarted

	go func() {
		_, err := WithTempDir("shared", func() (struct{}, error) {
			assert.Equal(t, "shared", zarfconfig.CommonOptions.TempDirectory)
			close(secondStarted)
			return struct{}{}, nil
		})
		errs <- err
	}()

	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
}

func resetTempDirConfig(t *testing.T) {
	t.Helper()

	zarfTempDir.mu.Lock()
	originalValue := zarfTempDir.value
	originalActive := zarfTempDir.active
	originalZarfValue := zarfconfig.CommonOptions.TempDirectory
	zarfTempDir.value = ""
	zarfTempDir.active = 0
	zarfTempDir.mu.Unlock()

	t.Cleanup(func() {
		zarfTempDir.mu.Lock()
		zarfTempDir.value = originalValue
		zarfTempDir.active = originalActive
		zarfconfig.CommonOptions.TempDirectory = originalZarfValue
		zarfTempDir.mu.Unlock()
	})
}

func TestWithTempDirWaitsForDifferentDirectoryAfterFailure(t *testing.T) {
	resetTempDirConfig(t)
	synctest.Test(t, func(t *testing.T) {
		firstStarted := make(chan struct{})
		releaseFirst := make(chan struct{})
		secondStarted := make(chan struct{})
		errs := make(chan error, 2)
		wantErr := errors.New("operation failed")
		go func() {
			_, err := WithTempDir("first", func() (struct{}, error) {
				close(firstStarted)
				<-releaseFirst
				return struct{}{}, wantErr
			})
			errs <- err
		}()
		<-firstStarted
		go func() {
			_, err := WithTempDir("second", func() (struct{}, error) {
				assert.Equal(t, "second", zarfconfig.CommonOptions.TempDirectory)
				close(secondStarted)
				return struct{}{}, nil
			})
			errs <- err
		}()
		synctest.Wait()
		assert.Equal(t, "first", zarfconfig.CommonOptions.TempDirectory)
		select {
		case <-secondStarted:
			t.Error("operation using another directory started before the active operation finished")
		default:
		}
		close(releaseFirst)
		synctest.Wait()
		<-secondStarted
		firstErr, secondErr := <-errs, <-errs
		require.ErrorIs(t, errors.Join(firstErr, secondErr), wantErr)
		assert.Equal(t, "second", zarfconfig.CommonOptions.TempDirectory)
	})
}

func TestSensitiveLoggingRestoresAfterPanic(t *testing.T) {
	original := log.Writer()
	require.Panics(t, func() {
		_ = withSensitiveLogging(true, func() error {
			assert.Equal(t, io.Discard, log.Writer())
			panic("backend panic")
		})
	})
	assert.Equal(t, original, log.Writer())
}

func TestSensitiveLoggingOverlappingDeployments(t *testing.T) {
	var logs bytes.Buffer
	original := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(original) })
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- withSensitiveLogging(true, func() error {
			close(started)
			<-release
			log.Print("secret after overlapping deployment")
			return nil
		})
	}()
	<-started
	wantErr := errors.New("deployment failed")
	err := withSensitiveLogging(true, func() error {
		log.Print("secret during overlapping deployment")
		return wantErr
	})
	assert.Equal(t, io.Discard, log.Writer())
	close(release)
	require.NoError(t, <-finished)
	require.ErrorIs(t, err, wantErr)
	assert.Same(t, &logs, log.Writer())
	assert.Empty(t, logs.String())
	log.Print("restored")
	assert.Contains(t, logs.String(), "restored")
}
