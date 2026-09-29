// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package operator

import (
	"bytes"
	"fmt"
)

func ExampleFormatter_WriteEvents() {
	events := []Event{
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

	var out bytes.Buffer
	formatter := Formatter{NoColor: true}
	_ = formatter.WriteEvents(&out, events)
	fmt.Print(out.String())

	// Output:
	// MUTATED  resource=istio-system/istiod
	//   ADDED path=/metadata/annotations/uds-core.pepr.dev~1uds-core-operator value="succeeded"
	//   ADDED path=/metadata/annotations/uds-core.pepr.dev~1uds-core-policies value="succeeded"
	//
	// ALLOWED  resource=istio-system/istiod repeated=1
	//
	// MUTATED  resource=istio-system
	//   ADDED path=/spec/securityContext/runAsNonRoot value=true
	//   ADDED path=/spec/securityContext/runAsUser value=1000
	//   ADDED path=/spec/securityContext/runAsGroup value=1000
	//   ADDED path=/metadata/annotations/uds-core.pepr.dev~1mutated value="[\"require-non-root-user\",\"drop-all-capabilities\"]"
	//
	// DENIED   resource=default/bad-pod
	//   message="Pod violates UDS policy"
}
