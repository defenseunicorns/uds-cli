// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package types contains all the types used by UDS.
package types

import (
	"github.com/defenseunicorns/uds-cli/src/types/chartvariable"
	"github.com/defenseunicorns/uds-cli/src/types/valuesources"
)

// UDSBundle is the top-level structure of a UDS bundle
type UDSBundle struct {
	Kind     string       `json:"kind" jsonschema:"description=The kind of UDS package,enum=UDSBundle"`
	Metadata UDSMetadata  `json:"metadata" jsonschema:"description=UDSBundle metadata"`
	Build    UDSBuildData `json:"build,omitempty" jsonschema:"description=Generated bundle build data"`
	Packages []Package    `json:"packages" jsonschema:"description=List of Zarf packages"`
}

// Package represents a Zarf package in a UDS bundle
type Package struct {
	Name               string                                     `json:"name" jsonschema:"name=Name of the Zarf package"`
	Description        string                                     `json:"description,omitempty" jsonschema:"description=Description of the Zarf package"`
	Repository         string                                     `json:"repository,omitempty" jsonschema:"description=The repository to import the package from"`
	Path               string                                     `json:"path,omitempty" jsonschema:"description=The local path to import the package from"`
	Ref                string                                     `json:"ref" jsonschema:"description=Ref (tag) of the Zarf package"`
	Flavor             string                                     `json:"flavor,omitempty" jsonschema:"description=Flavor of the Zarf package"`
	OptionalComponents []string                                   `json:"optionalComponents,omitempty" jsonschema:"description=List of optional components to include from the package (required components are always included)"`
	PublicKey          string                                     `json:"publicKey,omitempty" jsonschema:"description=The public key to use to verify the package"`
	Imports            []BundleVariableImport                     `json:"imports,omitempty" jsonschema:"description=List of Zarf variables to import from another Zarf package"`
	Exports            []BundleVariableExport                     `json:"exports,omitempty" jsonschema:"description=List of Zarf variables to export from the Zarf package"`
	Overrides          map[string]map[string]BundleChartOverrides `json:"overrides,omitempty" jsonschema:"description=Map of Helm chart overrides to set. The format is <component>:, <chart-name>:"`
	Values             *PackageValues                             `json:"values,omitempty" jsonschema:"description=Values configuration for Zarf packages using the values feature (alpha)"`
}

// PackageValues represents values configuration for Zarf packages using the values feature (alpha)
type PackageValues struct {
	Files     []string               `json:"files,omitempty" jsonschema:"description=Paths to values YAML files (relative to bundle directory, processed at create time and embedded in bundle)"`
	Set       map[string]interface{} `json:"set,omitempty" jsonschema:"description=Values to set using dot notation paths (e.g. .app.replicas)"`
	Variables []BundleValuesVariable `json:"variables,omitempty" jsonschema:"description=Map UDS variables to Zarf values paths"`
}

// BundleValuesVariable maps a UDS variable to a Zarf values path
type BundleValuesVariable struct {
	Name        string      `json:"name" jsonschema:"description=UDS variable name,required"`
	Path        string      `json:"path" jsonschema:"description=Dot notation path in Zarf values (e.g. .app.replicas),required"`
	Description string      `json:"description,omitempty" jsonschema:"description=Description of the variable"`
	Default     interface{} `json:"default,omitempty" jsonschema:"description=Default value if not provided"`
}

// BundleChartOverrides represents a Helm chart override to set via UDS variables
type BundleChartOverrides struct {
	Values      []BundleChartValue    `json:"values,omitempty" jsonschema:"description=List of Helm chart values to set statically"`
	Variables   []BundleChartVariable `json:"variables,omitempty" jsonschema:"description=List of Helm chart variables to set via UDS variables"`
	Namespace   string                `json:"namespace,omitempty" jsonschema:"description=The namespace to deploy the Helm chart to"`
	ValuesFiles []string              `json:"valuesFiles,omitempty" jsonschema:"description=List of Helm chart value file  paths to set statically"`
}

type BundleChartValue struct {
	Path  string      `json:"path" jsonschema:"name=Path to the Helm chart value to set. The format is <chart-value>, example=controller.service.type"`
	Value interface{} `json:"value" jsonschema:"name=The value to set"`
}

type BundleChartVariable struct {
	Path        string              `json:"path" jsonschema:"name=Path to the Helm chart value to set. The format is <chart-value>, example=controller.service.type"`
	Name        string              `json:"name" jsonschema:"name=Name of the variable to set"`
	Description string              `json:"description,omitempty" jsonschema:"name=Description of the variable"`
	Default     interface{}         `json:"default,omitempty" jsonschema:"name=The default value to set"`
	Type        chartvariable.Type  `json:"type,omitempty" jsonschema:"description=The type of value to be processed,enum=raw,enum=file"`
	Sensitive   bool                `json:"sensitive,omitempty" jsonschema:"description=Whether the value is sensitive"`
	Source      valuesources.Source `json:"source,omitempty" jsonschema:"description=Where the value is set from,enum=config,enum=env,enum=cli,enum=bundle"`
}

// BundleVariableImport represents variables in the bundle
type BundleVariableImport struct {
	Name        string `json:"name" jsonschema:"name=Name of the variable"`
	Package     string `json:"package" jsonschema:"name=Name of the Zarf package to get the variable from"`
	Description string `json:"description,omitempty" jsonschema:"name=Description of the variable"`
}

// BundleVariableExport represents variables in the bundle
type BundleVariableExport struct {
	Name        string `json:"name" jsonschema:"name=Name of the variable"`
	Description string `json:"description,omitempty" jsonschema:"name=Description of the variable"`
}

// UDSMetadata lists information about the current UDS Bundle.
type UDSMetadata struct {
	Name              string `json:"name" jsonschema:"description=Name to identify this UDS bundle,pattern=^[a-z0-9\\-]+$"`
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
