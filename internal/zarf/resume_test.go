// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/state"
)

type packageSpecLoaderFunc func(context.Context, *spec.Package) (*PackageSpec, error)

func (f packageSpecLoaderFunc) LoadPackageSpec(ctx context.Context, pkg *spec.Package) (*PackageSpec, error) {
	return f(ctx, pkg)
}

func TestDeployBundleResumeComponentEvents(t *testing.T) {
	bundle := &spec.UDSBundle{
		UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: spec.Metadata{Name: "bundle"},
		Packages: []spec.Package{{Name: "label", Source: "oci://example.com/pkg:v1"}},
	}
	type testCase struct {
		name      string
		component state.DeployedComponent
		skip      bool
	}
	tests := []testCase{
		{name: "missing event"},
		{name: "timestamp alone", component: state.DeployedComponent{LastEvent: state.ComponentEvent{Timestamp: time.Now()}}},
	}
	for _, eventType := range []state.EventType{state.EventTypeDeploy, state.EventTypeRemove, "", "Future"} {
		for _, outcome := range []state.EventOutcome{state.EventOutcomeSucceeded, state.EventOutcomeFailed, state.EventOutcomeInProgress, state.EventOutcomeCancelled, state.EventOutcomeUnknown, "", "Future"} {
			if eventType == "" && outcome == "" {
				continue
			}
			tests = append(tests, testCase{
				name:      string(eventType) + "/" + string(outcome),
				component: state.DeployedComponent{LastEvent: state.ComponentEvent{Type: eventType, Outcome: outcome}},
				skip:      eventType == state.EventTypeDeploy && outcome == state.EventOutcomeSucceeded,
			})
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deploys := 0
			tt.component.Name = "main"
			result, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(t.Context(), bundle, DeployOptions{
				Config: newDeployTestConfig(1), Resume: true,
				SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) {
					return &PackageSpec{Name: "zarf-name", Digest: "sha256:one", Components: []string{"main"}}, nil
				}),
				DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) {
					record := state.DeployedPackage{Name: "zarf-name", Digest: "sha256:one", Generation: 2}
					event := tt.component.LastEvent
					if event.Type != "" || event.Outcome != "" || !event.Timestamp.IsZero() {
						record.Events = []state.PackageEvent{{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeSucceeded, Digest: "sha256:one"}}
						tt.component.ObservedGeneration = record.Generation
					}
					record.DeployedComponents = []state.DeployedComponent{tt.component}
					return []state.DeployedPackage{record}, nil
				},
				PackageDeployFn: func(context.Context, *spec.Package, DeployPackageOptions) error { deploys++; return nil },
			})
			require.NoError(t, err)
			wantDeploys := 1
			if tt.skip {
				wantDeploys = 0
			}
			assert.Equal(t, wantDeploys, deploys)
			assert.Len(t, result.Packages, wantDeploys)
		})
	}
}

