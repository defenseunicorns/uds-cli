// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package disassemble

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/transform"
)

func localizeRepos(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, finalDir, tmpRoot string, component *api.Component) error {
	repoDir, err := pkgLayout.GetComponentDir(ctx, tmpRoot, component.Name, layout.RepoComponentDir)
	if err != nil {
		return fmt.Errorf("reading repository assets for component %s: %w", component.Name, err)
	}
	for idx := range component.Repositories {
		repository := &component.Repositories[idx]
		ref, err := repositoryLayoutReference(*repository)
		if err != nil {
			return fmt.Errorf("resolving repository %q: %w", repository.URL, err)
		}
		repoPath, err := findRepoPath(repoDir, ref)
		if err != nil {
			return err
		}
		rel := filepath.Join("repos", fmt.Sprintf("%d-%s", idx, filepath.Base(repoPath)))
		if err := helpers.CreatePathAndCopy(repoPath, filepath.Join(outputDir, rel)); err != nil {
			return fmt.Errorf("copying repository %q: %w", ref, err)
		}
		// Zarf currently requires a URL-shaped repo source and does not resolve it
		// against the package directory, so use the final local path explicitly.
		// TODO: (@wstarr) - this should be addressed upstream so that local repos can be better handled
		localizedURL := fileURL(filepath.Join(finalDir, componentSourcePath(component.Name, rel)))
		repository.URL = localizedURL
		repository.Ref = nil
		repository.LegacyURL = localizedURL
	}
	return nil
}

func repositoryLayoutReference(repository api.Repository) (string, error) {
	if repository.LegacyURL != "" {
		return repository.LegacyURL, nil
	}
	if repository.Ref == nil {
		return repository.URL, nil
	}
	base, _, err := transform.GitURLSplitRef(repository.URL)
	if err != nil {
		return "", err
	}
	switch {
	case repository.Ref.Tag != "":
		return base + "@" + repository.Ref.Tag, nil
	case repository.Ref.Branch != "":
		return base + "@refs/heads/" + repository.Ref.Branch, nil
	case repository.Ref.Commit != "":
		return base + "@" + repository.Ref.Commit, nil
	default:
		return base, nil
	}
}

func fileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func findRepoPath(repoDir, ref string) (string, error) {
	name, err := transform.GitURLtoFolderName(ref)
	if err != nil {
		return "", fmt.Errorf("mapping repository %q to packaged content: %w", ref, err)
	}
	path := filepath.Join(repoDir, name)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("checking packaged repository %q: %w", ref, err)
	}
	return "", fmt.Errorf("unable to map repository %q to packaged content; tried %s", ref, name)
}
