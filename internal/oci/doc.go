// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package oci contains UDS-specific OCI layout and registry orchestration.
//
// Generic OCI mechanics belong to ORAS: content-addressed storage, descriptor
// verification, graph traversal, registry copies, retries, and repository
// access. This package keeps the UDS bundle pieces that upstream libraries
// cannot infer, including ADR-0015 child/root indexes, bundle media types,
// source reference dispatch, and archive push/pull workflows. Code outside this
// package should use these helpers for descriptor fetches instead of calling
// ORAS fetch functions directly.
//
// Zarf package semantics belong in internal/zarf and upstream Zarf APIs. Code in
// this package should operate on standard OCI descriptors and ORAS targets, not
// reinterpret Zarf package structure.
package oci
