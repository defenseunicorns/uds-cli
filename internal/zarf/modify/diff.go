// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

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

// DiffResult describes the source package identity and changes from its baseline.
// Changes are sorted by path.
type DiffResult struct {
	PackageDigest string
	SourceHint    string
	Changes       []FileChange
}

// FileChange describes an added, deleted, modified, or renamed source path.
// At least one of Before and After must be present; they retain the original
// and current paths and metadata.
// Patch contains unified text changes, or nil when the text is unchanged.
type FileChange struct {
	Before *FileRecord
	After  *FileRecord
	Patch  gitdiff.Patch
}

// Path returns the original path when present, otherwise the added path.
func (c FileChange) Path() string {
	if c.Before != nil {
		return c.Before.Path
	}
	return c.After.Path
}

// Status reports whether the file was added, deleted, modified, or renamed.
func (c FileChange) Status() string {
	switch {
	case c.Before == nil:
		return "added"
	case c.After == nil:
		return "deleted"
	case c.Before.Path != c.After.Path:
		return "renamed"
	default:
		return "modified"
	}
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
	working, err := inventory(ctx, root)
	if err != nil {
		return DiffResult{}, err
	}
	result := DiffResult{PackageDigest: metadata.PackageDigest, SourceHint: metadata.SourceHint}
	// Keep unmatched text for rename detection and patch encoding without
	// rereading either tree.
	texts := make(map[string][]byte)
	for before, after := 0, 0; before < len(metadata.Files) || after < len(working); {
		if err := ctx.Err(); err != nil {
			return DiffResult{}, err
		}
		var change FileChange
		switch {
		case after == len(working) || before < len(metadata.Files) && metadata.Files[before].Path < working[after].Path:
			change = FileChange{Before: &metadata.Files[before]}
			before++
		case before == len(metadata.Files) || working[after].Path < metadata.Files[before].Path:
			change = FileChange{After: &working[after]}
			after++
		default:
			old, current := &metadata.Files[before], &working[after]
			before++
			after++
			change = FileChange{Before: old, After: current}
		}
		// Validate every stored text file, including unchanged paths, before reporting
		// anything. A matching working file cannot hide an incomplete baseline.
		var oldText, newText []byte
		if change.Before != nil && change.Before.Kind == "text" {
			oldText, err = readRecordedText(baseline, *change.Before)
			if err != nil {
				return DiffResult{}, fmt.Errorf("invalid text baseline: %w; restore the original .uds directory", err)
			}
		}
		if change.Before != nil && change.After != nil && *change.Before == *change.After {
			continue
		}
		if change.After != nil && change.After.Kind == "text" {
			newText, err = readRecordedText(root, *change.After)
			if err != nil {
				return DiffResult{}, fmt.Errorf("source changed during comparison: %w; retry with a stable source tree", err)
			}
		}
		switch {
		case change.Before == nil:
			if change.After.Kind == "text" {
				texts[change.After.Path] = newText
			}
		case change.After == nil:
			if change.Before.Kind == "text" {
				texts[change.Before.Path] = oldText
			}
		default:
			change.Patch = textPatch(change, oldText, newText)
		}
		result.Changes = append(result.Changes, change)
	}
	result.Changes, err = detectRenames(ctx, result.Changes, texts)
	if err != nil {
		return DiffResult{}, err
	}
	return result, nil
}

func textPatch(change FileChange, oldText, newText []byte) gitdiff.Patch {
	if bytes.Equal(oldText, newText) {
		return nil
	}
	from, to := "/dev/null", "/dev/null"
	if change.Before != nil && change.Before.Kind == "text" {
		from = strconv.Quote("a/" + change.Before.Path)
	}
	if change.After != nil && change.After.Kind == "text" {
		to = strconv.Quote("b/" + change.After.Path)
	}
	patch := textChunks{header: fmt.Sprintf("--- %s\n+++ %s\n", from, to)}
	for _, chunk := range diff.Do(string(oldText), string(newText)) {
		patch.chunks = append(patch.chunks, textChunk{chunk})
	}
	return patch
}

// textChunks supplies anonymous content to Go-Git's encoder. UDS owns the quoted
// path headers and SHA-256 inventory metadata; the encoder owns only the diff body.
type textChunks struct {
	header string
	chunks []gitdiff.Chunk
}

func (c textChunks) FilePatches() []gitdiff.FilePatch { return []gitdiff.FilePatch{c} }
func (c textChunks) Message() string                  { return c.header }
func (c textChunks) IsBinary() bool                   { return false }
func (c textChunks) Files() (gitdiff.File, gitdiff.File) {
	return nil, nil
}
func (c textChunks) Chunks() []gitdiff.Chunk { return c.chunks }

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
