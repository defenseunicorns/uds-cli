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

// UDSMetadata lists information about the current UDS Bundle.
type UDSMetadata struct {
	Name              string `json:"name" jsonschema:"description=Name to identify this Zarf package,pattern=^[a-z0-9\\-]+$"`
	Description       string `json:"description,omitempty" jsonschema:"description=Additional information about this package"`
	Version           string `json:"version,omitempty" jsonschema:"description=Generic string set by a package author to track the package version"`
	URL               string `json:"url,omitempty" jsonschema:"description=Link to package information when online"`
	Uncompressed      bool   `json:"uncompressed,omitempty" jsonschema:"description=Disable compression of this package"`
	Architecture      string `json:"architecture,omitempty" jsonschema:"description=The target cluster architecture for this package,example=arm64,example=amd64"`
	Authors           string `json:"authors,omitempty" jsonschema:"description=Comma-separated list of package authors (including contact info),example=Doug &#60;hello@defenseunicorns.com&#62;&#44; Pepr &#60;hello@defenseunicorns.com&#62;"`
	Documentation     string `json:"documentation,omitempty" jsonschema:"description=Link to package documentation when online"`
	Source            string `json:"source,omitempty" jsonschema:"description=Link to package source code when online"`
	Vendor            string `json:"vendor,omitempty" jsonschema_description:"Name of the distributing entity, organization or individual."`
	AggregateChecksum string `json:"aggregateChecksum,omitempty" jsonschema:"description=Checksum of a checksums.txt file that contains checksums all the layers within the package."`
}

// UDSBuildData is written during the bundle.Create() operation to track details of the created package.
type UDSBuildData struct {
	Terminal     string `json:"terminal" jsonschema:"description=The machine name that created this package"`
	User         string `json:"user" jsonschema:"description=The username who created this package"`
	Architecture string `json:"architecture" jsonschema:"description=The architecture this package was created on"`
	Timestamp    string `json:"timestamp" jsonschema:"description=The timestamp when this package was created"`
	Version      string `json:"version" jsonschema:"description=The version of Zarf used to build this package"`
}
