// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"gopkg.in/yaml.v3"
)

// Zarf OCI layer annotation key for the file path within the package.
// Zarf stores each package file as a raw-bytes OCI layer, with this annotation
// set to the file's relative path (e.g. "zarf.yaml", "components/my-pkg.tar").
const zarfLayerTitleAnnotation = "org.opencontainers.image.title"

// zarfMinimalPackage holds only the component metadata needed for optional-component filtering.
type zarfMinimalPackage struct {
	Components []zarfMinimalComponent `yaml:"components"`
}

// zarfMinimalComponent holds a component's name and required flag.
type zarfMinimalComponent struct {
	Name     string `yaml:"name"`
	Required *bool  `yaml:"required,omitempty"`
}

// zarfMetadata holds the minimal metadata we extract from zarf.yaml for OCI config labels.
type zarfMetadata struct {
	Metadata struct {
		Name        string `yaml:"name"`
		Version     string `yaml:"version"`
		Description string `yaml:"description"`
	} `yaml:"metadata"`
}

// filterZarfOptionalComponents rewrites the image manifest blob to exclude layers
// belonging to optional Zarf components not listed in keepComponents.
//
// When keepComponents is empty ALL optional components are excluded (only required
// components are retained). Non-Zarf manifests (no zarf.yaml layer) are returned
// unchanged. It also validates that every name in keepComponents is an actual
// optional (non-required) component defined in the package's zarf.yaml.
func filterZarfOptionalComponents(blobDir string, m ociManifest, keepComponents []string) (ociManifest, error) {
	mh, err := v1.NewHash(m.Digest)
	if err != nil {
		return ociManifest{}, err
	}

	manifestBytes, err := os.ReadFile(filepath.Join(blobDir, mh.Hex))
	if err != nil {
		return ociManifest{}, fmt.Errorf("reading manifest blob: %w", err)
	}

	var im ociImageManifest
	if err := json.Unmarshal(manifestBytes, &im); err != nil {
		return ociManifest{}, fmt.Errorf("parsing image manifest: %w", err)
	}

	pkg, err := readZarfPackageFromBlobs(blobDir, im.Layers)
	if err != nil {
		return ociManifest{}, err
	}
	if pkg == nil {
		// Not a Zarf package (no zarf.yaml layer); pass through unchanged.
		return m, nil
	}

	// Determine which components are optional (required == nil or false).
	optionalNames := make(map[string]bool, len(pkg.Components))
	for _, c := range pkg.Components {
		if c.Required == nil || !*c.Required {
			optionalNames[c.Name] = true
		}
	}

	// Validate: every entry in keepComponents must name an optional component.
	for _, name := range keepComponents {
		if optionalNames[name] {
			continue
		}
		// Check whether it exists at all (might be required or unknown).
		for _, c := range pkg.Components {
			if c.Name == name {
				return ociManifest{}, fmt.Errorf("component %q is required and cannot be listed in optional_components", name)
			}
		}
		return ociManifest{}, fmt.Errorf("component %q does not exist in this package", name)
	}

	keepSet := make(map[string]bool, len(keepComponents))
	for _, name := range keepComponents {
		keepSet[name] = true
	}

	// Filter layers: drop optional component layers not in keepSet.
	filtered := make([]ociDescriptor, 0, len(im.Layers))
	for _, l := range im.Layers {
		compName, isComp := zarfComponentNameFromTitle(l.Annotations[zarfLayerTitleAnnotation])
		if isComp && optionalNames[compName] && !keepSet[compName] {
			continue
		}
		filtered = append(filtered, l)
	}

	if len(filtered) == len(im.Layers) {
		return m, nil
	}

	im.Layers = filtered
	newManifestBytes, err := json.Marshal(im)
	if err != nil {
		return ociManifest{}, fmt.Errorf("marshalling filtered manifest: %w", err)
	}

	sum := sha256.Sum256(newManifestBytes)
	newHash := v1.Hash{Algorithm: "sha256", Hex: hex.EncodeToString(sum[:])}
	if err := writeBlobBytesIfMissingAndVerify(blobDir, newHash, newManifestBytes); err != nil {
		return ociManifest{}, fmt.Errorf("writing filtered manifest blob: %w", err)
	}

	return ociManifest{
		MediaType:   m.MediaType,
		Digest:      newHash.String(),
		Size:        int64(len(newManifestBytes)),
		Platform:    m.Platform,
		Annotations: m.Annotations,
	}, nil
}

// readZarfPackageFromBlobs finds the zarf.yaml layer among layers, reads its blob
// from blobDir (raw YAML bytes, as stored by Zarf via ORAS file.Store), and returns
// the parsed minimal package definition.
func readZarfPackageFromBlobs(blobDir string, layers []ociDescriptor) (*zarfMinimalPackage, error) {
	for _, l := range layers {
		if l.Annotations[zarfLayerTitleAnnotation] != "zarf.yaml" {
			continue
		}
		h, err := v1.NewHash(l.Digest)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(filepath.Join(blobDir, h.Hex))
		if err != nil {
			return nil, fmt.Errorf("reading zarf.yaml blob: %w", err)
		}
		var pkg zarfMinimalPackage
		if err := yaml.Unmarshal(data, &pkg); err != nil {
			return nil, fmt.Errorf("parsing zarf.yaml: %w", err)
		}
		return &pkg, nil
	}
	return nil, nil // no zarf.yaml found; not a Zarf package
}

// zarfComponentNameFromTitle extracts the Zarf component name from a layer title.
// Zarf stores component archives as "components/<name>.tar"; returns ("", false)
// for any other title.
func zarfComponentNameFromTitle(title string) (string, bool) {
	name, ok := strings.CutPrefix(title, "components/")
	if !ok || !strings.HasSuffix(name, ".tar") || name == ".tar" {
		return "", false
	}
	return strings.TrimSuffix(name, ".tar"), true
}

// readZarfMetadata reads and parses metadata from a zarf.yaml file.
// Returns an empty zarfMetadata if the file doesn't exist or cannot be parsed.
func readZarfMetadata(zarfYamlPath string) zarfMetadata {
	var meta zarfMetadata
	if data, err := os.ReadFile(zarfYamlPath); err == nil {
		_ = yaml.Unmarshal(data, &meta)
	}
	return meta
}
