// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

const (
	// BundleFileName is the standard name for UDS bundle definition files
	BundleFileName = "bundle.uds.hcl"
)

// InspectOptions holds the options for the inspect command.
type InspectOptions struct {
	BundleRef string
	iostreams.IOStreams
}

// NewInspectOptions returns a new InspectOptions with default values.
func NewInspectOptions(streams iostreams.IOStreams) *InspectOptions {
	return &InspectOptions{
		IOStreams: streams,
	}
}

// NewInspectCommand creates the inspect command.
func NewInspectCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewInspectOptions(streams)

	cmd := &cobra.Command{
		Use:   "inspect [bundle-file]",
		Short: "Inspect a UDS bundle",
		Long:  "Inspect a UDS bundle definition from a local HCL file, displaying metadata and package details.",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	return cmd
}

// Complete fills in options from command line args.
func (o *InspectOptions) Complete(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundleRef = args[0]
	} else {
		// Default to looking for bundle.uds.hcl in current directory
		o.BundleRef = "."
	}
	return nil
}

// Validate validates the options.
func (o *InspectOptions) Validate() error {
	if o.BundleRef == "" {
		return fmt.Errorf("bundle file path is required")
	}

	// Check for tar.zst archive first (before checking if it exists)
	if strings.HasSuffix(o.BundleRef, ".tar.zst") {
		return fmt.Errorf("tar.zst inspection not yet supported. Please provide a local .hcl file path or directory")
	}

	// Check for OCI reference (before checking if it exists as a file)
	if strings.Contains(o.BundleRef, "://") || isOCIRef(o.BundleRef) {
		return fmt.Errorf("OCI inspection not yet supported. Please provide a local .hcl file path or directory")
	}

	// Check if the path exists
	info, err := os.Stat(o.BundleRef)
	if err != nil {
		return fmt.Errorf("bundle path not found: %s", o.BundleRef)
	}

	if info.IsDir() {
		// If it's a directory, check if bundle.uds.hcl exists in it
		bundlePath := filepath.Join(o.BundleRef, BundleFileName)
		if _, err := os.Stat(bundlePath); err != nil {
			return fmt.Errorf("directory does not contain %s: %s", BundleFileName, o.BundleRef)
		}
		// Update BundleRef to point to the actual file
		o.BundleRef = bundlePath
		return nil
	}

	// It's a file - validate it's named bundle.uds.hcl
	if filepath.Base(o.BundleRef) != BundleFileName {
		return fmt.Errorf("expected file named '%s', got: %s", BundleFileName, filepath.Base(o.BundleRef))
	}

	return nil
}

// Run executes the inspect command.
func (o *InspectOptions) Run() error {
	b, err := bundle.NewHCLParser().ParseBundleFile(context.Background(), o.BundleRef)
	if err != nil {
		return err
	}

	if err := b.Validate(); err != nil {
		return fmt.Errorf("invalid bundle: %w", err)
	}

	out, err := b.BufferString()
	if err != nil {
		return err
	}
	_, err = o.Out.Write(out.Bytes())
	return err
}

// isOCIRef checks if a string looks like an OCI registry reference
// (e.g., "ghcr.io/org/repo:tag" or "registry.example.com/image").
// It should NOT match local file paths.
func isOCIRef(s string) bool {
	// If it starts with a path separator or contains backslash, it's a file path
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.Contains(s, "\\") {
		return false
	}

	// If it's the bundle filename or has file extensions, it's a file path
	if s == BundleFileName || strings.Contains(s, BundleFileName) || strings.Contains(s, ".tar") || strings.Contains(s, ".yaml") || strings.Contains(s, ".yml") {
		return false
	}

	// An OCI ref typically looks like: domain/path or domain/path:tag
	// It must contain both a dot (in the domain) and a slash (in the path)
	// and should not have spaces
	if strings.Contains(s, " ") {
		return false
	}

	// Must have both a dot and a slash to be considered an OCI ref
	return strings.Contains(s, ".") && strings.Contains(s, "/")
}
