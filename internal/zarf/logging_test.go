// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"bytes"
	"context"
	"errors"
	"log"
	"log/slog"
	"testing"
	"testing/synctest"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zarflogger "github.com/zarf-dev/zarf/src/pkg/logger"
	helmutil "helm.sh/helm/v4/pkg/chart/common/util"
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

func captureGlobalLogs(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	originalLogger, originalWriter, originalFlags := slog.Default(), log.Writer(), log.Flags()
	var standard, structured bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&structured, nil)))
	log.SetOutput(&standard)
	log.SetFlags(log.Lshortfile)
	t.Cleanup(func() {
		slog.SetDefault(originalLogger)
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
	})
	return &standard, &structured
}

func TestBundleLoggingRestoresGlobals(t *testing.T) {
	for _, outcome := range []string{"success", "error", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			standard, structured := captureGlobalLogs(t)
			previousLogger, previousWriter, previousFlags := slog.Default(), log.Writer(), log.Flags()
			wantErr := errors.New("backend failure")
			run := func() error {
				_, restore, err := silenceBundleLogs(t.Context())
				require.NoError(t, err)
				defer restore()
				log.Print("secret")
				slog.Error("secret")
				if outcome == "panic" {
					panic("backend panic")
				}
				if outcome == "error" {
					return wantErr
				}
				return nil
			}
			switch outcome {
			case "panic":
				require.Panics(t, func() { _ = run() })
			case "error":
				require.ErrorIs(t, run(), wantErr)
			default:
				require.NoError(t, run())
			}
			assert.Same(t, previousLogger, slog.Default())
			assert.Same(t, previousWriter, log.Writer())
			assert.Equal(t, previousFlags, log.Flags())
			assert.Empty(t, standard.String())
			assert.Empty(t, structured.String())
			log.Print("standard restored")
			slog.Info("structured restored")
			assert.Contains(t, standard.String(), "standard restored")
			assert.Contains(t, structured.String(), "structured restored")
		})
	}
}

func TestBundleLoggingSerializesBundles(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		standard, structured := captureGlobalLogs(t)
		done := make(chan error, 1)
		func() {
			_, restore, err := silenceBundleLogs(t.Context())
			require.NoError(t, err)
			defer restore()
			go func() {
				_, release, err := silenceBundleLogs(t.Context())
				if err == nil {
					log.Print("second bundle secret")
					slog.Warn("second bundle secret")
					release()
				}
				done <- err
			}()
			synctest.Wait()
			select {
			case err := <-done:
				t.Fatalf("second bundle entered before first finished: %v", err)
			default:
			}
		}()
		require.NoError(t, <-done)
		assert.Empty(t, standard.String())
		assert.Empty(t, structured.String())
	})
}

func TestBundleLoggingWaitCanBeCanceled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, restore, err := silenceBundleLogs(t.Context())
		require.NoError(t, err)
		defer restore()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, release, err := silenceBundleLogs(ctx)
			if release != nil {
				release()
			}
			done <- err
		}()
		synctest.Wait()
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
	})
}

func TestDeployBundleKeepsPackagesParallelAndGlobalsMuted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		standard, structured := captureGlobalLogs(t)
		streams, _, _, progress := iostreams.NewTestIOStreams()
		d := NewZarfDeployer(streams, nil)
		b := &spec.UDSBundle{
			UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
			Metadata: spec.Metadata{Name: "test"},
			Packages: []spec.Package{{Name: "alpha", Source: "oci://example/alpha:v1"}, {Name: "beta", Source: "oci://example/beta:v1"}},
		}
		entered := make(chan string, 2)
		release := make(chan struct{})
		done := make(chan error, 1)
		wantErr := errors.New("package failed")
		go func() {
			_, err := d.DeployBundle(t.Context(), b, DeployOptions{
				Config: newDeployTestConfig(2),
				PackageDeployFn: func(ctx context.Context, pkg *spec.Package, _ DeployPackageOptions) error {
					entered <- pkg.Name
					<-release
					helmutil.CoalesceTables(map[string]any{"password": map[string]any{}}, map[string]any{"password": "secret-canary"})
					slog.Warn("secret-canary")
					_, nestedErr := d.DeployBundle(ctx, b, DeployOptions{Config: newDeployTestConfig(1)})
					assert.ErrorIs(t, nestedErr, ErrNestedBundleDeploy)
					if pkg.Name == "alpha" {
						return wantErr
					}
					return nil
				},
			})
			done <- err
		}()
		synctest.Wait()
		// Both callbacks must enter before either is allowed to finish.
		count := len(entered)
		assert.Equal(t, 2, count)
		close(release)
		require.ErrorIs(t, <-done, wantErr)
		assert.Empty(t, standard.String())
		assert.Empty(t, structured.String())
		assert.Contains(t, progress.String(), "deploying package")
		log.Print("restored after packages finished")
		assert.Contains(t, standard.String(), "restored after packages finished")
	})
}
