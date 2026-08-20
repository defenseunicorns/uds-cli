// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// InspectOptions holds the options for the inspect command.
type InspectOptions struct {
	BundlePath   string // Path to a .tar.zst artifact or OCI reference
	Config       *bundle.UDSBundleConfig
	Printer      printer.ResourcePrinter
	Verification VerifyOptions

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
		Use:   "inspect <bundle-reference>",
		Short: "Inspect a UDS bundle",
		Long:  "Inspect a built UDS bundle from a local .tar.zst artifact or OCI reference, displaying metadata and package details.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			util.CheckErr(o.Complete(cmd, args))
			util.CheckErr(o.Validate())
			ctx := cmd.Context()
			util.CheckErr(o.Run(ctx))
		},
	}
	addVerificationFlags(cmd, &o.Verification, true)

	return cmd
}

// Complete fills in options from command line args.
func (o *InspectOptions) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.BundlePath = args[0]
	} else {
		o.BundlePath = ""
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	flags := SnapshotFlags(cmd)
	// Use the embedded definition; skip sibling defaults.
	cfg, _, err := NewConfigResolver().Resolve(ctx, o.IOStreams, flags, "")
	if err != nil {
		return err
	}
	o.Config = cfg
	o.Verification.Config = cfg
	o.Verification.Source = o.BundlePath

	p, err := ResolvePrinter(cmd)
	if err != nil {
		return err
	}
	o.Printer = p

	return nil
}

// Validate validates the options without modifying state.
func (o *InspectOptions) Validate() error {
	policy := bundle.VerificationPolicy{}
	if o.verificationRequested() {
		var err error
		policy, err = o.Verification.policy()
		if err != nil {
			return err
		}
	}
	if err := (bundle.InspectOptions{
		Source:                    o.BundlePath,
		Config:                    o.Config,
		Verification:              policy,
		SkipSignatureVerification: o.Verification.SkipSignatureVerification,
	}).Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(o.BundlePath) == "" {
		return fmt.Errorf("source must not be empty: %w", ErrInvalidArgument)
	}
	if !isOCIReference(o.BundlePath) && !isTarZst(o.BundlePath) {
		return fmt.Errorf("source must be a .tar.zst bundle artifact or OCI reference: %w", ErrUnsupportedSource)
	}
	if isOCIReference(o.BundlePath) {
		if _, err := udsoci.ReferenceIdentifier(o.BundlePath); err != nil {
			return err
		}
		return nil
	}
	if o.verificationRequested() {
		if _, err := o.Verification.policy(); err != nil {
			return err
		}
	}
	info, err := os.Stat(o.BundlePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("bundle artifact not found: %s: %w: %w", o.BundlePath, ErrPathNotFound, err)
		}
		return fmt.Errorf("cannot access bundle artifact %s: %w: %w", o.BundlePath, ErrInvalidPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("bundle artifact path is a directory: %s: %w", o.BundlePath, ErrInvalidPath)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("bundle artifact path is not a regular file: %s: %w", o.BundlePath, ErrInvalidPath)
	}
	return nil
}

// Run executes the inspect command.
func (o *InspectOptions) Run(ctx context.Context) error {
	policy := bundle.VerificationPolicy{}
	o.IOStreams = logger.Bind(o.IOStreams, o.Config.Options.LogLevel)
	o.Info("inspecting bundle", "source", o.BundlePath)
	if o.verificationRequested() {
		var err error
		policy, err = o.Verification.policy()
		if err != nil {
			return err
		}
	}
	result, err := bundle.Inspect(ctx, bundle.InspectOptions{
		Source:                    o.BundlePath,
		Config:                    o.Config,
		Verification:              policy,
		SkipSignatureVerification: o.Verification.SkipSignatureVerification,
		Streams:                   o.IOStreams,
	})
	if err != nil {
		return err
	}
	return o.Printer.PrintObj(result, o.Out())
}

func (o *InspectOptions) verificationRequested() bool {
	if o.Verification.SkipSignatureVerification {
		return false
	}
	return o.Verification.PublicKey != "" || o.Verification.Identity != "" || o.Verification.IdentityRE != "" ||
		o.Verification.Issuer != "" || o.Verification.IssuerRE != "" || o.Verification.TrustedRoot != "" ||
		(o.Config != nil && o.Config.SignatureVerification != nil)
}

type inspectResult = bundle.InspectResult
