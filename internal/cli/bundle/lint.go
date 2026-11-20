// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/bundle/analysis"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

// LintOptions holds CLI inputs for checking a saved bundle definition.
type LintOptions struct {
	Path string
	analysis.Options
	iostreams.IOStreams
}

// NewLintCommand creates the single-bundle source lint command.
func NewLintCommand(streams iostreams.IOStreams) *cobra.Command {
	o := &LintOptions{IOStreams: streams}
	cmd := &cobra.Command{
		Use:   "lint [bundle-definition]",
		Short: "Check a bundle definition and its values templates",
		Long: `Check a saved bundle definition without building or deploying it.

Accepts a directory containing bundle.uds.hcl or a direct definition path;
defaults to the current directory. Nested bundles are not searched.
Adjacent defaults.uds.hcl loads automatically; config loads only with --config.
When variables are configured, lint checks and executes values templates.
Use --skip-template-evaluation to check template syntax without executing it.
Lint prints text output. Output-format, registry, cache, temporary-directory,
concurrency, and prompt flags are rejected when supplied.`,
		Args: cobra.MaximumNArgs(1),
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
	cmd.Flags().StringArrayVarP(&o.Variables, "set", "s", nil, "set a variable using key=value (repeatable)")
	cmd.Flags().BoolVar(&o.SkipTemplateEvaluation, "skip-template-evaluation", false, "skip values-template execution, retaining syntax and other local checks")
	return cmd
}

// Complete collects CLI-only settings without resolving deployment configuration.
func (o *LintOptions) Complete(cmd *cobra.Command, args []string) error {
	o.Path = "."
	if len(args) > 0 {
		o.Path = args[0]
	}
	if flag := cmd.Flag("config"); flag != nil {
		o.ConfigPath = flag.Value.String()
		if flag.Changed && strings.TrimSpace(o.ConfigPath) == "" {
			return fmt.Errorf("--config must not be empty: %w", ErrInvalidArgument)
		}
	}
	if flag := cmd.Flag("architecture"); flag != nil && flag.Changed {
		o.Architecture = flag.Value.String()
	}
	for _, name := range []string{"output", "concurrency", "uds-cache", "tmp-dir", "plain-http", "skip-tls-verify", "prompt"} {
		if flag := cmd.Flag(name); flag != nil && flag.Changed {
			return fmt.Errorf("--%s is not supported by bundle lint: %w", name, ErrInvalidArgument)
		}
	}
	return nil
}

// Validate checks lint-specific CLI inputs.
func (o *LintOptions) Validate() error {
	if strings.TrimSpace(o.Path) == "" {
		return fmt.Errorf("bundle path must not be empty: %w", ErrInvalidArgument)
	}
	return ValidateBundlePath(o.Path)
}

// Run checks sources, prints their diagnostics, and returns a failure when found.
func (o *LintOptions) Run(ctx context.Context) error {
	diagnostics, err := analysis.Lint(ctx, o.Path, o.Options)
	if err != nil {
		return err
	}
	if len(diagnostics) == 0 {
		_, err := fmt.Fprintln(o.Out(), "No issues found.")
		return err
	}
	cwd, err := os.Getwd()
	for _, d := range diagnostics {
		path := d.Path
		if err == nil && filepath.IsAbs(path) && filepath.Clean(path) == path {
			// Use absolute paths outside the working directory so a leading
			// parent segment does not depend on how the caller entered it.
			if rel, relErr := filepath.Rel(cwd, path); relErr == nil && filepath.IsLocal(rel) {
				path = rel
			}
		}
		location := filepath.ToSlash(path)
		if d.Line > 0 {
			location += fmt.Sprintf(":%d", d.Line)
			if d.Column > 0 {
				location += fmt.Sprintf(":%d", d.Column)
			}
		}
		if _, err := fmt.Fprintf(o.Out(), "%s: %s\n", location, d.Message); err != nil {
			return fmt.Errorf("write lint result: %w", err)
		}
	}
	return fmt.Errorf("lint found %d error(s): %w", len(diagnostics), ErrLintFailed)
}
