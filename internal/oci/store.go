// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/defenseunicorns/uds-cli/internal/filesystem"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	godigest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	orasoci "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
)

// Store is a bundle OCI layout backed by ORAS content storage.
type Store struct {
	*orasoci.Store
	root string
}

// CreateStore opens a mutable OCI layout at root, creating it when necessary.
//
// Use CreateStore when the caller owns the layout and may write blobs, update tags,
// save indexes, run garbage collection, or verify/copy graphs. CreateStore opens
// the full ORAS OCI store, which indexes the layout graph while opening.
//
// Do not use CreateStore for metadata-only inspection of untrusted archives; use
// OpenReadOnlyStore with FetchBytes instead so size limits apply before blob
// bodies are read.
func CreateStore(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("OCI store root is required")
	}
	store, err := orasoci.New(root)
	if err != nil {
		return nil, fmt.Errorf("opening OCI layout %q: %w", root, err)
	}
	store.AutoSaveIndex = false
	store.AutoGC = false
	if err := os.MkdirAll(filepath.Join(root, ocispec.ImageBlobsDir, godigest.SHA256.String()), filesystem.PrivateDirectoryMode); err != nil {
		return nil, fmt.Errorf("creating OCI blob directory: %w", err)
	}
	return &Store{Store: store, root: root}, nil
}

// OpenReadOnlyStore opens an existing OCI layout as a lazy descriptor fetcher.
//
// Use OpenReadOnlyStore with FetchBytes for metadata-only local reads, such as
// inspect operations that need indexes, manifests, configs, or definition
// layers. This opener does not index the ORAS graph, so FetchBytes is the first
// code path that reads descriptor bodies and its size limit is effective.
//
// Do not use OpenReadOnlyStore when the caller needs tags, resolution, graph
// traversal, graph verification, pushes, deletes, saves, or garbage collection;
// use OpenStore or CreateStore for those full-store operations.
func OpenReadOnlyStore(root string) (content.Fetcher, error) {
	if root == "" {
		return nil, fmt.Errorf("OCI store root is required")
	}
	if _, err := os.Stat(filepath.Join(root, ocispec.ImageLayoutFile)); err != nil {
		return nil, fmt.Errorf("opening OCI layout %q: %w", root, err)
	}
	return orasoci.NewStorageFromFS(os.DirFS(root)), nil
}

// OpenStore opens an existing OCI layout as a full local store.
//
// Use OpenStore when the caller needs full OCI layout behavior: tag resolution,
// graph traversal, graph verification, copying, deleting, saving indexes, or
// pruning unreferenced blobs. OpenStore does not intentionally modify the layout,
// but it opens the full ORAS
// store, which indexes the layout graph while opening.
//
// Do not use OpenStore for metadata-only inspection of untrusted archives before
// bounded reads; use OpenReadOnlyStore with FetchBytes for that case.
func OpenStore(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("OCI store root is required")
	}
	if _, err := os.Stat(filepath.Join(root, ocispec.ImageLayoutFile)); err != nil {
		return nil, fmt.Errorf("opening OCI layout %q: %w", root, err)
	}
	store, err := orasoci.New(root)
	if err != nil {
		return nil, fmt.Errorf("opening OCI layout %q: %w", root, err)
	}
	store.AutoSaveIndex = false
	store.AutoGC = false
	return &Store{Store: store, root: root}, nil
}

// BlobPath returns the filesystem path for a content digest.
func (s *Store) BlobPath(d godigest.Digest) (string, error) {
	if err := d.Validate(); err != nil {
		return "", fmt.Errorf("invalid digest %q: %w", d, err)
	}
	return filepath.Join(s.root, ocispec.ImageBlobsDir, d.Algorithm().String(), d.Encoded()), nil
}

// Push stores verified content and treats an existing content-addressed blob as success.
func (s *Store) Push(ctx context.Context, desc ocispec.Descriptor, r io.Reader) error {
	if err := s.Store.Push(ctx, desc, r); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return err
	}
	return nil
}

// PushBytes stores data and returns its descriptor.
func (s *Store) PushBytes(ctx context.Context, mediaType string, data []byte) (ocispec.Descriptor, error) {
	return PushBytes(ctx, s, mediaType, data, nil)
}

