// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package operator

import "io"

// MonitorOptions configures operator monitor output.
type MonitorOptions struct {
	NoColor bool
}

// Monitor writes operator monitor events.
func Monitor(w io.Writer, opts MonitorOptions) error {
	formatter := Formatter{NoColor: opts.NoColor}
	return formatter.WriteEvents(w, sampleEvents())
}

func sampleEvents() []Event {
	return []Event{
		{
			Kind:     EventMutated,
			Resource: "istio-system/istiod",
			Patch: []PatchOperation{
				{Kind: "ADDED", Path: "/metadata/annotations/uds-core.pepr.dev~1uds-core-operator", Value: `"succeeded"`},
				{Kind: "ADDED", Path: "/metadata/annotations/uds-core.pepr.dev~1uds-core-policies", Value: `"succeeded"`},
			},
		},
		{
			Kind:     EventAllowed,
			Resource: "istio-system/istiod",
			Repeated: 1,
		},
		{
			Kind:     EventMutated,
			Resource: "istio-system",
			Patch: []PatchOperation{
				{Kind: "ADDED", Path: "/spec/securityContext/runAsNonRoot", Value: "true"},
				{Kind: "ADDED", Path: "/spec/securityContext/runAsUser", Value: "1000"},
				{Kind: "ADDED", Path: "/spec/securityContext/runAsGroup", Value: "1000"},
				{Kind: "ADDED", Path: "/metadata/annotations/uds-core.pepr.dev~1mutated", Value: `"[\"require-non-root-user\",\"drop-all-capabilities\"]"`},
			},
		},
		{
			Kind:     EventDenied,
			Resource: "default/bad-pod",
			Message:  "Pod violates UDS policy",
		},
	}
}
