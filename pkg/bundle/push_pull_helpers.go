// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	oraci "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
)

// resolvePushTarget picks the ORAS destination: the ToOrasTarget hook if set,
// otherwise a live registry repository.
func resolvePushTarget(ctx context.Context, ref string, opts *PushOptions) (oras.Target, error) {
	ref = TrimScheme(ref)
	if opts.PushHooks.ToOrasTarget != nil {
		return opts.PushHooks.ToOrasTarget(ctx, ref, opts)
	}
	return newRemoteRepository(ctx, ref, *opts.Config.Options)
}

// resolvePullTarget picks the ORAS source: the ToOrasTarget hook if set,
// otherwise a live registry repository.
func resolvePullTarget(ctx context.Context, ref string, opts *PullOptions) (oras.Target, error) {
	ref = TrimScheme(ref)
	if opts.PullHooks.ToOrasTarget != nil {
		return opts.PullHooks.ToOrasTarget(ctx, ref, opts)
	}
	return newRemoteRepository(ctx, ref, *opts.Config.Options)
}

// pushCopyOptions builds copy options: concurrency from config, then consumer hook.
func pushCopyOptions(ctx context.Context, opts *PushOptions) (oras.CopyOptions, error) {
	co := oras.DefaultCopyOptions
	co.Concurrency = opts.Config.Options.Concurrency
	if opts.PushHooks.ModifyOrasSettings != nil {
		if err := opts.PushHooks.ModifyOrasSettings(ctx, &co); err != nil {
			return co, err
		}
	}
	return co, nil
}

// pullCopyOptions builds copy options: concurrency from config, then consumer hook.
func pullCopyOptions(ctx context.Context, opts *PullOptions) (oras.CopyOptions, error) {
	co := oras.DefaultCopyOptions
	co.Concurrency = opts.Config.Options.Concurrency
	if opts.PullHooks.ModifyOrasSettings != nil {
		if err := opts.PullHooks.ModifyOrasSettings(ctx, &co); err != nil {
			return co, err
		}
	}
	return co, nil
}

// refIdentifier returns the tag/digest portion of an OCI reference.
func refIdentifier(ref string) (string, error) {
	r, err := name.ParseReference(TrimScheme(ref))
	if err != nil {
		return "", fmt.Errorf("parsing OCI reference: %w", err)
	}
	return r.Identifier(), nil
}

// packageRootDescriptor reads the index.json from ociDir and returns a descriptor
// for the first (and only) manifest entry, which is the root of a Zarf package layout.
func packageRootDescriptor(ociDir string) (ocispec.Descriptor, error) {
	idxBytes, err := os.ReadFile(filepath.Join(ociDir, "index.json"))
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("reading index.json: %w", err)
	}
	var idx ociIndex
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("parsing index.json: %w", err)
	}
	if len(idx.Manifests) != 1 {
		return ocispec.Descriptor{}, fmt.Errorf("index.json contains %d manifests; expected exactly 1 for a Zarf package layout", len(idx.Manifests))
	}
	m := idx.Manifests[0]
	d, err := parseDigest(m.Digest)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return ocispec.Descriptor{
		MediaType: m.MediaType,
		Digest:    d,
		Size:      m.Size,
	}, nil
}

// maxIndexSize bounds index manifest reads; real indexes are well under 10s of KiB.
// See https://github.com/opencontainers/distribution-spec/blob/4fc4ecbefaaa6e4e1682f59f5ac445d076cf642d/spec.md?plain=1#L540
const maxIndexSize = 4 << 20 // 4 MiB

