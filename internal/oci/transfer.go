// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package oci

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/zarf-dev/zarf/src/pkg/ocischeme"
	oras "oras.land/oras-go/v2"
	oraci "oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry"
	orasregistry "oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

// TrimScheme removes the scheme from a reference name
// (e.g., "oci://ghcr.io/org/repo:tag" -> "ghcr.io/org/repo:tag")
func TrimScheme(refName string) string {
	if idx := strings.Index(refName, "://"); idx >= 0 {
		return refName[idx+3:]
	}
	return refName
}

// IsOCIReference checks if a string looks like an OCI registry reference
// (e.g., "oci://ghcr.io/org/repo:tag", "ghcr.io/org/repo:tag", or "registry.example.com/image").
func IsOCIReference(s string) bool {
	// Check for explicit oci:// scheme prefix
	if strings.HasPrefix(s, "oci://") {
		return true
	}

	// Reject other scheme prefixes (http://, https://, etc.)
	if strings.Contains(s, "://") {
		return false
	}

	// If it starts with a path separator or contains backslash, it's a file path
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.Contains(s, "\\") {
		return false
	}

	// Reject strings with spaces
	if strings.Contains(s, " ") {
		return false
	}

	if strings.HasPrefix(s, "localhost/") || strings.HasPrefix(s, "localhost:") {
		return strings.Contains(s, "/")
	}
	if strings.Contains(s, ".") && strings.Contains(s, "/") && (strings.Contains(s, ":") || strings.Contains(s, "@")) {
		return true
	}

	// If it has known file extensions, it's a file path
	if strings.HasSuffix(s, ".hcl") || strings.Contains(s, ".tar") || strings.HasSuffix(s, ".yaml") || strings.HasSuffix(s, ".yml") {
		return false
	}

	// An OCI ref looks like: domain/path or domain/path:tag or ref@sha256:...
	// Must have both a dot (domain) and a slash (path)
	return strings.Contains(s, ".") && strings.Contains(s, "/")
}

// newRemoteRepository creates an ORAS remote repository configured with registry
// transport settings and credentials loaded from the Docker credential store.
func newRemoteRepository(ctx context.Context, ref string, opts ConfigOptions) (*orasregistry.Repository, error) {
	repo, err := orasregistry.NewRepository(ref)
	if err != nil {
		return nil, err
	}

	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t = &http.Transport{}
	}
	transport := t.Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: opts.SkipTLSVerify} //nolint:gosec // user-controlled via --skip-tls-verify
	plainHTTP, err := resolvePlainHTTP(ctx, ref, opts, transport)
	if err != nil {
		return nil, err
	}
	repo.PlainHTTP = plainHTTP

	credStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err != nil {
		return nil, fmt.Errorf("loading docker credentials: %w", err)
	}

	repo.Client = &auth.Client{
		Client:     &http.Client{Transport: retry.NewTransport(transport)},
		Cache:      auth.DefaultCache,
		Credential: credentials.Credential(credStore),
	}

	return repo, nil
}

// resolvePlainHTTP determines whether an OCI reference should use plain HTTP.
// Plain HTTP is only considered when the user explicitly enables it; HTTPS remains
// the default and is preferred whenever the registry supports it.
func resolvePlainHTTP(ctx context.Context, ref string, opts ConfigOptions, transport http.RoundTripper) (bool, error) {
	if !opts.PlainHTTP {
		return false, nil
	}

	parsed, err := registry.ParseReference(ref)
	if err != nil {
		return false, fmt.Errorf("parsing OCI reference %q: %w", ref, err)
	}

	plainHTTP, err := ocischeme.From(ctx).UsePlainHTTP(ctx, parsed.Registry, ocischeme.ProbeOptions{
		InsecureSkipTLSVerify: opts.SkipTLSVerify,
		Transport:             transport,
	})
	if err != nil {
		return false, fmt.Errorf("determining registry transport for %q: %w", ref, err)
	}
	return plainHTTP, nil
}

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
