// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// CreateOptions holds options for the create command.
type CreateOptions struct {
	BundleFile string
	Arch       string
	StatFn     func(string) (os.FileInfo, error)

	iostreams.IOStreams
}

// NewCreateOptions returns a CreateOptions with default values.
func NewCreateOptions(streams iostreams.IOStreams) *CreateOptions {
	return &CreateOptions{
		IOStreams: streams,
		StatFn:   os.Stat,
	}
}

// NewCreateCommand creates the create command.
func NewCreateCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewCreateOptions(streams)

	cmd := &cobra.Command{
		Use:   "create [directory]",
		Short: "Create a new UDS bundle",
		Long:  "Create a new UDS bundle from an HCL configuration file",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	cmd.Flags().StringVarP(&o.Arch, "arch", "a", "", "architecture to create bundle for (defaults to system architecture)")

	return cmd
}

// Complete fills in options from command line args.
func (o *CreateOptions) Complete(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundleFile = args[0]
	} else {
		o.BundleFile = "."
	}
	return nil
}

// Validate validates the options. It expects a directory containing bundle.uds.hcl
// and resolves BundleFile to the full path of that file.
func (o *CreateOptions) Validate() error {
	if o.BundleFile == "" {
		return fmt.Errorf("directory is required")
	}
	statFn := o.StatFn
	if statFn == nil {
		statFn = os.Stat
	}
	info, err := statFn(o.BundleFile)
	if err != nil {
		return fmt.Errorf("path not found: %s", o.BundleFile)
	}
	if !info.IsDir() {
		return fmt.Errorf("expected a directory containing %s, got: %s", util.BundleFileName, o.BundleFile)
	}
	bundlePath := filepath.Join(o.BundleFile, util.BundleFileName)
	if _, err := statFn(bundlePath); err != nil {
		return fmt.Errorf("directory does not contain %s: %s", util.BundleFileName, o.BundleFile)
	}
	o.BundleFile = bundlePath
	return nil
}

// Run executes the create command.
func (o *CreateOptions) Run() error {
	ctx := context.Background()
	fmt.Fprintf(o.Out, "Creating bundle from file: %s\n", o.BundleFile)
	outPath, err := bundle.Create(ctx, bundle.CreateOptions{
		BundleFile: o.BundleFile,
		Arch:       o.Arch,
		Out:        o.Out,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "Bundle created: %s\n", outPath)
	return nil
}
