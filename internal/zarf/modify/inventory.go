// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package modify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
	"unicode/utf8"
)

// FileRecord identifies one source file or symbolic link. Mode contains Unix
// permission and special bits; a link's size and digest describe its target string.
type FileRecord struct {
	Path   string `json:"path"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size"`
	Kind   string `json:"kind"`
	Digest string `json:"digest"`
	Target string `json:"target,omitempty"`
}

func inventory(ctx context.Context, root *os.Root) ([]FileRecord, error) {
	records := make([]FileRecord, 0)
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if excludedPath(name) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !validInventoryPath(name) {
			return fmt.Errorf("source path %q cannot be recorded; use a UTF-8 relative path without backslashes", name)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		record := FileRecord{Path: name, Mode: uint32(info.Mode().Perm())}
		if info.Mode()&os.ModeSetuid != 0 {
			record.Mode |= 0o4000
		}
		if info.Mode()&os.ModeSetgid != 0 {
			record.Mode |= 0o2000
		}
		if info.Mode()&os.ModeSticky != 0 {
			record.Mode |= 0o1000
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			record.Kind = "symlink"
			record.Target, err = root.Readlink(name)
			if err == nil && !utf8.ValidString(record.Target) {
				err = fmt.Errorf("symbolic link target is not valid UTF-8; use a UTF-8 target so its identity can be recorded")
			}
			record.Size = int64(len(record.Target))
			record.Digest = contentDigest([]byte(record.Target))
		case info.Mode().IsRegular():
			record.Kind, record.Digest, record.Size, err = inventoryFile(ctx, root, name)
		default:
			err = fmt.Errorf("unsupported file type %s", info.Mode().Type())
		}
		if err != nil {
			return fmt.Errorf("inventorying %q: %w", name, err)
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inventorying source tree: %w", err)
	}
	// WalkDir visits directories lexically, but a directory's children can sort
	// after its siblings (for example, a/z and a.txt). The persisted order is global.
	slices.SortFunc(records, func(a, b FileRecord) int { return strings.Compare(a.Path, b.Path) })
	return records, nil
}

func excludedPath(name string) bool {
	for part := range strings.SplitSeq(name, "/") {
		if part == ".uds" || part == ".git" {
			return true
		}
	}
	return false
}

func inventoryFile(ctx context.Context, root *os.Root, name string) (kind, digest string, size int64, err error) {
	file, err := root.Open(name)
	if err != nil {
		return "", "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	var classifier textClassifier
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", "", 0, err
		}
		n, readErr := file.Read(buffer)
		if _, err := hash.Write(buffer[:n]); err != nil {
			return "", "", 0, err
		}
		classifier.add(buffer[:n])
		size += int64(n)
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", "", 0, readErr
		}
	}
	kind = "text"
	if classifier.binary || len(classifier.tail) > 0 {
		kind = "binary"
	}
	return kind, fmt.Sprintf("sha256:%x", hash.Sum(nil)), size, nil
}

// Text is valid UTF-8 without NUL bytes, over the entire file. A partial rune
// survives chunk boundaries; binary content is hashed without retaining it.
type textClassifier struct {
	tail   []byte
	binary bool
}

func (c *textClassifier) add(p []byte) {
	if c.binary {
		return
	}
	data := p
	if len(c.tail) > 0 {
		data = append(c.tail, p...)
	}
	end := len(data)
	for end > 0 && len(data)-end < utf8.UTFMax && !utf8.RuneStart(data[end-1]) {
		end--
	}
	if end > 0 && !utf8.FullRune(data[end-1:]) {
		end--
	} else {
		end = len(data)
	}
	c.binary = bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data[:end])
	c.tail = bytes.Clone(data[end:])
}

func contentDigest(contents []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(contents))
}

func inventoryDigest(records []FileRecord) (string, error) {
	contents, err := json.Marshal(records)
	if err != nil {
		return "", fmt.Errorf("encoding working inventory: %w", err)
	}
	return contentDigest(contents), nil
}

func validInventoryPath(name string) bool {
	return fs.ValidPath(name) && utf8.ValidString(name) && name != "." && !strings.Contains(name, "\\") && !excludedPath(name)
}
