// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"fmt"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// DeployOptions holds options for the deploy command.
type DeployOptions struct {
	OCIReference string

	iostreams.IOStreams
}

// NewDeployOptions returns a DeployOptions with default values.
func NewDeployOptions(streams iostreams.IOStreams) *DeployOptions {
	return &DeployOptions{
		IOStreams: streams,
	}
}

// NewDeployCommand creates the deploy command.
func NewDeployCommand(streams iostreams.IOStreams) *cobra.Command {
	o := NewDeployOptions(streams)

	cmd := &cobra.Command{
		Use:   "deploy <bundle-oci-reference>",
		Short: "Deploy a bundle to a Kubernetes cluster",
		Long:  "Deploy a UDS bundle to a Kubernetes cluster using the provided OCI reference",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			util.CheckErr(o.Run())
		},
	}

	return cmd
}

// Complete fills in options from command line args.
func (o *DeployOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.OCIReference = args[0]
	}
	return nil
}

// Validate validates the options.
func (o *DeployOptions) Validate() error {
	if o.OCIReference == "" {
		return fmt.Errorf("OCI reference is required")
	}
	return nil
}

// Run executes the deploy command.
func (o *DeployOptions) Run() error {
	fmt.Fprintf(o.Out, "Deploying bundle to Kubernetes cluster: %s\n", o.OCIReference)
	return bundle.Deploy(o.OCIReference)
}
