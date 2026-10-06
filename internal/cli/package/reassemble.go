// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packagecli

import (
	"context"

	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/internal/zarf/modify"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// ReassembleOptions holds options for package reassembly.
type ReassembleOptions struct {
	modify.ReassembleOptions
	LogLevelName string

	iostreams.IOStreams
}

type reassembleResult struct {
	SourceDir  string `json:"sourceDir" yaml:"sourceDir" text:"Source Directory"`
	OutputPath string `json:"outputPath" yaml:"outputPath" text:"Output Path"`
}

// NewReassembleOptions returns package reassembly options.
func NewReassembleOptions(streams iostreams.IOStreams) *ReassembleOptions {
	return &ReassembleOptions{IOStreams: streams}
}

// NewReassembleCommand creates the package reassemble command.
func NewReassembleCommand(streams iostreams.IOStreams) *cobra.Command {
	return newReassembleCommand(NewReassembleOptions(streams))
}

func newReassembleCommand(o *ReassembleOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reassemble <source-dir>",
		Short: "Reassemble a Zarf package from disassembled source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Complete(cmd, args); err != nil {
				return err
			}
			return o.Run(cmd.Context())
		},
	}
	cmd.Flags().StringP("output", "o", "", "specify the output (either a directory or an oci:// URL) for the created Zarf package")
	cmd.Flags().IntP("max-package-size", "m", 0, "maximum package size in megabytes; larger packages are split, and 0 disables splitting")
	cmd.Flags().String("signing-key", "", "private key for signing the recreated package")
	cmd.Flags().String("signing-key-pass", "", "password for the package signing key")
	cmd.Flags().Bool("with-build-machine-info", false, "include the build hostname and username in package metadata")
	return cmd
}

// Complete fills options from command-line arguments and flags.
func (o *ReassembleOptions) Complete(cmd *cobra.Command, args []string) error {
	o.SourceDir = args[0]
	var err error
	o.Output, err = cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	o.MaxPackageSizeMB, err = cmd.Flags().GetInt("max-package-size")
	if err != nil {
		return err
	}
	o.SigningKeyPath, err = cmd.Flags().GetString("signing-key")
	if err != nil {
		return err
	}
	o.SigningKeyPassword, err = cmd.Flags().GetString("signing-key-pass")
	if err != nil {
		return err
	}
	o.WithBuildMachineInfo, err = cmd.Flags().GetBool("with-build-machine-info")
	if err != nil {
		return err
	}
	o.LogLevelName, err = completePackageOptions(cmd, &o.PackageOptions)
	return err
}

// Run reassembles the source directory and prints its result as text.
func (o *ReassembleOptions) Run(ctx context.Context) error {
	o.IOStreams = logger.Bind(o.IOStreams, o.LogLevelName)
	outputPath, err := modify.Reassemble(ctx, o.ReassembleOptions)
	if err != nil {
		return err
	}
	result := &reassembleResult{SourceDir: o.SourceDir, OutputPath: outputPath}
	return (&printer.TextPrinter{}).PrintObj(result, o.Out())
}
