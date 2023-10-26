// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package utils provides utility fns for UDS-CLI
package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/pkg/oci"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/pkg/utils/helpers"
	av4 "github.com/mholt/archiver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pterm/pterm"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	"github.com/defenseunicorns/uds-cli/src/config"
)

// MergeVariables merges the variables from the config file and the CLI
//
// TODO: move this to helpers.MergeAndTransformMap
func MergeVariables(left map[string]string, right map[string]string) map[string]string {
	// Ensure uppercase keys from viper
	leftUpper := helpers.TransformMapKeys(left, strings.ToUpper)
	rightUpper := helpers.TransformMapKeys(right, strings.ToUpper)

	// Merge the viper config file variables and provided CLI flag variables (CLI takes precedence))
	return helpers.MergeMap(leftUpper, rightUpper)
}

// IsValidTarballPath returns true if the path is a valid tarball path to a bundle tarball
func IsValidTarballPath(path string) bool {
	if utils.InvalidPath(path) || utils.IsDir(path) {
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

// UseLogFile writes output to stderr and a logFile.
func UseLogFile() {
	// LogWriter is the stream to write logs to.
	var LogWriter io.Writer = os.Stderr

	// Write logs to stderr and a buffer for logFile generation.
	var logFile *os.File

	// Prepend the log filename with a timestamp.
	ts := time.Now().Format("2006-01-02-15-04-05")

	var err error
	if logFile != nil {
		// Use the existing log file if logFile is set
		LogWriter = io.MultiWriter(os.Stderr, logFile)
		pterm.SetDefaultOutput(LogWriter)
	} else {
		// Try to create a temp log file if one hasn't been made already
		if logFile, err = os.CreateTemp("", fmt.Sprintf("uds-%s-*.log", ts)); err != nil {
			message.WarnErr(err, "Error saving a log file to a temporary directory")
		} else {
			LogWriter = io.MultiWriter(os.Stderr, logFile)
			pterm.SetDefaultOutput(LogWriter)
			msg := fmt.Sprintf("Saving log file to %s", logFile.Name())
			message.Note(msg)
		}
	}
}

// CreateCopyOpts creates the ORAS CopyOpts struct to use when copying OCI artifacts
func CreateCopyOpts(layersToPull []ocispec.Descriptor, concurrency int) oras.CopyOptions {
	var copyOpts oras.CopyOptions
	copyOpts.Concurrency = concurrency
	estimatedBytes := int64(0)
	var shas []string
	for _, layer := range layersToPull {
		if len(layer.Digest.String()) > 0 {
			estimatedBytes += layer.Size
			shas = append(shas, layer.Digest.Encoded())
		}
	}
	copyOpts.FindSuccessors = func(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		var nodes []ocispec.Descriptor
		if desc.MediaType == oci.ZarfLayerMediaTypeBlob && desc.Annotations == nil {
			layerBytes, err := content.FetchAll(ctx, fetcher, desc)
			if err != nil {
				return nil, err
			}
			var manifest oci.ZarfOCIManifest
			if err := json.Unmarshal(layerBytes, &manifest); err != nil {
				return nil, err
			}
			if manifest.Subject != nil {
				nodes = append(nodes, *manifest.Subject)
			}
			nodes = append(nodes, manifest.Config)
			nodes = append(nodes, manifest.Layers...)
		} else {
			successors, err := content.Successors(ctx, fetcher, desc)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, successors...)
		}
		var ret []ocispec.Descriptor
		for _, node := range nodes {
			if node.Size != 0 && slices.Contains(shas, node.Digest.Encoded()) {
				ret = append(ret, node)
			}
		}
		return ret, nil
	}
	return copyOpts
}

// ExtractJSON extracts and unmarshals a tarballed JSON file into a type
func ExtractJSON(j any) func(context.Context, av4.File) error {
	return func(_ context.Context, file av4.File) error {
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
