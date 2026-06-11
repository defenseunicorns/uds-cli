// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	oraci "oras.land/oras-go/v2/content/oci"
)

// resolvePushTarget picks the ORAS destination: the ToOrasTarget hook if set,
// otherwise a live registry repository.
func resolvePushTarget(ctx context.Context, ref string, opts *PushOptions) (oras.Target, error) {
	ref = TrimScheme(ref)
	if opts.PushHooks.ToOrasTarget != nil {
		return opts.PushHooks.ToOrasTarget(ctx, ref, opts)
	}
	return newRemoteRepository(ref, *opts.Config.Options)
}

// resolvePullTarget picks the ORAS source: the ToOrasTarget hook if set,
// otherwise a live registry repository.
func resolvePullTarget(ctx context.Context, ref string, opts *PullOptions) (oras.Target, error) {
	ref = TrimScheme(ref)
	if opts.PullHooks.ToOrasTarget != nil {
		return opts.PullHooks.ToOrasTarget(ctx, ref, opts)
	}
	return newRemoteRepository(ref, *opts.Config.Options)
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
