// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package modify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/opencontainers/go-digest"
)

const baselineDir = "baseline"

func captureBaseline(ctx context.Context, sourceDir, architecture, flavor, packageDigest, source string) error {
	root, err := os.OpenRoot(sourceDir)
	if err != nil {
		return fmt.Errorf("opening staged source: %w", err)
	}
	defer root.Close()
	records, err := inventory(ctx, root)
	if err != nil {
		return err
	}
	metadata := disassemblyMetadata{
		FormatVersion: disassemblyFormatVersion,
		Architecture:  architecture, Flavor: flavor,
		PackageDigest: packageDigest, SourceHint: sanitizedSourceHint(source), Files: records,
	}
	if err := validateDisassemblyMetadata(metadata); err != nil {
		return err
	}
	if err := root.MkdirAll(path.Join(disassemblyMetadataDir, baselineDir), 0o700); err != nil {
		return fmt.Errorf("creating private baseline: %w", err)
	}
	for _, record := range records {
		if record.Kind != "text" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		// Recheck bytes while copying so a changed source cannot publish a
		// baseline that disagrees with its inventory.
		contents, err := readRecordedText(root, record)
		if err != nil {
			return fmt.Errorf("capturing baseline: %w", err)
		}
		name := path.Join(disassemblyMetadataDir, baselineDir, record.Path)
		if err := root.MkdirAll(path.Dir(name), 0o700); err != nil {
			return fmt.Errorf("creating baseline parent for %q: %w", record.Path, err)
		}
		if err := root.WriteFile(name, contents, 0o600); err != nil {
			return fmt.Errorf("writing baseline for %q: %w", record.Path, err)
		}
	}
	return writeDisassemblyMetadata(sourceDir, metadata)
}

func sanitizedSourceHint(source string) string {
	if strings.Contains(source, "://") {
		parsed, err := url.Parse(source)
		if err != nil {
			return ""
		}
		return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: "/" + path.Base(parsed.Path)}).String()
	}
	return filepath.Base(source)
}

func validateDisassemblyMetadata(metadata disassemblyMetadata) error {
	if metadata.FormatVersion != disassemblyFormatVersion {
		return fmt.Errorf("unsupported disassembly metadata format version %q; use a matching CLI version (diff requires a baseline captured during disassembly)", metadata.FormatVersion)
	}
	if strings.TrimSpace(metadata.Architecture) == "" {
		return errors.New("disassembly metadata architecture is required")
	}
	if err := digest.Digest(metadata.PackageDigest).Validate(); err != nil {
		return fmt.Errorf("invalid source package digest in disassembly metadata: %w", err)
	}
	if metadata.Files == nil {
		return errors.New("disassembly metadata file inventory is required; use the original disassembly metadata")
	}
	seen := make(map[string]bool, len(metadata.Files))
	previous := ""
	for _, record := range metadata.Files {
		if !validInventoryPath(record.Path) || record.Path <= previous {
			return fmt.Errorf("invalid or unsorted inventory path %q in disassembly metadata", record.Path)
		}
		for parent := path.Dir(record.Path); parent != "."; parent = path.Dir(parent) {
			if seen[parent] {
				return fmt.Errorf("inventory file %q is also a parent directory", parent)
			}
		}
		if record.Mode > 0o7777 || record.Size < 0 || digest.Digest(record.Digest).Validate() != nil || !strings.HasPrefix(record.Digest, "sha256:") {
			return fmt.Errorf("invalid mode, size, or SHA-256 digest for inventory path %q", record.Path)
		}
		switch record.Kind {
		case "text", "binary":
			if record.Target != "" {
				return fmt.Errorf("unexpected link target for inventory path %q", record.Path)
			}
		case "symlink":
			if record.Target == "" || !utf8.ValidString(record.Target) || strings.ContainsRune(record.Target, 0) || record.Size != int64(len(record.Target)) || record.Digest != contentDigest([]byte(record.Target)) {
				return fmt.Errorf("invalid symbolic link record for inventory path %q", record.Path)
			}
		default:
			return fmt.Errorf("invalid file kind %q for inventory path %q", record.Kind, record.Path)
		}
		seen[record.Path] = true
		previous = record.Path
	}
	return nil
}

func readRecordedText(root *os.Root, record FileRecord) ([]byte, error) {
	info, err := root.Lstat(record.Path)
	if err != nil {
		return nil, fmt.Errorf("reading text file %q: %w", record.Path, err)
	}
	if !info.Mode().IsRegular() || info.Size() != record.Size {
		return nil, fmt.Errorf("text file %q does not match its recorded type or size", record.Path)
	}
	file, err := root.Open(record.Path)
	if err != nil {
		return nil, fmt.Errorf("opening text file %q: %w", record.Path, err)
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, record.Size+1))
	if err != nil {
		return nil, fmt.Errorf("reading text file %q: %w", record.Path, err)
	}
	if int64(len(contents)) != record.Size || contentDigest(contents) != record.Digest || !utf8.Valid(contents) || bytes.IndexByte(contents, 0) >= 0 {
		return nil, fmt.Errorf("text file %q does not match its recorded digest, size, or classification", record.Path)
	}
	return contents, nil
}