func TestDeployBundleResumePackageEvents(t *testing.T) {
	bundle := &spec.UDSBundle{
		UDS:      spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"},
		Metadata: spec.Metadata{Name: "bundle"},
		Packages: []spec.Package{{Name: "label", Source: "oci://example.com/pkg:v2"}},
	}
	success := state.PackageEvent{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeSucceeded, Digest: "sha256:two"}
	componentSuccess := state.ComponentEvent{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeSucceeded}
	for _, tt := range []struct {
		name             string
		events           []state.PackageEvent
		generations      []int
		legacyComponents bool
		skip             bool
	}{
		{name: "completed upgrade", events: []state.PackageEvent{success}, generations: []int{2, 2}, skip: true},
		{name: "crash before first component write", events: []state.PackageEvent{{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeSucceeded, Digest: "sha256:one"}, {Type: state.EventTypeDeploy, Outcome: state.EventOutcomeInProgress, Digest: "sha256:two"}}, generations: []int{1, 1}},
		{name: "upgrade from legacy before first component write", events: []state.PackageEvent{{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeInProgress, Digest: "sha256:two"}}, generations: []int{0, 0}, legacyComponents: true},
		{name: "all components done before package success", events: []state.PackageEvent{{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeInProgress, Digest: "sha256:two"}}, generations: []int{2, 2}},
		{name: "failed success hook", events: []state.PackageEvent{success, {Type: state.EventTypeDeploy, Outcome: state.EventOutcomeFailed, Digest: "sha256:two"}}, generations: []int{2, 2}},
		{name: "cancelled deploy", events: []state.PackageEvent{{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeCancelled, Digest: "sha256:two"}}, generations: []int{2, 2}},
		{name: "unknown outcome", events: []state.PackageEvent{{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeUnknown, Digest: "sha256:two"}}, generations: []int{2, 2}},
		{name: "successful remove", events: []state.PackageEvent{{Type: state.EventTypeRemove, Outcome: state.EventOutcomeSucceeded, Digest: "sha256:two"}}, generations: []int{2, 2}},
		{name: "missing event digest", events: []state.PackageEvent{{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeSucceeded}}, generations: []int{2, 2}},
		{name: "different event digest", events: []state.PackageEvent{{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeSucceeded, Digest: "sha256:one"}}, generations: []int{2, 2}},
		{name: "partial upgrade retains old component", events: []state.PackageEvent{success}, generations: []int{2, 1}},
		{name: "future component generation", events: []state.PackageEvent{success}, generations: []int{2, 3}},
		{name: "missing component generation", events: []state.PackageEvent{success}, generations: []int{2, 0}},
		{name: "modern package with legacy components", events: []state.PackageEvent{success}, generations: []int{2, 2}, legacyComponents: true},
		{name: "component events without package event", generations: []int{2, 2}},
		{name: "empty package event", events: []state.PackageEvent{{}}, generations: []int{0, 0}, legacyComponents: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			components := []state.DeployedComponent{{Name: "main"}, {Name: "optional"}}
			for i := range components {
				components[i].ObservedGeneration = tt.generations[i]
				if !tt.legacyComponents {
					components[i].LastEvent = componentSuccess
				}
			}
			deploys := 0
			result, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(t.Context(), bundle, DeployOptions{
				Config: newDeployTestConfig(1), Resume: true,
				SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) {
					return &PackageSpec{Name: "zarf-name", Digest: "sha256:two", Components: []string{"main", "optional"}}, nil
				}),
				DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) {
					return []state.DeployedPackage{{Name: "zarf-name", Digest: "sha256:two", Generation: 2, Events: tt.events, DeployedComponents: components}}, nil
				},
				PackageDeployFn: func(context.Context, *spec.Package, DeployPackageOptions) error { deploys++; return nil },
			})
			require.NoError(t, err)
			wantDeploys := 1
			if tt.skip {
				wantDeploys = 0
			}
			assert.Equal(t, wantDeploys, deploys)
			assert.Len(t, result.Packages, wantDeploys)
		})
	}
}

func TestResumeMatchesModernGeneration(t *testing.T) {
	for _, generation := range []int{-1, 0, 1} {
		t.Run(fmt.Sprintf("generation %d", generation), func(t *testing.T) {
			for _, names := range [][]string{nil, {"main"}} {
				intended := &PackageSpec{Digest: "sha256:one", Components: names}
				record := state.DeployedPackage{
					Digest: "sha256:one", Generation: generation,
					Events: []state.PackageEvent{{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeSucceeded, Digest: "sha256:one"}},
				}
				for _, name := range names {
					record.DeployedComponents = append(record.DeployedComponents, state.DeployedComponent{
						Name: name, ObservedGeneration: generation,
						LastEvent: state.ComponentEvent{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeSucceeded},
					})
				}
				assert.Equal(t, generation > 0, resumeMatches(intended, []state.DeployedPackage{record}), "components: %v", names)
			}
		})
	}
}