// pushBundleToRemote copies the child (single-arch bundle) graph to ref's
// repository, then publishes the root index at the tag: this architecture's
// entry is inserted or replaced and other-arch bundle entries already present
// are preserved (ADR-0015). child must carry Platform and ArtifactType.
func pushBundleToRemote(ctx context.Context, store *oraci.Store, child ocispec.Descriptor, ref string, opts *PushOptions) (*PushResult, error) {
	dst, err := resolvePushTarget(ctx, ref, opts)
	if err != nil {
		return nil, fmt.Errorf("resolving push target %s: %w", ref, err)
	}
	parsed, err := name.ParseReference(TrimScheme(ref))
	if err != nil {
		return nil, fmt.Errorf("parsing OCI reference: %w", err)
	}
	// A digest reference cannot address the mutable root index this push maintains.
	if _, isDigest := parsed.(name.Digest); isDigest {
		return nil, fmt.Errorf("bundles must be pushed to a tag reference, not a digest: %s", ref)
	}
	tag := parsed.Identifier()
	copyOpts, err := pushCopyOptions(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("configuring push: %w", err)
	}

	// Copy the child graph without tagging it; the root index published below
	// is the only tagged object.
	if err := oras.CopyGraph(ctx, store, dst, child, copyOpts.CopyGraphOptions); err != nil {
		return nil, fmt.Errorf("pushing bundle content to %s: %w", ref, err)
	}

	rootBytes, rootDesc, err := mergeRootIndex(ctx, dst, tag, child)
	if err != nil {
		return nil, err
	}
	if err := dst.Push(ctx, rootDesc, bytes.NewReader(rootBytes)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return nil, fmt.Errorf("pushing root index to %s: %w", ref, err)
	}
	if err := dst.Tag(ctx, rootDesc, tag); err != nil {
		return nil, fmt.Errorf("tagging root index as %s: %w", tag, err)
	}
	return &PushResult{OCIReference: ref}, nil
}

// mergeRootIndex builds the platform-keyed root index for the tag: the entry
// for child's architecture is replaced with child, other-arch bundle entries
// are preserved, and anything else at the tag is superseded. Entries are
// sorted by architecture for determinism.
func mergeRootIndex(ctx context.Context, dst oras.Target, tag string, child ocispec.Descriptor) ([]byte, ocispec.Descriptor, error) {
	existing, err := existingRootEntries(ctx, dst, tag, child.Platform.Architecture)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("reading existing root index at %s: %w", tag, err)
	}
	entries := append(existing, child)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Platform.Architecture < entries[j].Platform.Architecture
	})

	root := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: entries,
	}
	rootBytes, err := json.Marshal(&root)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("marshaling root index: %w", err)
	}
	return rootBytes, content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, rootBytes), nil
}

// existingRootEntries returns the other-arch bundle entries of the root index
// currently at the tag. A missing tag or a non-root artifact at the tag (e.g.
// a child bundle index) yields nil entries — the push then publishes a fresh
// root containing only the incoming architecture. Any other failure to read
// the existing root is an error: proceeding would silently clobber the other
// architectures' entries.
func existingRootEntries(ctx context.Context, dst oras.Target, tag, arch string) ([]ocispec.Descriptor, error) {
	desc, err := dst.Resolve(ctx, tag)
	if err != nil {
		if errors.Is(err, errdef.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolving %s: %w", tag, err)
	}
	data, err := fetchIndexBytes(ctx, dst, desc)
	if err != nil {
		return nil, err
	}

	var idx ocispec.Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parsing existing content at %s: %w", tag, err)
	}
	// A root index has no artifactType of its own; a child bundle index (or any
	// other typed artifact) at the tag is superseded rather than merged.
	if idx.ArtifactType != "" {
		return nil, nil
	}

	var keep []ocispec.Descriptor
	for _, m := range idx.Manifests {
		if m.MediaType != ocispec.MediaTypeImageIndex || m.ArtifactType != MediaTypeBundle {
			continue
		}
		if m.Platform == nil || m.Platform.Architecture == "" || m.Platform.Architecture == arch {
			continue
		}
		keep = append(keep, m)
	}
	return keep, nil
}

