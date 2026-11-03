package modify

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	gitdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/utils/diff"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// DiffResult describes the baseline identity and exact inventory compared.
type DiffResult struct {
	PackageDigest     string
	SourceHint        string
	WorkingTreeDigest string
	Changes           []FileChange
}

// FileChange describes an added, deleted, or modified source path. Diff contains
// a unified patch when either side is text; absent or non-text sides have no text.
type FileChange struct {
	Path   string
	Status string
	Before *FileRecord
	After  *FileRecord
	Diff   string
}

// Diff compares source against its disassembly baseline without writing to either
// tree or accessing the original package. Invalid baselines never yield partial results.
func Diff(ctx context.Context, sourceDir string) (DiffResult, error) {
	if strings.TrimSpace(sourceDir) == "" {
		return DiffResult{}, fmt.Errorf("source directory is required")
	}
	root, err := os.OpenRoot(sourceDir)
	if err != nil {
		return DiffResult{}, fmt.Errorf("opening source directory: %w", err)
	}
	defer root.Close()
	metadata, err := readDisassemblyMetadata(root)
	if err != nil {
		return DiffResult{}, err
	}
	baseline, err := root.OpenRoot(disassemblyMetadataDir + "/" + baselineDir)
	if err != nil {
		return DiffResult{}, fmt.Errorf("opening text baseline: %w; restore the original .uds directory", err)
	}
	defer baseline.Close()
	// Validate every stored text file, including unchanged paths, before reporting
	// anything. A matching working file cannot hide an incomplete baseline.
	for _, record := range metadata.Files {
		if err := ctx.Err(); err != nil {
			return DiffResult{}, err
		}
		if record.Kind == "text" {
			if _, err := readRecordedText(baseline, record); err != nil {
				return DiffResult{}, fmt.Errorf("invalid text baseline: %w; restore the original .uds directory", err)
			}
		}
	}
	working, err := inventory(ctx, root)
	if err != nil {
		return DiffResult{}, err
	}
	workingDigest, err := inventoryDigest(working)
	if err != nil {
		return DiffResult{}, err
	}
	result := DiffResult{PackageDigest: metadata.PackageDigest, SourceHint: metadata.SourceHint, WorkingTreeDigest: workingDigest, Changes: make([]FileChange, 0)}
	for before, after := 0, 0; before < len(metadata.Files) || after < len(working); {
		if err := ctx.Err(); err != nil {
			return DiffResult{}, err
		}
		var change FileChange
		switch {
		case after == len(working) || before < len(metadata.Files) && metadata.Files[before].Path < working[after].Path:
			change = FileChange{Path: metadata.Files[before].Path, Status: "deleted", Before: &metadata.Files[before]}
			before++
		case before == len(metadata.Files) || working[after].Path < metadata.Files[before].Path:
			change = FileChange{Path: working[after].Path, Status: "added", After: &working[after]}
			after++
		default:
			old, current := &metadata.Files[before], &working[after]
			before++
			after++
			if *old == *current {
				continue
			}
			change = FileChange{Path: old.Path, Status: "modified", Before: old, After: current}
		}
		change.Diff, err = textPatch(root, baseline, change)
		if err != nil {
			return DiffResult{}, err
		}
		result.Changes = append(result.Changes, change)
	}
	return result, nil
}

func textPatch(working, baseline *os.Root, change FileChange) (string, error) {
	var oldText, newText []byte
	var err error
	from, to := "/dev/null", "/dev/null"
	if change.Before != nil && change.Before.Kind == "text" {
		oldText, err = readRecordedText(baseline, *change.Before)
		if err != nil {
			return "", fmt.Errorf("reading baseline: %w", err)
		}
		from = strconv.Quote("a/" + change.Path)
	}
	if change.After != nil && change.After.Kind == "text" {
		newText, err = readRecordedText(working, *change.After)
		if err != nil {
			return "", fmt.Errorf("source changed during comparison: %w; retry with a stable source tree", err)
		}
		to = strconv.Quote("b/" + change.Path)
	}
	if bytes.Equal(oldText, newText) {
		return "", nil
	}
	var chunks textChunks
	for _, chunk := range diff.Do(string(oldText), string(newText)) {
		chunks = append(chunks, textChunk{chunk})
	}
	var patch strings.Builder
	fmt.Fprintf(&patch, "--- %s\n+++ %s\n", from, to)
	if err := gitdiff.NewUnifiedEncoder(&patch, gitdiff.DefaultContextLines).Encode(chunks); err != nil {
		return "", fmt.Errorf("encoding text diff for %q: %w", change.Path, err)
	}
	return patch.String(), nil
}

// textChunks supplies anonymous content to Go-Git's encoder. UDS owns the quoted
// path headers and SHA-256 inventory metadata; the encoder owns only the diff body.
type textChunks []gitdiff.Chunk

func (c textChunks) FilePatches() []gitdiff.FilePatch { return []gitdiff.FilePatch{c} }
func (c textChunks) Message() string                  { return "" }
func (c textChunks) IsBinary() bool                   { return false }
func (c textChunks) Files() (gitdiff.File, gitdiff.File) {
	return nil, nil
}
func (c textChunks) Chunks() []gitdiff.Chunk { return c }

type textChunk struct {
	diffmatchpatch.Diff
}

func (c textChunk) Content() string { return c.Text }

func (c textChunk) Type() gitdiff.Operation {
	switch c.Diff.Type {
	case diffmatchpatch.DiffDelete:
		return gitdiff.Delete
	case diffmatchpatch.DiffInsert:
		return gitdiff.Add
	default:
		return gitdiff.Equal
	}
}