func successfulResumeRecord(name, namespace, packageDigest string, components ...string) state.DeployedPackage {
	record := state.DeployedPackage{
		Name: name, NamespaceOverride: namespace, Digest: packageDigest, Generation: 1,
		Events: []state.PackageEvent{{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeSucceeded, Digest: digest.Digest(packageDigest)}},
	}
	for _, name := range components {
		record.DeployedComponents = append(record.DeployedComponents, state.DeployedComponent{
			Name: name, ObservedGeneration: record.Generation,
			LastEvent: state.ComponentEvent{Type: state.EventTypeDeploy, Outcome: state.EventOutcomeSucceeded},
		})
	}
	return record
}

func TestResumeMatchesRequiresPackageEvent(t *testing.T) {
	// Read the old wire format to verify that a successful legacy status does
	// not qualify a record without a completed deployment event.
	var record state.DeployedPackage
	require.NoError(t, json.Unmarshal([]byte(`{"digest":"sha256:one","deployedComponents":[{"name":"main","status":"Succeeded"}]}`), &record))
	intended := &PackageSpec{Digest: record.Digest, Components: []string{"main"}}
	require.Len(t, record.DeployedComponents, 1)
	assert.False(t, resumeMatches(intended, []state.DeployedPackage{record}))
	intended.Components = nil
	record.DeployedComponents = nil
	assert.False(t, resumeMatches(intended, []state.DeployedPackage{record}))
}

func TestResumeMatches(t *testing.T) {
	intended := &PackageSpec{Name: "zarf-name", Digest: "sha256:one", Components: []string{"required", "optional"}}
	matching := []state.DeployedPackage{successfulResumeRecord("zarf-name", "", intended.Digest, "optional", "required")}
	failed := successfulResumeRecord("zarf-name", "", intended.Digest, "required", "optional")
	failed.DeployedComponents[1].LastEvent.Outcome = state.EventOutcomeFailed
	redeploy := []struct {
		name     string
		intended *PackageSpec
		deployed []state.DeployedPackage
	}{
		{name: "missing state"},
		{name: "missing intended digest", intended: &PackageSpec{Name: "zarf-name", Components: intended.Components}, deployed: matching},
		{name: "missing deployed digest", intended: intended, deployed: []state.DeployedPackage{successfulResumeRecord("zarf-name", "", "", "optional", "required")}},
		{name: "different digest", intended: intended, deployed: []state.DeployedPackage{successfulResumeRecord("zarf-name", "", "sha256:two", "optional", "required")}},
		{name: "failed component", intended: intended, deployed: []state.DeployedPackage{failed}},
		{name: "missing component", intended: intended, deployed: []state.DeployedPackage{successfulResumeRecord("zarf-name", "", intended.Digest, "required")}},
		{name: "extra component", intended: intended, deployed: []state.DeployedPackage{successfulResumeRecord("zarf-name", "", intended.Digest, "required", "optional", "extra")}},
		{name: "duplicate deployed component", intended: intended, deployed: []state.DeployedPackage{successfulResumeRecord("zarf-name", "", intended.Digest, "required", "required")}},
		{name: "duplicate intended component", intended: &PackageSpec{Name: "zarf-name", Digest: intended.Digest, Components: []string{"required", "required"}}, deployed: matching},
		{name: "duplicate deployed package", intended: intended, deployed: append(matching, matching[0])},
	}

	assert.True(t, resumeMatches(intended, matching))
	for _, tt := range redeploy {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, resumeMatches(tt.intended, tt.deployed))
		})
	}
}

