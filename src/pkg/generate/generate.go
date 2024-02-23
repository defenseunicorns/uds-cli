package generate

import (
	"embed"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/zarf/src/pkg/packager"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
)

//go:embed chart/*
var folder embed.FS

//go:embed tasks.yaml
var tasks embed.FS

var kubeVersionOverride = "1.28.0"

type Generator struct {
	pkg       types.PackagerConfig
	component types.ZarfComponent
}

var generator Generator

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
	generator.component = types.ZarfComponent{
		Name:     config.GenerateChartName,
		Required: true,
		Charts:   []types.ZarfChart{configChart, upstreamChart},
		Only: types.ZarfComponentOnlyTarget{
			Flavor: "upstream",
		},
	}
	components := []types.ZarfComponent{generator.component}

	// Generate the package
	packageInstance := types.ZarfPackage{
		Kind:       types.ZarfPackageConfig,
		Metadata:   metadata,
		Components: components,
	}

	// Create generated directory if it doesn't exist
	if err := os.MkdirAll(config.GenerateOutputDir, 0755); err != nil {
		panic(err)
	}
	zarfPath := filepath.Join(config.GenerateOutputDir, "zarf.yaml")

	// Write in progress zarf yaml to a file
	text, _ := goyaml.Marshal(packageInstance)
	os.WriteFile(zarfPath, text, 0644)

	// Copy template chart to destination
	writeChart(folder)

	// Manipulate chart
	if err := manipulatePackage(); err != nil {
		panic(err)
	}

	writeTasks(tasks)

	// Find images to add to the component
	generator.pkg = types.PackagerConfig{
		CreateOpts: types.ZarfCreateOptions{
			Flavor:  "upstream",
			BaseDir: config.GenerateOutputDir,
		},
		// TODO: Why is this needed?
		FindImagesOpts: types.ZarfFindImagesOptions{
			KubeVersionOverride: kubeVersionOverride,
		},
	}

	packager := packager.NewOrDie(&generator.pkg)
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
	os.WriteFile(zarfPath, text, 0644)
}

// Write an embedded folder (template helm chart) to the localDir
func writeChart(folder embed.FS) {
	// Walk through the embedded filesystem
	err := fs.WalkDir(folder, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Construct destination file path
		destPath := filepath.Join(config.GenerateOutputDir, path)

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

func manipulatePackage() error {
	var udsPackage Package
	packagePath := filepath.Join(config.GenerateOutputDir, "chart", "templates", "uds-package.yaml")
	packageYaml, err := os.ReadFile(packagePath)
	if err != nil {
		return err
	}
	if err := goyaml.Unmarshal(packageYaml, &udsPackage); err != nil {
		return err
	}
	udsPackage.ObjectMeta.Name = config.GenerateChartName
	udsPackage.ObjectMeta.Namespace = config.GenerateChartName

	expose, err := findHttpServices()
	if err != nil {
		return err
	}
	if expose != nil {
		udsPackage.Spec.Network.Expose = expose
	}

	text, _ := goyaml.Marshal(udsPackage)
	os.WriteFile(packagePath, text, 0644)
	return nil
}

func writeTasks(tasks embed.FS) error {
	fileName := "tasks.yaml"

	// Open the embedded file.
	fileData, err := tasks.Open(fileName)
	if err != nil {
		return err
	}
	defer fileData.Close()

	// Create a new file in the target directory.
	targetPath := config.GenerateOutputDir + "/" + fileName
	outFile, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// Copy the content of the embedded file to the new file.
	if _, err := io.Copy(outFile, fileData); err != nil {
		return err
	}

	log.Printf("File %s copied to %s successfully.", fileName, config.GenerateOutputDir)
	return nil
}

func findHttpServices() ([]Expose, error) {
	return nil, nil
}
