package generate

import (
	"os"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/zarf/src/cmd/common"
	"github.com/defenseunicorns/zarf/src/pkg/packager"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
)

func Generate() {
	// Generate the metadata
	metadata := types.ZarfMetadata{
		Name:    config.GenerateChartName,
		Version: config.GenerateChartVersion + "-uds.0",
		URL:     config.GenerateChartUrl,
		Authors: "2 days no pROBlem",
	}

	// Generate the config chart zarf yaml
	// configChart := types.ZarfChart{
	// 	Name:      "uds-config",
	// 	Namespace: config.GenerateChartName,
	// 	LocalPath: "chart",
	// 	Version:   "0.0.1",
	// }

	// Generate the upstream chart zarf yaml
	upstreamChart := types.ZarfChart{
		Name:      config.GenerateChartName,
		Namespace: config.GenerateChartName,
		URL:       config.GenerateChartUrl,
		Version:   config.GenerateChartVersion,
	}

	// Generate the component
	component := types.ZarfComponent{
		Name:     config.GenerateChartName,
		Required: true,
		Charts:   []types.ZarfChart{upstreamChart},
		Only: types.ZarfComponentOnlyTarget{
			Flavor: "upstream",
		},
	}
	components := []types.ZarfComponent{component}

	// Generate the package
	packageInstance := types.ZarfPackage{
		Kind:       types.ZarfPackageConfig,
		Metadata:   metadata,
		Components: components,
	}

	// Write in progress zarf yaml to a file
	text, _ := goyaml.Marshal(packageInstance)
	os.WriteFile("zarf.yaml", text, 0644)

	// Find images to add to the component
	packagerConfig := types.PackagerConfig{
		Pkg: packageInstance,
		CreateOpts: types.ZarfCreateOptions{
			Flavor: "upstream",
		},
		// TODO: Why is this needed?
		FindImagesOpts: types.ZarfFindImagesOptions{
			KubeVersionOverride: "1.28.0",
		},
	}
	common.SetBaseDirectory(nil, &packagerConfig)

	packager := packager.NewOrDie(&packagerConfig)
	defer packager.ClearTempPaths()

	stdout := os.Stdout
	os.Stdout = nil
	images, _ := packager.FindImages()
	os.Stdout = stdout
	// TODO: Strip off cosign signatures/attestations?
	components[0].Images = images[config.GenerateChartName]

	// Generate UDS Config chart

	utils.ColorPrintYAML(packageInstance, nil, false)

	// Write in progress zarf yaml to a file
	text, _ = goyaml.Marshal(packageInstance)
	os.WriteFile("zarf.yaml", text, 0644)
}
