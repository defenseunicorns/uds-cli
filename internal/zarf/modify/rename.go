package modify

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

const textRenameLimit = 100

func detectRenames(ctx context.Context, changes []FileChange, texts map[string][]byte) ([]FileChange, error) {
	type identity struct {
		kind, digest string
		size         int64
	}
	type candidates struct{ deleted, added []int }
	exact := make(map[identity]*candidates)
	byPath := make(map[string]int)
	var textDeleted, textAdded int
	for i, change := range changes {
		if change.Before != nil && change.After != nil {
			continue
		}
		record := change.Before
		if record == nil {
			record = change.After
		}
		byPath[record.Path] = i
		if record.Kind == "text" {
			if change.Before != nil {
				textDeleted++
			} else {
				textAdded++
			}
		}
		key := identity{kind: record.Kind, digest: record.Digest, size: record.Size}
		if exact[key] == nil {
			exact[key] = &candidates{}
		}
		if change.Before != nil {
			exact[key].deleted = append(exact[key].deleted, i)
		} else {
			exact[key].added = append(exact[key].added, i)
		}
	}
	mates := make(map[int]int)
	for key, group := range exact {
		// Binary and link duplicates do not establish which path was renamed.
		if key.kind != "text" && (len(group.deleted) != 1 || len(group.added) != 1) {
			continue
		}
		// Inventory order pairs duplicate text deterministically without building
		// Go-Git's quadratic exact-match name-similarity matrix.
		for i := range min(len(group.deleted), len(group.added)) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			before, after := group.deleted[i], group.added[i]
			mates[before], mates[after] = after, before
			if key.kind == "text" {
				textDeleted--
				textAdded--
			}
		}
	}
	if textDeleted > 0 && textAdded > 0 && max(textDeleted, textAdded) <= textRenameLimit {
		store := memory.NewStorage()
		var candidates object.Changes
		for i, change := range changes {
			if _, matched := mates[i]; matched {
				continue
			}
			name := change.Path()
			contents, ok := texts[name]
			if !ok {
				continue
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			entry, err := textRenameEntry(store, name, contents)
			if err != nil {
				return nil, fmt.Errorf("preparing text rename candidate %q: %w", name, err)
			}
			candidate := &object.Change{}
			if change.Before != nil {
				candidate.From = entry
			} else {
				candidate.To = entry
			}
			candidates = append(candidates, candidate)
		}
		options := *object.DefaultDiffTreeOptions
		// Exact matches are already paired; only unmatched text reaches Go-Git's
		// bounded approximate comparison.
		options.RenameLimit = textRenameLimit
		detected, err := object.DetectRenames(candidates, &options)
		if err != nil {
			return nil, fmt.Errorf("detecting text renames: %w", err)
		}
		for _, change := range detected {
			if change.From.Name != "" && change.To.Name != "" {
				before, after := byPath[change.From.Name], byPath[change.To.Name]
				mates[before], mates[after] = after, before
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]FileChange, 0, len(changes)-len(mates)/2)
	// Renames keep the deleted path's position, preserving inventory order.
	for i, change := range changes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if after, ok := mates[i]; ok {
			if change.Before == nil {
				continue
			}
			change.After = changes[after].After
		}
		if change.Status() != "modified" {
			var oldText, newText []byte
			if change.Before != nil {
				oldText = texts[change.Before.Path]
			}
			if change.After != nil {
				newText = texts[change.After.Path]
			}
			var err error
			change.Patch, err = textPatch(ctx, change, oldText, newText)
			if err != nil {
				return nil, fmt.Errorf("comparing text for %q: %w", change.Path(), err)
			}
		}
		result = append(result, change)
	}
	return result, nil
}

func textRenameEntry(store *memory.Storage, name string, contents []byte) (object.ChangeEntry, error) {
	blob := &plumbing.MemoryObject{}
	blob.SetType(plumbing.BlobObject)
	if _, err := blob.Write(contents); err != nil {
		return object.ChangeEntry{}, err
	}
	hash, err := store.SetEncodedObject(blob)
	if err != nil {
		return object.ChangeEntry{}, err
	}
	// The tree supplies content to Go-Git; ChangeEntry.Name keeps the inventory
	// path. Regular mode makes matching independent of the real permissions.
	entry := object.TreeEntry{Name: "content", Mode: filemode.Regular, Hash: hash}
	tree := &object.Tree{Entries: []object.TreeEntry{entry}}
	encoded := &plumbing.MemoryObject{}
	if err := tree.Encode(encoded); err != nil {
		return object.ChangeEntry{}, err
	}
	parent, err := object.DecodeTree(store, encoded)
	if err != nil {
		return object.ChangeEntry{}, err
	}
	return object.ChangeEntry{Name: name, Tree: parent, TreeEntry: entry}, nil
}
