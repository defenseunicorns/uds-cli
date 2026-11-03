package packagecli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/zarf/modify"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
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
			return printDiffReport(streams.Out(), result)
		},
	}
}

func printDiffReport(out io.Writer, result modify.DiffResult) error {
	var report strings.Builder
	fmt.Fprintf(&report, "Package digest: %s\nSource hint: %q\nWorking inventory digest: %s\n",
		result.PackageDigest, result.SourceHint, result.WorkingTreeDigest)
	if len(result.Changes) == 0 {
		report.WriteString("\nNo changes.\n")
	}
	changes := slices.Clone(result.Changes)
	slices.SortFunc(changes, func(a, b modify.FileChange) int { return strings.Compare(a.Path, b.Path) })
	for _, change := range changes {
		fmt.Fprintf(&report, "\n%s %q\n", change.Status, change.Path)
		for _, side := range []struct {
			label  string
			record *modify.FileRecord
		}{
			{label: "Before", record: change.Before},
			{label: "After", record: change.After},
		} {
			if side.record == nil {
				fmt.Fprintf(&report, "  %s: absent\n", side.label)
				continue
			}
			record := side.record
			fmt.Fprintf(&report, "  %s: %s mode %04o, %d bytes, %s", side.label, record.Kind, record.Mode, record.Size, record.Digest)
			if record.Kind == "symlink" {
				fmt.Fprintf(&report, ", target %q", record.Target)
			}
			report.WriteByte('\n')
		}
		if change.Diff != "" {
			report.WriteString(change.Diff)
			if !strings.HasSuffix(change.Diff, "\n") {
				report.WriteByte('\n')
			}
		}
	}
	_, err := io.WriteString(out, report.String())
	return err
}
