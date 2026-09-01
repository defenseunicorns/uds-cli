// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"errors"
	"fmt"
	"os"

	"github.com/defenseunicorns/pkg/helpers/v2"
	goyaml "github.com/goccy/go-yaml"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/api/v1beta1"
)

func writeV1beta1Definition(path string, definition v1beta1.Package) error {
	contents, err := goyaml.Marshal(definition)
	if err != nil {
		return fmt.Errorf("marshaling v1beta1 package definition: %w", err)
	}
	return os.WriteFile(path, contents, helpers.ReadWriteUser)
}

func localizedV1beta1Definition(beta v1beta1.Package, alpha v1alpha1.ZarfPackage) (v1beta1.Package, error) {
	if len(beta.Components) != len(alpha.Components) {
		return v1beta1.Package{}, fmt.Errorf("converted package component count changed from %d to %d", len(beta.Components), len(alpha.Components))
	}
	beta.Metadata.Version = alpha.Metadata.Version
	beta.Build = v1beta1.BuildData{Migrations: beta.Build.Migrations}
	beta.Values.Files = alpha.Values.Files
	beta.Values.Schema = alpha.Values.Schema
	beta.Documentation = alpha.Documentation

	for idx := range beta.Components {
		if err := applyLocalizedComponent(&beta.Components[idx], alpha.Components[idx]); err != nil {
			return v1beta1.Package{}, fmt.Errorf("localizing v1beta1 component %q: %w", beta.Components[idx].Name, err)
		}
	}
	return beta, nil
}

func applyLocalizedComponent(beta *v1beta1.Component, alpha v1alpha1.ZarfComponent) error {
	if beta.Name != alpha.Name {
		return fmt.Errorf("converted component name changed to %q", alpha.Name)
	}
	if len(beta.Files) != len(alpha.Files) || len(beta.Manifests) != len(alpha.Manifests) || len(beta.Charts) != len(alpha.Charts) || len(beta.Repositories) != len(alpha.Repos) {
		return errors.New("converted component resource counts changed")
	}

	beta.Actions.OnCreate = v1beta1.ComponentActionSet{}
	beta.Selector.Flavor = ""
	for idx := range beta.Files {
		beta.Files[idx].Source = alpha.Files[idx].Source
		beta.Files[idx].ExtractPath = ""
	}
	for idx := range beta.Manifests {
		beta.Manifests[idx].Files = alpha.Manifests[idx].Files
		if len(alpha.Manifests[idx].Kustomizations) == 0 {
			beta.Manifests[idx].Kustomize = nil
		} else {
			beta.Manifests[idx].Kustomize = &v1beta1.KustomizeManifest{Files: alpha.Manifests[idx].Kustomizations}
		}
	}
	for idx := range beta.Charts {
		localized := alpha.Charts[idx]
		beta.Charts[idx].HelmRepository = nil
		beta.Charts[idx].Git = nil
		beta.Charts[idx].OCI = nil
		beta.Charts[idx].Local = &v1beta1.LocalSource{Path: localized.LocalPath}
		beta.Charts[idx].ValuesFiles = make([]v1beta1.ValuesFile, 0, len(localized.ValuesFiles)+len(localized.TemplatedValuesFiles))
		for _, path := range localized.ValuesFiles {
			beta.Charts[idx].ValuesFiles = append(beta.Charts[idx].ValuesFiles, v1beta1.ValuesFile{Path: path})
		}
		for _, path := range localized.TemplatedValuesFiles {
			beta.Charts[idx].ValuesFiles = append(beta.Charts[idx].ValuesFiles, v1beta1.ValuesFile{Path: path, EnableTemplating: true})
		}
	}
	for idx := range beta.Repositories {
		beta.Repositories[idx].URL = alpha.Repos[idx]
		beta.Repositories[idx].Ref = nil
	}
	beta.Images = nil
	beta.ImageArchives = make([]v1beta1.ImageArchive, len(alpha.ImageArchives))
	for idx, archive := range alpha.ImageArchives {
		beta.ImageArchives[idx] = v1beta1.ImageArchive{Path: archive.Path, Images: archive.Images}
	}
	return nil
}
