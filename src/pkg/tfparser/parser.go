package tfparser

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// Provider represents a provider requirement in Terraform
type Provider struct {
	Source  string `json:"source"`
	Version string `json:"version"`
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
		Packages:  make([]Packages, 0),
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
				if v := value.GetAttr("source"); v.Type() == cty.String {
					provider.Source = v.AsString()
				}
				if v := value.GetAttr("version"); v.Type() == cty.String {
					provider.Version = v.AsString()
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
