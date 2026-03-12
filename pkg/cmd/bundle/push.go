// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/config"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// PushOptions holds options for the push command.
type PushOptions struct {
	Tarball      string
	OCIReference string
	Config       config.BundleConfig

	iostreams.IOStreams
}

// NewPushOptions returns a PushOptions with default values.
func NewPushOptions(streams iostreams.IOStreams) *PushOptions {
	return &PushOptions{
		IOStreams: streams,
	}
}

// NewPushCommand creates the push command.
func NewPushCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewPushOptions(streams)

	cmd := &cobra.Command{
		Use:   "push <bundle-tarball> <oci-reference>",
		Short: "Push a bundle to an OCI registry",
		Long:  "Push a UDS bundle tarball to a remote OCI registry",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	return cmd
}

// Complete fills in options from command line args.
func (o *PushOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.Tarball = args[0]
	}
	if len(args) > 1 {
		o.OCIReference = args[1]
	}
	o.Config = config.BuildBundleConfig(cmd)
	return nil
}

// Validate validates the options.
func (o *PushOptions) Validate() error {
	if o.Tarball == "" || !strings.HasSuffix(o.Tarball, ".tar.zst") {
		return fmt.Errorf("source must be a .tar.zst bundle file")
	}
	if _, err := os.Stat(o.Tarball); os.IsNotExist(err) {
		return fmt.Errorf("bundle file not found: %s", o.Tarball)
	}
	if o.OCIReference == "" {
		return fmt.Errorf("OCI reference is required")
	}
	return ValidateBundleConfig(o.Config)
}

// Run executes the push command.
func (o *PushOptions) Run() error {
	slog.Debug("pushing bundle", "tarball", o.Tarball, "reference", o.OCIReference)
	if err := bundle.Push(context.Background(), bundle.PushOptions{
		RegistryOptions: bundle.RegistryOptions{
			PlainHTTP:     o.Config.PlainHTTP,
			SkipTLSVerify: o.Config.SkipTLSVerify,
		},
		BundleTarball: o.Tarball,
		OCIReference:  o.OCIReference,
		TmpDir:        o.Config.TmpDir,
	}); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(o.Out, "Bundle pushed successfully: %s\n", o.OCIReference)
	return nil
}