// resolveBundleChild resolves reference to the canonical single-arch bundle
// (child) index and returns its descriptor and raw bytes. A reference
// addressing a child directly (e.g. digest-pinned) is returned as-is; a tag
// pointing at a root index is platform-selected for arch (empty falls back to
// runtime.GOARCH). Anything else is an error.
func resolveBundleChild(ctx context.Context, src oras.Target, reference, arch string) (ocispec.Descriptor, []byte, error) {
	if arch == "" {
		arch = runtime.GOARCH
	}

	desc, err := src.Resolve(ctx, reference)
	if err != nil {
		return ocispec.Descriptor{}, nil, fmt.Errorf("resolving %s: %w", reference, err)
	}
	data, err := fetchIndexBytes(ctx, src, desc)
	if err != nil {
		return ocispec.Descriptor{}, nil, err
	}

	var idx ocispec.Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return ocispec.Descriptor{}, nil, fmt.Errorf("%s does not appear to be a UDS bundle: content is not an OCI index", reference)
	}

	// Direct child: the index self-identifies via artifactType.
	if idx.ArtifactType == MediaTypeBundle {
		return desc, data, nil
	}

	// Root index: select the child entry for the requested architecture.
	var available []string
	for _, m := range idx.Manifests {
		if m.MediaType != ocispec.MediaTypeImageIndex || m.ArtifactType != MediaTypeBundle || m.Platform == nil {
			continue
		}
		if m.Platform.Architecture != arch {
			available = append(available, m.Platform.Architecture)
			continue
		}
		childData, err := fetchIndexBytes(ctx, src, m)
		if err != nil {
			return ocispec.Descriptor{}, nil, err
		}
		var child ocispec.Index
		if err := json.Unmarshal(childData, &child); err != nil || child.ArtifactType != MediaTypeBundle {
			return ocispec.Descriptor{}, nil, fmt.Errorf("root index entry for %s does not reference a UDS bundle", arch)
		}
		return m, childData, nil
	}

	if len(available) > 0 {
		return ocispec.Descriptor{}, nil, fmt.Errorf("no bundle for architecture %q at %s; available: %v", arch, reference, available)
	}
	return ocispec.Descriptor{}, nil, fmt.Errorf("%s does not appear to be a UDS bundle: index does not declare artifactType %s", reference, MediaTypeBundle)
}

// fetchIndexBytes fetches and digest-verifies a manifest's raw bytes from src
// via oras' content.FetchAll, guarding against absurd descriptor sizes first.
func fetchIndexBytes(ctx context.Context, src oras.Target, desc ocispec.Descriptor) ([]byte, error) {
	if desc.Size > maxIndexSize {
		return nil, fmt.Errorf("index %s exceeds maximum allowed size of %d bytes", desc.Digest, maxIndexSize)
	}
	data, err := content.FetchAll(ctx, src, desc)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", desc.Digest, err)
	}
	return data, nil
}

// pushToRemote tags root in store and copies it (and all it references) to ref.
func pushToRemote(ctx context.Context, store *oraci.Store, root ocispec.Descriptor, ref string, opts *PushOptions) (*PushResult, error) {
	if err := store.Tag(ctx, root, "push-root"); err != nil {
		return nil, fmt.Errorf("tagging root: %w", err)
	}
	dst, err := resolvePushTarget(ctx, ref, opts)
	if err != nil {
		return nil, fmt.Errorf("resolving push target %s: %w", ref, err)
	}
	copyOpts, err := pushCopyOptions(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("configuring push: %w", err)
	}
	tag, err := refIdentifier(ref)
	if err != nil {
		return nil, err
	}
	if _, err := oras.Copy(ctx, store, "push-root", dst, tag, copyOpts); err != nil {
		return nil, fmt.Errorf("pushing to %s: %w", ref, err)
	}
	return &PushResult{OCIReference: ref}, nil
}

// pullToStore copies ref into store and returns the copied root descriptor.
func pullToStore(ctx context.Context, ref string, store *oraci.Store, opts *PullOptions) (ocispec.Descriptor, error) {
	src, err := resolvePullTarget(ctx, ref, opts)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("resolving pull source %s: %w", ref, err)
	}
	copyOpts, err := pullCopyOptions(ctx, opts)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("configuring pull: %w", err)
	}
	tag, err := refIdentifier(ref)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	desc, err := oras.Copy(ctx, src, tag, store, tag, copyOpts)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pulling from %s: %w", ref, err)
	}
	return desc, nil
}
