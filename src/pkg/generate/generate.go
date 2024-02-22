package generate

import (
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/types"
)

func Generate() {
	metadata := types.ZarfMetadata{
		Name:    config.GenerateChartName,
		Version: config.GenerateChartVersion + "-uds.0",
		URL:     config.GenerateChartUrl,
		Authors: "Rob Ferguson",
	}

	// component :=

	packageInstance := types.ZarfPackage{
		Kind:       types.ZarfPackageConfig,
		Metadata:   metadata,
		Components: []types.ZarfComponent{},
	}
	utils.ColorPrintYAML(packageInstance, nil, false)
}
