// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package types contains all the types used by UDS.
package types

// UDSBundle is the top-level structure of a UDS bundle
type UDSBundle struct {
	Kind         string              `json:"kind" jsonschema:"description=The kind of UDS package,enum=UDSBundle"`
	Metadata     UDSMetadata         `json:"metadata" jsonschema:"description=UDSBundle metadata"`
	Build        UDSBuildData        `json:"build,omitempty" jsonschema:"description=Generated bundle build data"`
	ZarfPackages []BundleZarfPackage `json:"zarf-packages" jsonschema:"description=List of Zarf packages"`
}

// BundleZarfPackage represents a Zarf package in a UDS bundle
type BundleZarfPackage struct {
	Name               string           `json:"name" jsonschema:"name=Name of the Zarf package"`
	Repository         string           `json:"repository,omitempty" jsonschema:"description=The repository to import the package from"`
	Path               string           `json:"path,omitempty" jsonschema:"description=The local path to import the package from"`
	Ref                string           `json:"ref" jsonschema:"description=Ref (tag) of the Zarf package"`
	OptionalComponents []string         `json:"optional-components,omitempty" jsonschema:"description=List of optional components to include from the package (required components are always included)"`
	PublicKey          string           `json:"public-key,omitempty" jsonschema:"description=The public key to use to verify the package"`
	Imports            []BundleVariable `json:"imports,omitempty" jsonschema:"description=List of Zarf variables to import from another Zarf package"`
	Exports            []BundleVariable `json:"exports,omitempty" jsonschema:"description=List of Zarf variables to export from the Zarf package"`
}

// BundleVariable represents variables in the bundle
type BundleVariable struct {
	Name        string `json:"name" jsonschema:"name=Name of the variable"`
	Package     string `json:"package" jsonschema:"name=Name of the Zarf package to get the variable from"`
	Description string `json:"description" jsonschema:"name=Description of the variable"`
}
