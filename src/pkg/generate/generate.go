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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
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
	var exposeList []Expose
	chartName := config.GenerateChartName
	chartVersion := config.GenerateChartVersion
	repoURL := config.GenerateChartUrl

	settings := cli.New()
	actionConfig := new(action.Configuration)
	actionConfig.Init(settings.RESTClientGetter(), chartName, "", log.Printf)

	pull := action.NewPull()
	pull.Settings = cli.New()

	chartDownloader := downloader.ChartDownloader{
		Out:            nil,
		RegistryClient: nil,
		Verify:         downloader.VerifyNever,
		Getters:        getter.All(pull.Settings),
		Options: []getter.Option{
			getter.WithInsecureSkipVerifyTLS(config.CommonOptions.Insecure),
		},
	}

	temp := filepath.Join(config.GenerateOutputDir, "temp")
	if err := utils.CreateDirectory(temp, 0700); err != nil {
		return nil, err
	}
	defer os.RemoveAll(temp)

	chartURL, _ := repo.FindChartInAuthRepoURL(repoURL, "", "", chartName, chartVersion, pull.CertFile, pull.KeyFile, pull.CaFile, getter.All(pull.Settings))

	saved, _, err := chartDownloader.DownloadTo(chartURL, pull.Version, temp)
	if err != nil {
		return nil, err
	}

	client := action.NewInstall(actionConfig)
	client.DryRun = true
	client.Replace = true // Skip the name check.
	client.ClientOnly = true
	client.IncludeCRDs = true
	client.Verify = false
	client.KubeVersion, _ = chartutil.ParseKubeVersion(kubeVersionOverride)
	client.InsecureSkipTLSverify = config.CommonOptions.Insecure
	client.ReleaseName = chartName
	client.Namespace = chartName

	loadedChart, err := loader.Load(saved)
	if err != nil {
		return nil, err
	}

	templatedChart, err := client.Run(loadedChart, nil)
	if err != nil {
		return nil, err
	}
	template := templatedChart.Manifest
	yamls, _ := utils.SplitYAML([]byte(template))
	var resources []*unstructured.Unstructured
	resources = append(resources, yamls...)

	for _, resource := range resources {
		if resource.GetKind() == "Service" {
			contents := resource.UnstructuredContent()
			var service v1.Service
			runtime.DefaultUnstructuredConverter.FromUnstructured(contents, &service)
			for _, port := range service.Spec.Ports {
				// Guess that we want to expose any ports named "http"
				if port.Name == "http" {
					expose := Expose{
						Gateway:  "tenant",
						Host:     service.ObjectMeta.Name,
						Port:     int(port.Port),
						Selector: service.Spec.Selector,
						Service:  service.ObjectMeta.Name,
						// TODO: Target Port
					}
					exposeList = append(exposeList, expose)
				}
			}
		}
	}
	return exposeList, nil
}
