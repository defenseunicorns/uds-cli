// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/artifact"
	bundleinternal "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/internal/cli/util"
	"github.com/defenseunicorns/uds-cli/internal/logger"
	udsoci "github.com/defenseunicorns/uds-cli/internal/oci"
	"github.com/defenseunicorns/uds-cli/internal/printer"
	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

const bundleSignatureNotCheckedWarning = "bundle signature verification was not performed; package verification policy and signing metadata do not establish bundle integrity"

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
	if err := bundleinternal.ValidateConfig(toInternalConfig(o.Config)); err != nil {
		return err
	}
	if strings.TrimSpace(o.BundlePath) == "" {
		return fmt.Errorf("source must not be empty")
	}
	if !isOCIReference(o.BundlePath) && !isTarZst(o.BundlePath) {
		return fmt.Errorf("source must be a .tar.zst bundle artifact or OCI reference")
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
			return fmt.Errorf("bundle artifact not found: %s", o.BundlePath)
		}
		return fmt.Errorf("cannot access bundle artifact %s: %w", o.BundlePath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("bundle artifact path is a directory: %s", o.BundlePath)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("bundle artifact path is not a regular file: %s", o.BundlePath)
	}
	return nil
}

// Run executes the inspect command.
func (o *InspectOptions) Run(ctx context.Context) error {
	o.IOStreams = logger.Bind(o.IOStreams, o.Config.Options.LogLevel)
	o.Debug("inspecting bundle", "source", o.BundlePath)
	verified := false
	if o.verificationRequested() {
		policy, err := o.Verification.policy()
		if err != nil {
			return err
		}
		if err := bundle.Verify(ctx, bundle.VerifyOptions{Source: o.BundlePath, Policy: policy, Config: o.Config, TmpDir: o.Config.Options.TmpDir, Streams: o.IOStreams}); err != nil {
			return fmt.Errorf("verifying bundle: %w", err)
		}
		verified = true
	}

	internalResult, err := artifact.Inspect(ctx, artifact.InspectOptions{
		Source:  o.BundlePath,
		Config:  toInternalConfig(o.Config),
		Streams: o.IOStreams,
	})
	if err != nil {
		return fmt.Errorf("inspecting bundle: %w", err)
	}
	result := &inspectResult{
		Name: internalResult.Bundle.Metadata.Name, Description: internalResult.Bundle.Metadata.Description,
		Version: internalResult.Bundle.Metadata.Version, ArtifactDigest: internalResult.ArtifactDigest,
		ReconfiguredFrom: internalResult.ReconfiguredFrom,
		BundleSignature:  &bundleSignatureSummary{Status: "not_checked"}, Packages: make([]packageSummary, len(internalResult.Packages)),
	}
	if verified {
		result.BundleSignature.Status = bundle.BundleSignatureStatusVerified
	}
	for i, pkg := range internalResult.Packages {
		summary := internalResult.PackageSignatures[pkg.Name]
		dependsOn := make([]string, len(pkg.DependsOn))
		for j, dependency := range pkg.DependsOn {
			dependsOn[j] = dependency.Name
		}
		result.Packages[i] = packageSummary{
			Name: pkg.Name, Source: pkg.Source, Namespace: pkg.Namespace, DependsOn: dependsOn, ValuesFiles: pkg.ValuesFiles,
			Signature: &packageSignatureSummary{Signed: signingStatus(summary.Signed), Verification: verificationStatus(summary.Verification)},
		}
	}
	o.Warn(bundleSignatureNotCheckedWarning)

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

func signingStatus(status artifact.PackageSigningStatus) string {
	switch status {
	case artifact.PackageSigningStatusSigned:
		return "signed"
	case artifact.PackageSigningStatusUnsigned:
		return "unsigned"
	default:
		return "unknown"
	}
}

func verificationStatus(status artifact.PackageVerificationStatus) string {
	switch status {
	case artifact.PackageVerificationStatusVerified:
		return "verified"
	case artifact.PackageVerificationStatusSkipped:
		return "skipped"
	default:
		return "unknown"
	}
}

type inspectResult struct {
	Name             string                  `json:"name" yaml:"name" text:"Name"`
	Description      string                  `json:"description,omitempty" yaml:"description,omitempty" text:"Description,omitempty"`
	Version          string                  `json:"version,omitempty" yaml:"version,omitempty" text:"Version,omitempty"`
	ArtifactDigest   string                  `json:"artifactDigest,omitempty" yaml:"artifactDigest,omitempty" text:"Artifact Digest,omitempty"`
	ReconfiguredFrom string                  `json:"reconfiguredFrom,omitempty" yaml:"reconfiguredFrom,omitempty" text:"Reconfigured From,omitempty"`
	BundleSignature  *bundleSignatureSummary `json:"bundleSignature,omitempty" yaml:"bundleSignature,omitempty" text:"Bundle Signature,omitempty"`
	Packages         []packageSummary        `json:"packages" yaml:"packages" text:"Packages"`
}

type packageSummary struct {
	Name        string                   `json:"name" yaml:"name" text:"Name"`
	Source      string                   `json:"source" yaml:"source" text:"Source"`
	Namespace   string                   `json:"namespace,omitempty" yaml:"namespace,omitempty" text:"Namespace,omitempty"`
	DependsOn   []string                 `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty" text:"DependsOn,omitempty"`
	ValuesFiles []string                 `json:"valuesFiles,omitempty" yaml:"valuesFiles,omitempty" text:"Value Files,omitempty"`
	Signature   *packageSignatureSummary `json:"signature,omitempty" yaml:"signature,omitempty" text:"Signature,omitempty"`
}

type bundleSignatureSummary struct {
	Status string `json:"status" yaml:"status" text:"Status"`
}

type packageSignatureSummary struct {
	Signed       string `json:"signed" yaml:"signed" text:"Signed"`
	Verification string `json:"verification" yaml:"verification" text:"Verification Posture"`
}

func toInternalConfig(cfg *bundle.UDSBundleConfig) *bundleinternal.UDSBundleConfig {
	if cfg == nil {
		return nil
	}
	var options *bundleinternal.ConfigOptions
	if cfg.Options != nil {
		options = &bundleinternal.ConfigOptions{LogLevel: cfg.Options.LogLevel, Architecture: cfg.Options.Architecture, PlainHTTP: cfg.Options.PlainHTTP, SkipTLSVerify: cfg.Options.SkipTLSVerify, TmpDir: cfg.Options.TmpDir, Concurrency: cfg.Options.Concurrency}
	}
	return &bundleinternal.UDSBundleConfig{Options: options, Variables: bundleinternal.Variables(cfg.Variables)}
}
