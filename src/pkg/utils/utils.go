// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package utils provides utility fns for UDS-CLI
package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/uds-cli/src/config"
	"github.com/defenseunicorns/uds-cli/src/pkg/message"
	"github.com/defenseunicorns/uds-cli/src/types"
	goyaml "github.com/goccy/go-yaml"
	"github.com/mholt/archives"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
)

// IsValidTarballPath returns true if the path is a valid tarball path to a bundle tarball
func IsValidTarballPath(path string) bool {
	if helpers.InvalidPath(path) || helpers.IsDir(path) {
		return false
	}
	name := filepath.Base(path)
	if name == "" {
		return false
	}
	if !strings.HasPrefix(name, config.BundlePrefix) {
		return false
	}
	re := regexp.MustCompile(`^uds-bundle-.*-.*.tar(.zst)?$`)
	return re.MatchString(name)
}

// IncludeComponent checks if a component has been specified in a a list of components (used for filtering optional components)
func IncludeComponent(componentToCheck string, filteredComponents []v1alpha1.ZarfComponent) bool {
	for _, component := range filteredComponents {
		// get component name from annotation
		nameWithSuffix := strings.Split(componentToCheck, "components/")[1]
		componentName := strings.Split(nameWithSuffix, ".tar")[0]
		if componentName == component.Name {
			return true
		}
	}
	return false
}

// ConfigureLogs sets up the log file, log cache and output for the CLI
func ConfigureLogs(cmd *cobra.Command) error {
	// don't configure UDS logs for vendored cmds
	if strings.HasPrefix(cmd.Use, "zarf") || strings.HasPrefix(cmd.Use, "run") {
		return nil
	}

	// create a temporary log file
	ts := time.Now().Format("2006-01-02-15-04-05")
	tmpLogFile, err := os.CreateTemp("", fmt.Sprintf("uds-%s-*.log", ts))
	if err != nil {
		message.WarnErr(err, "Error creating a log file in a temporary directory")
		return err
	}
	tmpLogLocation := tmpLogFile.Name()

	writer, err := message.UseLogFile(tmpLogFile)
	if err != nil {
		return err
	}
	pterm.SetDefaultOutput(io.MultiWriter(os.Stderr, writer))

	// Set up cache dir and cache logs file
	cacheDir := filepath.Join(config.CommonOptions.CachePath)
	if err := os.MkdirAll(cacheDir, 0o0755); err != nil { // Ensure the directory exists
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// remove old cache logs file, and set up symlink to the new log file
	os.Remove(filepath.Join(config.CommonOptions.CachePath, config.CachedLogs))
	if err = os.Symlink(tmpLogLocation, filepath.Join(config.CommonOptions.CachePath, config.CachedLogs)); err != nil {
		return err
	}

	// don't print the note for inspect cmds because they are used in automation
	if !strings.Contains(cmd.Use, "inspect") {
		message.Notef("Saving log file to %s", tmpLogLocation)
	}
	return nil
}

// ExtractJSON extracts and unmarshals a tarballed JSON file into a type
func ExtractJSON(j any, expectedFilepath string) archives.FileHandler {
	return func(_ context.Context, file archives.FileInfo) error {
		if file.NameInArchive != expectedFilepath {
			return nil
		}

		stream, err := file.Open()
		if err != nil {
			return err
		}
		defer stream.Close()

		fileBytes, err := io.ReadAll(stream)
		if err != nil {
			return err
		}
		return json.Unmarshal(fileBytes, &j)
	}
}

// ExtractBytes returns an archives.FileHandler that extracts a byte contents of a file from an archive
func ExtractBytes(b *[]byte, expectedFilepath string) archives.FileHandler {
	return func(_ context.Context, file archives.FileInfo) error {
		if file.NameInArchive != expectedFilepath {
			return nil
		}

		stream, err := file.Open()
		if err != nil {
			return err
		}
		defer stream.Close()

		fileBytes, err := io.ReadAll(stream)
		if err != nil {
			return err
		}

		*b = fileBytes
		return nil
	}
}

// ExtractFile returns an archives.FileHandler that extracts a file from an archive
func ExtractFile(expectedFilepath, outDirPath string) archives.FileHandler {
	return extractFiles(expectedFilepath, outDirPath)
}

// ExtractAllFiles returns a archives.FileHandler that extracts all the contents of the archive into the provided outDirPath
func ExtractAllFiles(outDirPath string) archives.FileHandler {
	return extractFiles("", outDirPath)
}

// extractFiles returns an archives.FileHandler that extracts file(s) from an archive.
// If the provided extractedPath is empty, all files will be extracted
func extractFiles(expectedFilepath string, outDirPath string) archives.FileHandler {
	return func(_ context.Context, file archives.FileInfo) error {
		// If an expectedFilepath was provided and it doesn't match the name of this file; do nothing
		if expectedFilepath != "" && file.NameInArchive != expectedFilepath {
			return nil
		}

		outPath := filepath.Join(outDirPath, file.NameInArchive)

		// If the entry is a directory, just create it and return
		if file.IsDir() {
			return os.MkdirAll(outPath, 0755)
		}

		// For files, ensure parent directory exists
		err := os.MkdirAll(filepath.Dir(outPath), 0755)
		if err != nil {
			return err
		}

		// Create the output file
		outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer outFile.Close()

		// Open the stream from the archive
		stream, err := file.Open()
		if err != nil {
			return err
		}
		defer stream.Close()

		// Stream directly from the archive to the file without loading everything into memory
		_, err = io.Copy(outFile, stream)
		return err
	}
}

// ToLocalFile takes an arbitrary type, typically a struct, marshals it into JSON and stores it as a local file
func ToLocalFile(t any, filePath string) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	tFile, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer tFile.Close()
	_, err = tFile.Write(b)
	if err != nil {
		return err
	}
	return nil
}