func TestFilterResumeLevels_UsesBatchStateAndZarfIdentity(t *testing.T) {
	levels := [][]*spec.Package{{{Name: "hcl-label", Namespace: "team-a"}}}
	stateCalls := 0
	loaderCalls := 0
	filtered, err := filterResumeLevels(t.Context(), levels, DeployOptions{
		SpecLoader: packageSpecLoaderFunc(func(_ context.Context, pkg *spec.Package) (*PackageSpec, error) {
			loaderCalls++
			assert.Equal(t, "hcl-label", pkg.Name)
			return &PackageSpec{Name: "zarf-name", Digest: "sha256:one", Components: []string{"main"}}, nil
		}),
		DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) {
			stateCalls++
			return []state.DeployedPackage{successfulResumeRecord("zarf-name", "team-a", "sha256:one", "main")}, nil
		},
	}, iostreams.IOStreams{})
	require.NoError(t, err)
	assert.Equal(t, 1, stateCalls)
	assert.Equal(t, 1, loaderCalls)
	assert.Empty(t, filtered)
}

func TestFilterResumeLevels_RedeploysNamespaceMismatch(t *testing.T) {
	levels := [][]*spec.Package{{{Name: "pkg", Namespace: "team-a"}}}
	filtered, err := filterResumeLevels(t.Context(), levels, DeployOptions{
		SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) {
			return &PackageSpec{Name: "zarf-name", Digest: "sha256:one", Components: []string{"main"}}, nil
		}),
		DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) {
			return []state.DeployedPackage{successfulResumeRecord("zarf-name", "team-b", "sha256:one", "main")}, nil
		},
	}, iostreams.IOStreams{})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "pkg", filtered[0][0].Name)
}

func TestDeployBundleResumeOrderAndFailures(t *testing.T) {
	bundle := &spec.UDSBundle{UDS: spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"}, Metadata: spec.Metadata{Name: "bundle"}, Packages: []spec.Package{{Name: "core", Source: "oci://example.com/core:v1"}, {Name: "app", Source: "oci://example.com/app:v1", DependsOn: []spec.PackageRef{{Name: "core"}}}}}
	t.Run("disabled performs no state or spec reads", func(t *testing.T) {
		stateCalls, specCalls, deployCalls := 0, 0, 0
		result, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(t.Context(), bundle, DeployOptions{
			Config: newDeployTestConfig(1),
			SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) {
				specCalls++
				return nil, errors.New("unexpected spec read")
			}),
			DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) {
				stateCalls++
				return nil, errors.New("unexpected state read")
			},
			PackageDeployFn: func(context.Context, *spec.Package, DeployPackageOptions) error { deployCalls++; return nil },
		})
		require.NoError(t, err)
		assert.Equal(t, 0, stateCalls)
		assert.Equal(t, 0, specCalls)
		assert.Equal(t, 2, deployCalls)
		assert.Len(t, result.Packages, 2)
	})

	t.Run("selection happens before spec loading", func(t *testing.T) {
		var loaded []string
		_, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(t.Context(), bundle, DeployOptions{
			Config:   newDeployTestConfig(1),
			Resume:   true,
			Packages: []string{"app"},
			SpecLoader: packageSpecLoaderFunc(func(_ context.Context, pkg *spec.Package) (*PackageSpec, error) {
				loaded = append(loaded, pkg.Name)
				return &PackageSpec{Name: pkg.Name, Digest: "sha256:one"}, nil
			}),
			DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) { return nil, nil },
			PackageDeployFn:    func(context.Context, *spec.Package, DeployPackageOptions) error { return nil },
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"app"}, loaded)
	})

	for _, tt := range []struct {
		name string
		opts DeployOptions
	}{
		{name: "state read", opts: DeployOptions{DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) { return nil, errors.New("state") }}},
		{name: "spec read", opts: DeployOptions{DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) { return nil, nil }, SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) { return nil, errors.New("spec") })}},
		{name: "missing digest", opts: DeployOptions{DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) { return nil, nil }, SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) { return &PackageSpec{Name: "pkg"}, nil })}},
	} {
		t.Run(tt.name+" aborts before hooks and deploy", func(t *testing.T) {
			hookCalls, deployCalls := 0, 0
			tt.opts.Config = newDeployTestConfig(1)
			tt.opts.Resume = true
			tt.opts.BundleDeployHooks.PreDeploy = func(context.Context, *spec.UDSBundle, *DeployOptions) error { hookCalls++; return nil }
			tt.opts.PackageDeployFn = func(context.Context, *spec.Package, DeployPackageOptions) error { deployCalls++; return nil }
			_, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(t.Context(), bundle, tt.opts)
			require.Error(t, err)
			assert.Zero(t, hookCalls)
			assert.Zero(t, deployCalls)
		})
	}
}

