package generate

import (
	"embed"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/zarf/src/cmd/common"
	"github.com/defenseunicorns/zarf/src/pkg/packager"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
)

//go:embed chart/*
var folder embed.FS

// Generate a UDS Package from a given helm chart in the config
func Generate() {
	// Generate the metadata
	metadata := types.ZarfMetadata{
		Name:    config.GenerateChartName,
		Version: config.GenerateChartVersion + "-uds.0",
		URL:     config.GenerateChartUrl,
		Authors: "2 days no pROBlem",
	}

	// Generate the config chart zarf yaml
	configChart := types.ZarfChart{
		Name:      "uds-config",
		Namespace: config.GenerateChartName,
		LocalPath: "chart",
		Version:   "0.1.0",
	}

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
		Charts:   []types.ZarfChart{configChart, upstreamChart},
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

	// Copy template chart to destination
	writeChart(folder)

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

	utils.ColorPrintYAML(packageInstance, nil, false)

	// Write final zarf yaml to a file
	text, _ = goyaml.Marshal(packageInstance)
	os.WriteFile("zarf.yaml", text, 0644)
}

// Write an embedded folder (template helm chart) to the localDir
func writeChart(folder embed.FS) {
	// Destination directory where you want to write the files
	destDir := "."

	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(destDir, 0755); err != nil {
		panic(err)
	}

	// Walk through the embedded filesystem
	err := fs.WalkDir(folder, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Construct destination file path
		destPath := filepath.Join(destDir, path)

		if d.IsDir() {
			// Create directory if it doesn't exist
			return os.MkdirAll(destPath, 0755)
		}

		// Open source file from embedded filesystem
		srcFile, err := folder.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		// Create destination file
		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		// Copy contents from source to destination
		_, err = io.Copy(destFile, srcFile)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		panic(err)
	}
}