// PruneUnreferencedBlobs removes blobs not referenced by manifests, including manifest and config blobs.
func (s *Store) PruneUnreferencedBlobs(ctx context.Context, streams iostreams.IOStreams, manifests []ocispec.Descriptor) error {
	streams.Debug("pruning unreferenced blobs", "manifests", len(manifests))
	keep, err := reachableDigests(ctx, s, manifests, false)
	if err != nil {
		return err
	}

	blobDir := filepath.Join(s.root, ocispec.ImageBlobsDir)
	if _, err := os.Stat(blobDir); err != nil {
		return fmt.Errorf("listing OCI blobs: %w", err)
	}
	return filepath.WalkDir(blobDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		encoded := entry.Name()
		algorithm := godigest.Algorithm(filepath.Base(filepath.Dir(path)))
		digest := godigest.NewDigestFromEncoded(algorithm, encoded)
		if err := digest.Validate(); err != nil {
			return fmt.Errorf("parsing OCI blob digest %s/%s: %w", algorithm, encoded, err)
		}
		if keep[digest] {
			return nil
		}
		if err := s.Delete(ctx, ocispec.Descriptor{Digest: digest}); err != nil {
			return fmt.Errorf("removing unreferenced blob %s: %w", digest, err)
		}
		return nil
	})
}

// VerifyGraph verifies the size and digest of every node reachable from roots.
//
// Manifest-like nodes are read by ORAS while discovering successors. Leaf blobs
// can be package layers and may be large, so they are streamed through Fetch and
// content.NewVerifyReader instead of using FetchBytes/content.FetchAll.
func (s *Store) VerifyGraph(ctx context.Context, roots []ocispec.Descriptor) error {
	_, err := reachableDigests(ctx, s, roots, true)
	return err
}

// VerifyLocalLayoutGraph verifies every descriptor reachable from the OCI index in root.
func VerifyLocalLayoutGraph(ctx context.Context, root string, index []byte) error {
	var parsed ocispec.Index
	if err := json.Unmarshal(index, &parsed); err != nil {
		return fmt.Errorf("parsing OCI index: %w", err)
	}
	store, err := OpenStore(root)
	if err != nil {
		return fmt.Errorf("opening OCI layout: %w", err)
	}
	return store.VerifyGraph(ctx, parsed.Manifests)
}

func reachableDigests(ctx context.Context, store content.Fetcher, roots []ocispec.Descriptor, verifyLeafContent bool) (map[godigest.Digest]bool, error) {
	queue := append([]ocispec.Descriptor(nil), roots...)
	seen := make(map[godigest.Digest]bool)
	seenSizes := make(map[godigest.Digest]int64)
	for len(queue) > 0 {
		desc := queue[0]
		queue = queue[1:]
		if size, ok := seenSizes[desc.Digest]; ok && size != desc.Size {
			return nil, fmt.Errorf("descriptor %s has conflicting sizes %d and %d", desc.Digest, size, desc.Size)
		}
		seenSizes[desc.Digest] = desc.Size
		if seen[desc.Digest] {
			continue
		}
		seen[desc.Digest] = true
		successors, err := content.Successors(ctx, store, desc)
		if err != nil {
			return nil, fmt.Errorf("reading successors of %s: %w", desc.Digest, err)
		}
		// Successors fetches and verifies manifest-like nodes so it can parse
		// their children. Descriptors with no successors may be content leaves,
		// including large package layers, or empty manifests. When full graph
		// verification is requested, verify those bytes as a stream rather than
		// buffering them with content.FetchAll.
		if verifyLeafContent && len(successors) == 0 {
			if err := verifyContent(ctx, store, desc); err != nil {
				return nil, fmt.Errorf("verifying %s: %w", desc.Digest, err)
			}
		}
		queue = append(queue, successors...)
	}
	return seen, nil
}

func verifyContent(ctx context.Context, store content.Fetcher, desc ocispec.Descriptor) (err error) {
	// Fetch returns an io.ReadCloser for the descriptor. Wrapping it in
	// VerifyReader keeps ORAS' digest and size checks while io.Copy drains the
	// stream to io.Discard, so large layer blobs are never held in memory.
	r, err := store.Fetch(ctx, desc)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, r.Close())
	}()
	vr := content.NewVerifyReader(r, desc)
	if _, err := io.Copy(io.Discard, vr); err != nil {
		return err
	}
	return vr.Verify()
}