func TestDeployBundleResume_AllSkippedRunsHooks(t *testing.T) {
	bundle := &spec.UDSBundle{UDS: spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"}, Metadata: spec.Metadata{Name: "bundle"}, Packages: []spec.Package{{Name: "label", Namespace: "team", Source: "oci://example.com/pkg:v1"}}}
	pre, post, deployed := 0, 0, 0
	result, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(t.Context(), bundle, DeployOptions{
		Config: newDeployTestConfig(1), Resume: true,
		SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) {
			return &PackageSpec{Name: "zarf-name", Digest: "sha256:one", Components: []string{"main"}}, nil
		}),
		DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) {
			return []state.DeployedPackage{successfulResumeRecord("zarf-name", "team", "sha256:one", "main")}, nil
		},
		BundleDeployHooks: BundleDeployHooks{PreDeploy: func(context.Context, *spec.UDSBundle, *DeployOptions) error { pre++; return nil }, PostDeploy: func(context.Context, *spec.UDSBundle) error { post++; return nil }},
		PackageDeployFn:   func(context.Context, *spec.Package, DeployPackageOptions) error { deployed++; return nil },
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotNil(t, result.Packages)
	assert.Empty(t, result.Packages)
	assert.Equal(t, 1, pre)
	assert.Equal(t, 1, post)
	assert.Zero(t, deployed)
}

func TestDeployBundleResumeRejectsPackagePreDeployHooks(t *testing.T) {
	bundle := &spec.UDSBundle{UDS: spec.UDSBlock{BundleAPIVersion: "uds.dev/v1alpha1"}, Metadata: spec.Metadata{Name: "bundle"}, Packages: []spec.Package{{Name: "pkg", Source: "oci://example.com/pkg:v1"}}}
	for _, tt := range []struct {
		name    string
		install func(*DeployOptions)
	}{
		{
			name: "direct",
			install: func(opts *DeployOptions) {
				opts.PackageDeployHooks.PreDeploy = func(context.Context, *spec.Package, *layout.PackageLayout, *packager.DeployOptions, *DeployPackageOptions) error {
					return nil
				}
			},
		},
		{
			name: "bundle pre-deploy",
			install: func(opts *DeployOptions) {
				opts.BundleDeployHooks.PreDeploy = func(_ context.Context, _ *spec.UDSBundle, opts *DeployOptions) error {
					opts.PackageDeployHooks.PreDeploy = func(context.Context, *spec.Package, *layout.PackageLayout, *packager.DeployOptions, *DeployPackageOptions) error {
						return nil
					}
					return nil
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stateReads, specReads, deploys := 0, 0, 0
			opts := DeployOptions{
				Config: newDeployTestConfig(1),
				Resume: true,
				SpecLoader: packageSpecLoaderFunc(func(context.Context, *spec.Package) (*PackageSpec, error) {
					specReads++
					return &PackageSpec{Name: "pkg", Digest: "sha256:one"}, nil
				}),
				DeployedPackagesFn: func(context.Context) ([]state.DeployedPackage, error) { stateReads++; return nil, nil },
				PackageDeployFn:    func(context.Context, *spec.Package, DeployPackageOptions) error { deploys++; return nil },
			}
			tt.install(&opts)

			_, err := NewZarfDeployer(iostreams.IOStreams{}, nil).DeployBundle(t.Context(), bundle, opts)
			require.ErrorIs(t, err, ErrResumePackagePreDeployHook)
			assert.Equal(t, 1, stateReads)
			assert.Equal(t, 1, specReads)
			assert.Zero(t, deploys)
		})
	}
}
