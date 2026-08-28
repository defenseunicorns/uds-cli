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
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/transform"
)

func localizeRepos(ctx context.Context, pkgLayout *layout.PackageLayout, outputDir, finalDir, tmpRoot string, component *v1alpha1.ZarfComponent) error {
	repoDir, err := pkgLayout.GetComponentDir(ctx, tmpRoot, component.Name, layout.RepoComponentDir)
	if err != nil {
		return fmt.Errorf("reading repository assets for component %s: %w", component.Name, err)
	}
	for idx, ref := range component.Repos {
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
		component.Repos[idx] = fileURL(filepath.Join(finalDir, component.Name, rel))
	}
	return nil
}

func fileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func findRepoPath(repoDir, ref string) (string, error) {
	modern, modernErr := transform.GitURLtoFolderName(ref)
	legacy, legacyErr := transform.GitURLtoRepoName(ref)
	for _, name := range []string{modern, legacy} {
		if name == "" {
			continue
		}
		path := filepath.Join(repoDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("checking packaged repository %q: %w", ref, err)
		}
	}
	return "", fmt.Errorf("unable to map repository %q to packaged content (modern: %w, legacy: %w)", ref, modernErr, legacyErr)
}
