// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package types contains all the types used by UDS.
package types

// UDSMetadata lists information about the current ZarfPackage.
type UDSMetadata struct {
	Name              string `json:"name" jsonschema:"description=Name to identify this Zarf package,pattern=^[a-z0-9\\-]+$"`
	Description       string `json:"description,omitempty" jsonschema:"description=Additional information about this package"`
	Version           string `json:"version,omitempty" jsonschema:"description=Generic string set by a package author to track the package version (Note: ZarfInitConfigs will always be versioned to the CLIVersion they were created with)"`
	URL               string `json:"url,omitempty" jsonschema:"description=Link to package information when online"`
	Image             string `json:"image,omitempty" jsonschema:"description=An image URL to embed in this package (Reserved for future use in Zarf UI)"`
	Uncompressed      bool   `json:"uncompressed,omitempty" jsonschema:"description=Disable compression of this package"`
	Architecture      string `json:"architecture,omitempty" jsonschema:"description=The target cluster architecture for this package,example=arm64,example=amd64"`
	YOLO              bool   `json:"yolo,omitempty" jsonschema:"description=Yaml OnLy Online (YOLO): True enables deploying a Zarf package without first running zarf init against the cluster. This is ideal for connected environments where you want to use existing VCS and container registries."`
	Authors           string `json:"authors,omitempty" jsonschema:"description=Comma-separated list of package authors (including contact info),example=Doug &#60;hello@defenseunicorns.com&#62;&#44; Pepr &#60;hello@defenseunicorns.com&#62;"`
	Documentation     string `json:"documentation,omitempty" jsonschema:"description=Link to package documentation when online"`
	Source            string `json:"source,omitempty" jsonschema:"description=Link to package source code when online"`
	Vendor            string `json:"vendor,omitempty" jsonschema_description:"Name of the distributing entity, organization or individual."`
	AggregateChecksum string `json:"aggregateChecksum,omitempty" jsonschema:"description=Checksum of a checksums.txt file that contains checksums all the layers within the package."`
}

// UDSBuildData is written during the packager.Create() operation to track details of the created package.
type UDSBuildData struct {
	Terminal              string            `json:"terminal" jsonschema:"description=The machine name that created this package"`
	User                  string            `json:"user" jsonschema:"description=The username who created this package"`
	Architecture          string            `json:"architecture" jsonschema:"description=The architecture this package was created on"`
	Timestamp             string            `json:"timestamp" jsonschema:"description=The timestamp when this package was created"`
	Version               string            `json:"version" jsonschema:"description=The version of Zarf used to build this package"`
	Migrations            []string          `json:"migrations,omitempty" jsonschema:"description=Any migrations that have been run on this package"`
	Differential          bool              `json:"differential,omitempty" jsonschema:"description=Whether this package was created with differential components"`
	RegistryOverrides     map[string]string `json:"registryOverrides,omitempty" jsonschema:"description=Any registry domains that were overridden on package create when pulling images"`
	DifferentialMissing   []string          `json:"differentialMissing,omitempty" jsonschema:"description=List of components that were not included in this package due to differential packaging"`
	OCIImportedComponents map[string]string `json:"OCIImportedComponents,omitempty" jsonschema:"description=Map of components that were imported via OCI. The keys are OCI Package URLs and values are the component names"`
}
