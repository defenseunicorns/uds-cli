// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"errors"
	"fmt"
)

var (
	ErrNotImplemented               = errors.New("not yet implemented")
	ErrPackageNotDeployed           = errors.New("package not deployed")
	ErrInvalidSignatureVerification = errors.New("invalid package signature verification")
	ErrPackageRequired              = errors.New("package is required")
	ErrCreateVerificationMaterial   = errors.New("creating verification material directory")
	ErrWriteVerificationMaterial    = errors.New("writing verification material")
	ErrBundleValidation             = errors.New("bundle validation failed")
	ErrBuildDependencyGraph         = errors.New("failed to build dependency graph")
	ErrComputeDeploymentLevels      = errors.New("failed to compute deployment levels")
	ErrLoadPackage                  = errors.New("loading package")
	ErrIngestPackage                = errors.New("ingesting package")
	ErrDeployPackage                = errors.New("deploying package")
	ErrRemovePackage                = errors.New("removing package")
	ErrPackageHook                  = errors.New("package hook failed")
	ErrBundleHook                   = errors.New("bundle hook failed")
	ErrConnectCluster               = errors.New("connecting to cluster")
	ErrReadDeployedPackages         = errors.New("reading deployed packages")
	ErrResolvePackageManifest       = errors.New("resolving package manifest")
	ErrReadPackageManifest          = errors.New("reading package manifest")
	ErrWritePackageManifest         = errors.New("writing package manifest")
	ErrMarshalPackageManifest       = errors.New("marshaling package manifest")
	ErrCopyPackageContent           = errors.New("copying package content")
	ErrCreatePackageWorkspace       = errors.New("creating package workspace")
	ErrFetchPackageMetadata         = errors.New("fetching package metadata")
	ErrApplyComponentFilter         = errors.New("applying component filter")
	ErrAssemblePackageLayers        = errors.New("assembling package layers")
	ErrCreateOCIRemote              = errors.New("creating OCI remote")
	ErrResolveRootManifest          = errors.New("resolving root manifest")
	ErrFetchRootManifest            = errors.New("fetching root manifest")
	ErrResolvePackageLayers         = errors.New("resolving package layers")
	ErrPullPackage                  = errors.New("pulling package")
	ErrReadCanonicalManifest        = errors.New("reading canonical package manifest")
	ErrStagePackage                 = errors.New("staging package")
	ErrResolveDestinationDirectory  = errors.New("resolving destination directory")
	ErrResolveLayerTitle            = errors.New("resolving layer title")
	ErrCheckLayerTitle              = errors.New("checking layer title")
	ErrLocalSourcePathRequired      = errors.New("local package source path is empty")
	ErrInvalidLocalPackageSource    = errors.New("unsupported local package source")
	ErrUnsupportedPackageSymlink    = errors.New("unsupported symlink in local package")
	ErrCopyLocalPackage             = errors.New("copying local package")
	ErrExtractLocalPackage          = errors.New("extracting local package archive")
	ErrStatLocalPackage             = errors.New("stating local package source")
	ErrInvalidManifestDigest        = errors.New("invalid package manifest digest")
	ErrPackageNotFoundInArtifact    = errors.New("package not found in bundle artifact")
	ErrMissingLayerTitle            = errors.New("package layer is missing its title annotation")
	ErrCreateLayerDirectory         = errors.New("creating package layer directory")
	ErrStagePackageLayer            = errors.New("staging package layer")
	ErrOrchestratedBundleDeploy     = errors.New("orchestrated deployer does not support bundle deployment")
	ErrCreateTemporaryDirectory     = errors.New("creating temporary directory")
	ErrTemplateValues               = errors.New("templating package values")
	ErrParseValues                  = errors.New("parsing package values")
	ErrFlattenVariables             = errors.New("flattening package variables")
	ErrReadValuesFile               = errors.New("reading values file")
	ErrParseValuesTemplate          = errors.New("parsing values template")
	ErrRenderValues                 = errors.New("rendering values file")
	ErrCreateTemporaryValuesFile    = errors.New("creating temporary values file")
	ErrWriteTemporaryValuesFile     = errors.New("writing temporary values file")
	ErrCloseTemporaryValuesFile     = errors.New("closing temporary values file")
	ErrBundleDirRequired            = errors.New("bundle directory is required")
	ErrStatPackageManifest          = errors.New("stating package manifest")
	ErrOpenOCILayout                = errors.New("opening OCI layout")
)

var (
	_ error = (*NilParameterError)(nil)
	_ error = (*LayerPathEscapeError)(nil)
)

type NilParameterError struct{ Name string }

func (e NilParameterError) Error() string {
	return fmt.Sprintf("%s must not be nil", e.Name)
}

type LayerPathEscapeError struct{ Title string }

func (e LayerPathEscapeError) Error() string {
	return fmt.Sprintf("layer title %q escapes destination directory", e.Title)
}
