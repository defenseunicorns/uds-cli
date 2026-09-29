// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/state"
)

// PackageSpec is the identity Zarf records for a package deployment.
type PackageSpec struct {
	Name, Digest string
	Components   []string
}

// PackageSpecLoader loads a filtered package definition without deploying it.
type PackageSpecLoader interface {
	LoadPackageSpec(context.Context, *spec.Package) (*PackageSpec, error)
}

// DeployedPackagesFn reads the Zarf deployment state once for a resume operation.
type DeployedPackagesFn func(context.Context) ([]state.DeployedPackage, error)

func filterResumeLevels(ctx context.Context, levels [][]*spec.Package, opts DeployOptions, streams iostreams.IOStreams) ([][]*spec.Package, error) {
	if opts.SpecLoader == nil {
		return nil, fmt.Errorf("resume requires a package spec loader")
	}
	deployedPackages := opts.DeployedPackagesFn
	if deployedPackages == nil {
		deployedPackages = getDeployedPackages
	}
	streams.Info("filtering deployed packages for resume")
	deployed, err := deployedPackages(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w for resume: %w", ErrReadDeployedPackages, err)
	}
	byIdentity := make(map[deployedPackageIdentity][]state.DeployedPackage, len(deployed))
	for _, deployedPackage := range deployed {
		identity := deployedPackageIdentity{name: deployedPackage.Name, namespace: deployedPackage.NamespaceOverride}
		byIdentity[identity] = append(byIdentity[identity], deployedPackage)
	}
	streams.Debug("loaded deployed packages for resume", "records", len(deployed))

	filtered := make([][]*spec.Package, 0, len(levels))
	for _, level := range levels {
		kept := make([]*spec.Package, 0, len(level))
		for _, pkg := range level {
			intended, err := opts.SpecLoader.LoadPackageSpec(ctx, pkg)
			if err != nil {
				return nil, fmt.Errorf("loading intended package %q for resume: %w", pkg.Name, err)
			}
			if intended == nil {
				return nil, fmt.Errorf("loading intended package %q for resume: loader returned no package", pkg.Name)
			}
			if intended.Name == "" {
				return nil, fmt.Errorf("loading intended package %q for resume: name is empty", pkg.Name)
			}
			if intended.Digest == "" {
				return nil, fmt.Errorf("loading intended package %q for resume: digest is empty", pkg.Name)
			}
			if resumeMatches(intended, byIdentity[deployedPackageIdentity{name: intended.Name, namespace: pkg.Namespace}]) {
				streams.Info("skipping previously deployed package", "name", pkg.Name)
				continue
			}
			kept = append(kept, pkg)
		}
		if len(kept) > 0 {
			filtered = append(filtered, kept)
		}
	}
	if len(filtered) == 0 {
		streams.Info("all selected packages are already deployed")
	}
	return filtered, nil
}

type deployedPackageIdentity struct {
	name      string
	namespace string
}

func resumeMatches(intended *PackageSpec, deployed []state.DeployedPackage) bool {
	if intended == nil || intended.Digest == "" || len(deployed) != 1 || deployed[0].Digest == "" || deployed[0].Digest != intended.Digest || len(intended.Components) != len(deployed[0].DeployedComponents) {
		return false
	}
	components := make(map[string]struct{}, len(intended.Components))
	for _, name := range intended.Components {
		if name == "" {
			return false
		}
		if _, duplicate := components[name]; duplicate {
			return false
		}
		components[name] = struct{}{}
	}
	for _, component := range deployed[0].DeployedComponents {
		if component.Name == "" || component.Status != state.ComponentStatusSucceeded {
			return false
		}
		if _, ok := components[component.Name]; !ok {
			return false
		}
		delete(components, component.Name)
	}
	return len(components) == 0
}

func getDeployedPackages(ctx context.Context) ([]state.DeployedPackage, error) {
	c, err := cluster.New(ctx)
	if err != nil {
		return nil, err
	}
	return c.GetDeployedZarfPackages(ctx)
}
