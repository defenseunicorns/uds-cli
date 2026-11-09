// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package packagecli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/zarf/modify"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	gitdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/spf13/cobra"
)

// NewDiffCommand creates the package source change-report command.
func NewDiffCommand(streams iostreams.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:   "diff <source-dir>",
		Short: "Report changes against the disassembled package baseline",
		Long: "Report local source changes without the original package or network access. " +
			"The report can include sensitive baseline text, including deleted content. " +
			"Both clean and changed reports return success.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := modify.Diff(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			color := streams.IsOutTerminal() && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
			return printDiffReport(streams.Out(), result, color)
		},
	}
}

func printDiffReport(out io.Writer, result modify.DiffResult, color bool) error {
	var colors gitdiff.ColorConfig
	if color {
		colors = gitdiff.NewColorConfig()
		colors[gitdiff.Func] = colors[gitdiff.Frag]
	}
	var report strings.Builder
	fmt.Fprintf(&report, "Package source: %q\nPackage digest: %s\n",
		result.SourceHint, result.PackageDigest)
	if len(result.Changes) == 0 {
		report.WriteString("\nNo changes.\n")
	}
	for _, change := range result.Changes {
		report.WriteByte('\n')
		status := change.Status()
		title := fmt.Sprintf("%s %q", status, change.Path())
		if status == "renamed" {
			title = fmt.Sprintf("renamed %q -> %q", change.Before.Path, change.After.Path)
		}
		writeDiffLine(&report, colors, gitdiff.Meta, title)
		writeDiffMetadata(&report, colors, change)
		if change.Patch != nil {
			for header := range strings.SplitSeq(strings.TrimSuffix(change.Patch.Message(), "\n"), "\n") {
				writeDiffLine(&report, colors, gitdiff.Meta, header)
			}
			if err := gitdiff.NewUnifiedEncoder(&report, gitdiff.DefaultContextLines).SetColor(colors).Encode(patchBody{change.Patch}); err != nil {
				return fmt.Errorf("encoding text diff for %q: %w", change.Path(), err)
			}
		}
	}
	// Keep Go-Git's missing-newline annotation uncolored.
	text := strings.ReplaceAll(report.String(), "\n\\ No newline at end of file", colors.Reset(gitdiff.New)+"\n\\ No newline at end of file")
	_, err := io.WriteString(out, text)
	return err
}

func writeDiffMetadata(report *strings.Builder, colors gitdiff.ColorConfig, change modify.FileChange) {
	before, after := change.Before, change.After
	paired := before != nil && after != nil
	typeChanged := paired && before.Kind != after.Kind
	if typeChanged {
		writeDiffLine(report, colors, gitdiff.Meta, "old type "+before.Kind)
		writeDiffLine(report, colors, gitdiff.Meta, "new type "+after.Kind)
	}
	for _, side := range []struct {
		prefix string
		record *modify.FileRecord
	}{
		{prefix: "old ", record: before},
		{prefix: "new ", record: after},
	} {
		record := side.record
		if record == nil {
			continue
		}
		prefix := side.prefix
		if !paired {
			prefix = ""
		}
		if record.Kind == "binary" {
			if !paired || typeChanged || before.Size != after.Size {
				writeDiffLine(report, colors, gitdiff.Meta, fmt.Sprintf("%ssize %d bytes", prefix, record.Size))
			}
			if !paired || typeChanged || before.Digest != after.Digest {
				writeDiffLine(report, colors, gitdiff.Meta, prefix+record.Digest)
			}
		}
		if record.Kind == "symlink" && (!paired || typeChanged || before.Target != after.Target) {
			writeDiffLine(report, colors, gitdiff.Meta, fmt.Sprintf("%starget %q", prefix, record.Target))
		}
	}
	if change.Status() == "renamed" && paired && before.Kind == "binary" && after.Kind == "binary" && before.Digest == after.Digest {
		writeDiffLine(report, colors, gitdiff.Meta, before.Digest)
	}
}

type patchBody struct{ gitdiff.Patch }

func (patchBody) Message() string { return "" }

func writeDiffLine(report *strings.Builder, colors gitdiff.ColorConfig, key gitdiff.ColorKey, line string) {
	report.WriteString(colors[key])
	report.WriteString(line)
	report.WriteString(colors.Reset(key))
	report.WriteByte('\n')
}
