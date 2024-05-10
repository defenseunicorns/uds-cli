// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package bundle contains functions for interacting with, managing and deploying UDS packages
package bundle

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/utils"
	zarfUtils "github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"gopkg.in/yaml.v2"
)

func (b *Bundle) extractImagesFromPackages() {
	for i, pkg := range b.bundle.Packages {
		if pkg.Repository != "" && pkg.Ref != "" {
			message.Debugf("Package: %s, Repository: %s, Ref: %s", pkg.Name, pkg.Repository, pkg.Ref)
			type Component struct {
				Images []string `yaml:"images"`
			}

			type Output struct {
				Components []Component `yaml:"components"`
			}

			cmd := exec.Command("uds", "zarf", "package", "inspect", "oci://"+pkg.Repository+":"+pkg.Ref, "--no-color", "--no-log-file")

			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output

			err := cmd.Run() // Use Run instead of CombinedOutput
			if err != nil {
				message.Fatalf("Error executing command: %s, output: %s", err.Error(), output.String())
				continue
			}
			outputStr := output.String()

			// Find the index of "kind:"
			idx := strings.Index(outputStr, "kind:")
			if idx == -1 {
				message.Fatalf("Invalid output: %s", outputStr)
				continue
			}

			// Trim the output
			trimmedOutput := outputStr[idx:]

			message.Debugf("Trimmed Output: %s", trimmedOutput)

			var result interface{}
			err = yaml.Unmarshal([]byte(trimmedOutput), &result)
			if err != nil {
				message.Fatalf("Error unmarshaling YAML: %s", err.Error())
				continue
			}

			var allImages []string
			findImages(result, &allImages)

			// Add the images to the package
			if len(allImages) > 0 {
				message.Debugf("Images: %v", allImages)
				b.bundle.Packages[i].Images = allImages
			} else {
				message.Debugf("No images found in package %v", pkg.Name)
			}
		}
	}
}

// Inspect pulls/unpacks a bundle's metadata and shows it
func (b *Bundle) Inspect() error {
	// Check if the source is a YAML file
	if filepath.Ext(b.cfg.InspectOpts.Source) == ".yaml" {
		source, err := CheckYAMLSourcePath(b.cfg.InspectOpts.Source)
		if err != nil {
			return err
		}
		b.cfg.InspectOpts.Source = source
		return b.InspectYAML(b.cfg.InspectOpts.Source)
	}

	// Check that provided oci source path is valid, and update it if it's missing the full path
	source, err := CheckOCISourcePath(b.cfg.InspectOpts.Source)
	if err != nil {
		return err
	}
	b.cfg.InspectOpts.Source = source

	// create a new provider
	provider, err := NewBundleProvider(b.cfg.InspectOpts.Source, b.tmp)
	if err != nil {
		return err
	}

	// pull the bundle's metadata + sig + sboms (optional)
	loaded, err := provider.LoadBundleMetadata()
	if err != nil {
		return err
	}

	// validate the sig (if present)
	if err := ValidateBundleSignature(loaded[config.BundleYAML], loaded[config.BundleYAMLSignature], b.cfg.InspectOpts.PublicKeyPath); err != nil {
		return err
	}

	// pull sbom
	if b.cfg.InspectOpts.IncludeSBOM {
		err := provider.CreateBundleSBOM(b.cfg.InspectOpts.ExtractSBOM)
		if err != nil {
			return err
		}
	}
	// read the bundle's metadata into memory
	if err := utils.ReadYAMLStrict(loaded[config.BundleYAML], &b.bundle); err != nil {
		return err
	}
	// If ExtractImages flag is set, extract images from each package
	if b.cfg.InspectOpts.ExtractImages {
		message.Debugf("Extracting images from packages")
		b.extractImagesFromPackages()
	} else {
		message.Debugf("Skipping image extraction")
	}

	// show the bundle's metadata
	zarfUtils.ColorPrintYAML(b.bundle, nil, false)

	// TODO: showing package metadata?
	// TODO: could be cool to have an interactive mode that lets you select a package and show its metadata
	return nil
}

// InspectYAML inspects a bundle from a YAML file
func (b *Bundle) InspectYAML(yamlPath string) error {
	message.Debugf("Reading the yaml file: %s", yamlPath)
	// Read the YAML file
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return err
	}

	// Unmarshal the YAML data into the Bundle struct
	err = yaml.Unmarshal(data, &b.bundle)
	if err != nil {
		return err
	}

	// If ExtractImages flag is set, extract images from each package
	if b.cfg.InspectOpts.ExtractImages {
		b.extractImagesFromPackages()
	}

	// show the bundle's metadata
	utils.ColorPrintYAML(b.bundle, nil, false)
	return nil
}

func findImages(node interface{}, images *[]string) {
	switch node := node.(type) {
	case map[interface{}]interface{}:
		for k, v := range node {
			if k == "images" {
				// Check if v is a slice of interfaces
				if imgSlice, ok := v.([]interface{}); ok {
					// Convert each element to a string and append it to images
					for _, img := range imgSlice {
						if imgStr, ok := img.(string); ok {
							*images = append(*images, imgStr)
						}
					}
				}
			} else {
				findImages(v, images)
			}
		}
	case []interface{}:
		for _, v := range node {
			findImages(v, images)
		}
	}
}
