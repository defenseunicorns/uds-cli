// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	godigest "github.com/opencontainers/go-digest"
)

// parseDigest parses a digest string (e.g. "sha256:<hex>") into a digest.Digest.
// This is the same type used by ocispec.Descriptor.Digest throughout the OCI ecosystem.
func parseDigest(s string) (godigest.Digest, error) {
	d, err := godigest.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid digest %q: %w", s, err)
	}
	return d, nil
}

// sha256Digest creates a digest.Digest from a sha256 hex string.
func sha256Digest(hexStr string) godigest.Digest {
	return godigest.NewDigestFromEncoded(godigest.SHA256, hexStr)
}

// writeBlobBytesIfMissingAndVerify atomically writes b to blobDir, verifying
// the content matches the expected digest. Skips if already exists.
func writeBlobBytesIfMissingAndVerify(blobDir string, d godigest.Digest, b []byte) error {
	verifier := d.Verifier()
	if _, err := verifier.Write(b); err != nil {
		return fmt.Errorf("computing digest for %s: %w", d, err)
	}
	if !verifier.Verified() {
		return fmt.Errorf("digest mismatch for %s", d)
	}
	dst := filepath.Join(blobDir, d.Encoded())
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, tmpFilePerm)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := f.Write(b)
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(dst)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return closeErr
	}
	return nil
}

// writeBlobReaderIfMissingAndVerify streams content from r into blobDir as a
// content-addressed blob, verifying the digest matches. Skips if the blob
// already exists. This is the core streaming blob writer — other blob-writing
// functions delegate to it.
func writeBlobReaderIfMissingAndVerify(blobDir string, d godigest.Digest, r io.Reader) error {
	dst := filepath.Join(blobDir, d.Encoded())
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking blob %s: %w", d, err)
	}

	tmp, err := os.CreateTemp(blobDir, ".blob-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	verifier := d.Verifier()
	_, copyErr := io.Copy(io.MultiWriter(tmp, verifier), r)
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	if !verifier.Verified() {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("digest mismatch for %s", d)
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		_ = os.Remove(tmpPath)
		if _, statErr := os.Stat(dst); statErr == nil {
			return nil // blob was written by a concurrent writer
		}
		return err
	}
	return nil
}

// copyBlobFileIfMissingAndVerify copies a blob unless verified content already exists.
func copyBlobFileIfMissingAndVerify(dstBlobDir, srcPath string, d godigest.Digest) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	return writeBlobReaderIfMissingAndVerify(dstBlobDir, d, in)
}

// writeAndDigestBlob computes the sha256 digest of data, writes it to blobDir,
// and returns the digest. Combines the common pattern of hash + write + return digest.
func writeAndDigestBlob(blobDir string, data []byte) (godigest.Digest, error) {
	sum := sha256.Sum256(data)
	d := sha256Digest(hex.EncodeToString(sum[:]))
	if err := writeBlobBytesIfMissingAndVerify(blobDir, d, data); err != nil {
		return "", err
	}
	return d, nil
}

// gcUnreferencedBlobs removes blobs from blobDir that are not referenced by
// any of the provided manifests (their manifest blob, config blob, and layer blobs).
// Call this after all optional-component filtering is complete so that excluded
// component layers are not shipped in the final bundle.
func gcUnreferencedBlobs(_ context.Context, streams iostreams.IOStreams, blobDir string, manifests []OciManifest) error {
	streams.Debug("garbage collecting unreferenced blobs", "manifests", len(manifests))
	keep := make(map[string]bool)
	for _, m := range manifests {
		md, err := parseDigest(m.Digest)
		if err != nil {
			return err
		}
		keep[md.Encoded()] = true

		data, err := os.ReadFile(filepath.Join(blobDir, md.Encoded()))
		if err != nil {
			return err
		}
		var im OciImageManifest
		if err := json.Unmarshal(data, &im); err != nil {
			return err
		}
		if im.Config.Digest != "" {
			cd, err := parseDigest(im.Config.Digest)
			if err != nil {
				return fmt.Errorf("invalid config digest in manifest %s: %w", md.Encoded(), err)
			}
			keep[cd.Encoded()] = true
		}
		for _, l := range im.Layers {
			ld, err := parseDigest(l.Digest)
			if err != nil {
				return fmt.Errorf("invalid layer digest in manifest %s: %w", md.Encoded(), err)
			}
			keep[ld.Encoded()] = true
		}
	}

	entries, err := os.ReadDir(blobDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if !keep[e.Name()] {
			if err := os.Remove(filepath.Join(blobDir, e.Name())); err != nil {
				streams.Warn("failed to remove unreferenced blob", "blob", e.Name(), "error", err)
			}
		}
	}
	return nil
}