// IsRemotePkg returns true if the Zarf package is remote
func IsRemotePkg(pkg types.Package) bool {
	return pkg.Repository != ""
}

func hasScheme(s string) bool {
	return strings.Contains(s, "://")
}

// hasDomain checks if a string contains a domain.
// It assumes the domain is at the beginning of a URL and there is no scheme (e.g., oci://).
func hasDomain(s string) bool {
	dotIndex := strings.Index(s, ".")
	firstSlashIndex := strings.Index(s, "/")

	// dot exists; dot is not first char; not preceded by any / if / exists
	return dotIndex != -1 && dotIndex != 0 && (firstSlashIndex == -1 || firstSlashIndex > dotIndex)
}

func hasPort(s string) bool {
	// look for colon and port (e.g localhost:31999)
	colonIndex := strings.Index(s, ":")
	firstSlashIndex := strings.Index(s, "/")
	endIndex := firstSlashIndex
	if firstSlashIndex == -1 {
		endIndex = len(s) - 1
	}
	if colonIndex != -1 {
		port := s[colonIndex+1 : endIndex]

		// port valid number ?
		_, err := strconv.Atoi(port)
		if err == nil {
			return true
		}
	}
	return false
}

// IsRegistryURL checks if a string is a URL
func IsRegistryURL(s string) bool {
	if hasScheme(s) || hasDomain(s) || hasPort(s) {
		return true
	}

	return false
}

// ReadYAMLStrict reads a YAML file into a struct, with strict parsing
func ReadYAMLStrict(path string, destConfig any) error {
	message.Debugf("Reading YAML at %s", path)

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file at %s: %v", path, err)
	}
	defer file.Close()

	// First try with strict mode
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file at %s: %v", path, err)
	}

	err = goyaml.UnmarshalWithOptions(fileBytes, destConfig, goyaml.Strict())
	if err != nil {
		message.Warnf("failed strict unmarshalling of YAML at %s: %v", path, err)

		// Try again with non-strict mode
		err = goyaml.UnmarshalWithOptions(fileBytes, destConfig)
		if err != nil {
			return fmt.Errorf("failed to unmarshal YAML at %s: %v", path, err)
		}
	}
	return nil
}

// CheckYAMLSourcePath checks if the provided YAML source path is valid
func CheckYAMLSourcePath(source string) error {
	// check if the source is a YAML file
	isYaml := strings.HasSuffix(source, ".yaml") || strings.HasSuffix(source, ".yml")
	if !isYaml {
		return errors.New("source must have .yaml or yml file extension")
	}
	// Check if the file exists
	if isInvalid := helpers.InvalidPath(source); isInvalid {
		return fmt.Errorf("file %s does not exist or has incorrect permissions", source)
	}

	return nil
}

// JSONValue prints any value as JSON.
func JSONValue(value any) (string, error) {
	bytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CanWriteToDir verifies the process can write to the provided directory
func CanWriteToDir(dir string) error {
	if dir == "" {
		dir = "."
	}

	if err := os.MkdirAll(dir, 0o0755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dir, err)
	}

	file, err := os.CreateTemp(dir, ".permcheck")
	if err != nil {
		return err
	}

	// we don't care much for errors on closing & removing, we only want to validate that we can create
	_ = file.Close()
	_ = os.Remove(file.Name())

	return nil
}

// GetPackageVerificationStrategy determines the package verification strategy in which to pass to the Zarf SDK based on the skipSignatureValidation flag
func GetPackageVerificationStrategy(skipSignatureValidation bool) layout.VerificationStrategy {
	if skipSignatureValidation {
		return layout.VerifyNever
	}
	return layout.VerifyAlways
}

// LoadPackage fetches, verifies, and loads a Zarf package from the specified source.
func LoadPackage(ctx context.Context, source string, opts packager.LoadOptions) (_ *layout.PackageLayout, err error) {
	verificationStrategy := opts.VerificationStrategy

	// Load the package without package verification, in case it is unsigned
	opts.VerificationStrategy = layout.VerifyNever
	pkgLayout, err := packager.LoadPackage(ctx, source, opts)
	if err != nil {
		return pkgLayout, err
	}

	// Verify is package is signed and verificationStrategy not set to never (skip)
	if pkgLayout.IsSigned() && verificationStrategy != layout.VerifyNever {
		verifyOpts := zarfUtils.VerifyBlobOptions{}
		verifyOpts.KeyRef = opts.PublicKeyPath
		err := pkgLayout.VerifyPackageSignature(ctx, verifyOpts)
		if err != nil {
			return nil, err
		}
	}

	return pkgLayout, nil
}

func LoadPackageFromDir(ctx context.Context, dirPath string, opts layout.PackageLayoutOptions) (*layout.PackageLayout, error) {
	verificationStrategy := opts.VerificationStrategy

	// Load the package without package verification, in case it is unsigned
	opts.VerificationStrategy = layout.VerifyNever
	pkgLayout, err := layout.LoadFromDir(ctx, dirPath, opts)
	if err != nil {
		return pkgLayout, err
	}

	// Verify is package is signed and verificationStrategy not set to never (skip)
	if pkgLayout.IsSigned() && verificationStrategy != layout.VerifyNever {
		verifyOpts := zarfUtils.VerifyBlobOptions{}
		verifyOpts.KeyRef = opts.PublicKeyPath
		err := pkgLayout.VerifyPackageSignature(ctx, verifyOpts)
		if err != nil {
			return nil, err
		}
	}

	return pkgLayout, nil
}
