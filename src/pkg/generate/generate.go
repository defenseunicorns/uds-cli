package generate

import (
	"fmt"

	"github.com/defenseunicorns/uds-cli/src/config"
)

func Generate() {
	fmt.Println(config.GenerateChartUrl)
	fmt.Println(config.GenerateChartName)
	fmt.Println(config.GenerateChartVersion)
}
