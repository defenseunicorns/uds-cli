// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package types contains all the types used by UDS.
package types

// UDSBundle is the top-level structure of a UDS package file.
type UDSBundle struct {
	Kind         string              `json:"kind" jsonschema:"description=The kind of UDS package,enum=UDSBundle"`
	Metadata     UDSMetadata         `json:"metadata" jsonschema:"description=UDSBundle metadata"`
	Build        UDSBuildData        `json:"build,omitempty" jsonschema:"description=Generated bundle build data"`
	ZarfPackages []ZarfPackageImport `json:"zarf-packages" jsonschema:"description=List of Zarf packages"`
}

// ZarfPackageImport is a Zarf package import statement in a UDS package file.
type ZarfPackageImport struct {
	Repository         string   `json:"repository" jsonschema:"description=The repository to import the package from"`
	Ref                string   `json:"ref"`
	OptionalComponents []string `json:"optional-components,omitempty" jsonschema:"description=List of optional components to include from the package (required components are always included)"`
	PublicKey          string   `json:"public-key,omitempty" jsonschema:"description=The public key to use to verify the package"`
}
