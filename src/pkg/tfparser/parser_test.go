package tfparser

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseTerraformFile(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.tf")

	testCases := []struct {
		name     string
		content  string
		expected *TerraformConfig
		wantErr  bool
	}{
		{
			name: "valid configuration",
			content: `
terraform {
  required_providers {
    uds = {
      source = "defenseunicorns/uds"
      version = "0.1.0"
    }
  }
}

provider "uds" {}

resource "uds_package" "init" {
  oci_url = "ghcr.io/zarf-dev/packages/init@v0.45.0"
  ref = "v0.45.0"
}

resource "uds_package" "prometheus" {
  oci_url = "localhost:888/prometheus@v0.1.0"
  depends_on = [
    uds_package.init
  ]
}`,

			expected: &TerraformConfig{
				Providers: map[string]Provider{
					"uds": {
						Source:  "defenseunicorns/uds",
						Version: "0.1.0",
					},
				},
				Packages: []Packages{
					{
						Name:   "init",
						OCIUrl: "ghcr.io/zarf-dev/packages/init@v0.45.0",
						Ref:    "v0.45.0",
						Type:   "uds_package",
					},
					{
						Name:   "prometheus",
						OCIUrl: "localhost:888/prometheus@v0.1.0",
						Type:   "uds_package",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid HCL syntax",
			content: `
terraform {
  required_providers {
    uds = {
      source = "defenseunicorns/uds"
      version = "0.1.0"
    }
  }
  }
} # Extra closing brace
`,
			expected: nil,
			wantErr:  true,
		},
		{
			// for now we ignore only uds_package resources with regards to the
			// things we want to eventually download and write in to the bundle
			// OCI image.
			name: "ignores non-uds_package resources",
			content: `
terraform {
  required_providers {
    uds = {
      source = "defenseunicorns/uds"
      version = "0.1.0"
    }
    aws = {
      source = "hashicorp/aws"
      version = "4.0.0"
    }
  }
}

provider "uds" {}

resource "uds_package" "init" {
  oci_url = "ghcr.io/zarf-dev/packages/init@v0.45.0"
  ref = "v0.45.0"
}

resource "aws_instance" "test" {
  // some things
}`,

			expected: &TerraformConfig{
				Providers: map[string]Provider{
					"uds": {
						Source:  "defenseunicorns/uds",
						Version: "0.1.0",
					},
					"aws": {
						Source:  "hashicorp/aws",
						Version: "4.0.0",
					},
				},
				Packages: []Packages{
					{
						Name:   "init",
						OCIUrl: "ghcr.io/zarf-dev/packages/init@v0.45.0",
						Ref:    "v0.45.0",
						Type:   "uds_package",
					},
				},
			},
			wantErr: false,
		},
		{
			// support for multiple providers is okay at the moment, but we may
			// change this to be an allow-list in the future
			name: "multiple providers",
			content: `
terraform {
  required_providers {
    uds = {
      source = "defenseunicorns/uds"
      version = "0.1.0"
    }
    aws = {
      source = "hashicorp/aws"
      version = "4.0.0"
    }
  }
}`,
			expected: &TerraformConfig{
				Providers: map[string]Provider{
					"uds": {
						Source:  "defenseunicorns/uds",
						Version: "0.1.0",
					},
					"aws": {
						Source:  "hashicorp/aws",
						Version: "4.0.0",
					},
				},
				Packages: []Packages{},
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Write test content to temporary file
			err := os.WriteFile(testFile, []byte(tc.content), 0644)
			if err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			// Parse the file
			got, err := ParseFile(testFile)

			// Check error expectations
			if (err != nil) != tc.wantErr {
				t.Errorf("ParseTerraformFile() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if tc.wantErr {
				return
			}

			// Compare results
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("ParseTerraformFile() got = %v, want %v", got, tc.expected)
			}
		})
	}
}
