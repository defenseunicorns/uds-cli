// Copyright 2025 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package tfparser

import (
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// Provider represents a provider requirement in Terraform
type Provider struct {
	Source  string  `json:"source"`
	Version *string `json:"version"`
}

// Packages represents a uds_package resource in Terraform
type Packages struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	OCIUrl string `json:"oci_url"`
	Ref    string `json:"ref,omitempty"`
}

// TerraformConfig represents the root Terraform configuration
type TerraformConfig struct {
	Providers map[string]Provider `json:"required_providers"`
	Packages  []Packages          `json:"uds_packages"`
	Metadata  *BundleMetadata     `json:"uds_bundle_metadata"`
}

// BundleMetadata describes the resource data model.
type BundleMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// Kind reflects the type of package; typicaly always UDSBundle
	Kind string `json:"kind"`
	// these are optional
	Description  *string `json:"description"`
	URL          *string `json:"url"`
	Architecture *string `json:"architecture"`
}

// ParseFile reads and parses a Terraform file, returning the structured configuration
func ParseFile(filename string) (*TerraformConfig, error) {
	parser := hclparse.NewParser()

	f, diags := parser.ParseHCLFile(filename)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing error: %s", diags.Error())
	}

	config := &TerraformConfig{
		Providers: make(map[string]Provider),
	}

	content, diags := f.Body.Content(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "terraform"},
			{Type: "provider", LabelNames: []string{"name"}},
			{Type: "resource", LabelNames: []string{"type", "name"}},
		},
	})
	if diags.HasErrors() {
		return nil, fmt.Errorf("content error: %s", diags.Error())
	}

	// Parse blocks
	for _, block := range content.Blocks {
		switch block.Type {
		case "terraform":
			if err := parseTerraformBlock(block, config); err != nil {
				return nil, err
			}
		case "resource":
			if len(block.Labels) == 2 && block.Labels[0] == "uds_package" {
				pkg, err := parseUDSPackageBlock(block)
				if err != nil {
					return nil, err
				}
				config.Packages = append(config.Packages, *pkg)
			}

			if len(block.Labels) == 2 && block.Labels[0] == "uds_bundle_metadata" {
				meta, err := parseUDSBundleMetadataBlock(block)
				if err != nil {
					return nil, err
				}
				config.Metadata = meta
			}
		}
	}

	return config, nil
}

// parseTerraformBlock parses the terraform block in the given hcl.Block. At
// this time we only care about the required_providers block inside. Also at
// this time we'll parse out the entire required_providers list, not just the
// UDS provider, though we may not actually use the other providers or pull them
// down to be included in the bundle.
func parseTerraformBlock(block *hcl.Block, config *TerraformConfig) error {
	content, diags := block.Body.Content(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "required_providers"},
		},
	})
	if diags.HasErrors() {
		return fmt.Errorf("terraform block error: %s", diags.Error())
	}

	providers := content.Blocks.OfType("required_providers")
	for _, p := range providers {
		attrs, diags := p.Body.JustAttributes()
		if diags.HasErrors() {
			return fmt.Errorf("required_providers error: %s", diags.Error())
		}

		for key, attr := range attrs {
			value, diags := attr.Expr.Value(&hcl.EvalContext{})
			if diags.HasErrors() {
				return fmt.Errorf("attribute error: %s", diags.Error())
			}

			if value.Type().IsObjectType() {
				provider := Provider{}
				if value.Type().HasAttribute("source") {
					if v := value.GetAttr("source"); v.Type() == cty.String {
						provider.Source = v.AsString()
					}
				}

				if value.Type().HasAttribute("version") {
					if v := value.GetAttr("version"); v.Type() == cty.String {
						provider.Version = stringPtr(v.AsString())
					}
				}

				if provider.Source == "" {
					return errors.New("provider source is required")
				}
				config.Providers[key] = provider
			}
		}
	}
	return nil
}

// parseUDSPackageBlock parses the uds_package block in the given hcl.Block. At
// this time we only care about the oci_url and ref attributes, so we can pull
// the sources for inclusion in the bundle.
func parseUDSPackageBlock(block *hcl.Block) (*Packages, error) {
	// labels are in the resource "title", ex:
	// resource "uds_package" "zarf_init" {}
	pkg := &Packages{
		Type: block.Labels[0], // "uds_package"
		Name: block.Labels[1], // "zarf_init"
	}

	attrs, diags := block.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, fmt.Errorf("uds_package block error: %s", diags.Error())
	}

	ctx := &hcl.EvalContext{}

	if attr, exists := attrs["oci_url"]; exists {
		value, diags := attr.Expr.Value(ctx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("oci_url error: %s", diags.Error())
		}
		pkg.OCIUrl = value.AsString()
	}

	if attr, exists := attrs["ref"]; exists {
		value, diags := attr.Expr.Value(ctx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("ref error: %s", diags.Error())
		}
		pkg.Ref = value.AsString()
	}

	return pkg, nil
}

// parseUDSBundleMetadataBlock parses the uds_block in the given hcl.Block. At
// this time Name, Kind, and Version are required, and all other fields are
// optional.
func parseUDSBundleMetadataBlock(block *hcl.Block) (*BundleMetadata, error) {
	// labels are in the resource "title", ex:
	// resource "uds_bundle_metadata" "core_slim_dev" {}
	metadata := &BundleMetadata{
		Name: block.Labels[1], // "core_slim_dev"
	}

	attrs, diags := block.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, fmt.Errorf("uds_bundle_metadata block error: %s", diags.Error())
	}

	ctx := &hcl.EvalContext{}
	if attr, exists := attrs["version"]; exists {
		value, diags := attr.Expr.Value(ctx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("version error: %s", diags.Error())
		}
		metadata.Version = value.AsString()
	}

	if attr, exists := attrs["kind"]; exists {
		value, diags := attr.Expr.Value(ctx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("ref error: %s", diags.Error())
		}
		metadata.Kind = value.AsString()
	}

	if attr, exists := attrs["description"]; exists {
		value, diags := attr.Expr.Value(ctx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("ref error: %s", diags.Error())
		}
		metadata.Description = stringPtr(value.AsString())
	}

	if attr, exists := attrs["url"]; exists {
		value, diags := attr.Expr.Value(ctx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("ref error: %s", diags.Error())
		}
		str := value.AsString()
		metadata.URL = &str
	}

	if attr, exists := attrs["architecture"]; exists {
		value, diags := attr.Expr.Value(ctx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("ref error: %s", diags.Error())
		}
		str := value.AsString()
		metadata.Architecture = &str
	}

	// validate that we have the required fields set
	if metadata.Kind == "" {
		return nil, fmt.Errorf("uds_bundle_metadata kind is required")
	}
	if metadata.Version == "" {
		return nil, fmt.Errorf("uds_bundle_metadata version is required")
	}
	if metadata.Name == "" {
		return nil, fmt.Errorf("uds_bundle_metadata name is required")
	}

	return metadata, nil
}

// stringToPtr is a convienence method to convert a string to a *string
func stringPtr(s string) *string {
	return &s
}
