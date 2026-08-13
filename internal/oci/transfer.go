// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"context"
	"fmt"

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
	return NewRemoteRepository(ctx, ref, *opts.Config.Options)
}

// resolvePullTarget picks the ORAS source: the ToOrasTarget hook if set,
// otherwise a live registry repository.
func resolvePullTarget(ctx context.Context, ref string, opts *PullOptions) (oras.Target, error) {
	ref = TrimScheme(ref)
	if opts.PullHooks.ToOrasTarget != nil {
		return opts.PullHooks.ToOrasTarget(ctx, ref, opts)
	}
	return NewRemoteRepository(ctx, ref, *opts.Config.Options)
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
	rootBytes, rootDesc, currentRoot, err := mergeRootIndex(ctx, dst, tag, child)
	if err != nil {
		return nil, err
	}
	if currentRoot.Digest == rootDesc.Digest {
		exists, err := dst.Exists(ctx, child)
		if err != nil {
			return nil, fmt.Errorf("checking bundle content at %s: %w", ref, err)
		}
		if exists {
			return &PushResult{OCIReference: ref}, nil
		}
	}

	copyOpts, err := pushCopyOptions(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("configuring push: %w", err)
	}

	// Copy the child graph without tagging it; the root index published below
	// is the only tagged object.
	if err := copyGraph(ctx, store, dst, child, copyOpts.CopyGraphOptions); err != nil {
		return nil, fmt.Errorf("pushing bundle content to %s: %w", ref, err)
	}
	if err := PushReferenceBytes(ctx, dst, rootDesc, rootBytes, tag); err != nil {
		return nil, fmt.Errorf("pushing root index to %s: %w", ref, err)
	}
	return &PushResult{OCIReference: ref}, nil
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
	tag, err := ReferenceIdentifier(ref)
	if err != nil {
		return nil, err
	}
	if _, err := copyReference(ctx, store, "push-root", dst, tag, copyOpts); err != nil {
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
	tag, err := ReferenceIdentifier(ref)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	desc, err := copyReference(ctx, src, tag, store, tag, copyOpts)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("pulling from %s: %w", ref, err)
	}
	return desc, nil
}
