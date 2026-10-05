// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packagecli

import (
	"context"
	"errors"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/internal/mode/disassemble"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/internal/zarf"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// ReassembleOptions holds options for package reassembly.
type ReassembleOptions struct {
	SourceDir            string
	Output               string
	MaxPackageSizeMB     int
	SigningKeyPath       string
	SigningKeyPassword   string
	WithBuildMachineInfo bool
	commonOptions

	iostreams.IOStreams
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
			if err := o.Validate(); err != nil {
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
	return completeCommonOptions(cmd, &o.commonOptions)
}

// Validate validates package reassembly options.
func (o *ReassembleOptions) Validate() error {
	switch {
	case strings.TrimSpace(o.SourceDir) == "":
		return errors.New("source directory is required")
	case strings.TrimSpace(o.TmpDir) == "":
		return errors.New("temporary directory is required")
	case o.MaxPackageSizeMB < 0:
		return errors.New("maximum package size must not be negative")
	case o.Concurrency < 1:
		return errors.New("concurrency must be greater than zero")
	default:
		return nil
	}
}

// Run reassembles the source directory and prints its result as text.
func (o *ReassembleOptions) Run(ctx context.Context) error {
	o.IOStreams = logger.Bind(o.IOStreams, o.LogLevelName)
	zarf.ConfigureTempDir(o.TmpDir)
	result, err := disassemble.Reassemble(ctx, disassemble.ReassembleOptions{
		SourceDir:            o.SourceDir,
		Output:               o.Output,
		PlainHTTP:            o.PlainHTTP,
		SkipTLSVerify:        o.SkipTLSVerify,
		CachePath:            o.CachePath,
		MaxPackageSizeMB:     o.MaxPackageSizeMB,
		SigningKeyPath:       o.SigningKeyPath,
		SigningKeyPassword:   o.SigningKeyPassword,
		WithBuildMachineInfo: o.WithBuildMachineInfo,
		Concurrency:          o.Concurrency,
	})
	if err != nil {
		return err
	}
	return (&printer.TextPrinter{}).PrintObj(result, o.Out())
}
