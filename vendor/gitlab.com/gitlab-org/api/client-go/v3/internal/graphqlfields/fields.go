// Package graphqlfields renders GraphQL fragment bodies from a per-service
// field registry and a caller-supplied selection. Fields sharing a non-empty
// Container are grouped under `<container> { ... }`.
//
// Nested template references (e.g. `{{ template "UserCoreBasic" }}`) are
// passed through verbatim; callers parse the returned string as part of the
// outer template tree.
package graphqlfields

import (
	"fmt"
	"strings"
)

// Field maps a logical name to a GraphQL fragment. Container groups related
// fields under a single wrapping block (e.g. work-item widgets under
// `features { ... }`).
type Field struct {
	Name      string
	Fragment  string
	Container string
}

// BuildFragment renders selected fields in registry order. Unknown names
// return an error before rendering.
func BuildFragment(registry []Field, selected []string) (string, error) {
	byName := make(map[string]struct{}, len(registry))
	for _, f := range registry {
		byName[f.Name] = struct{}{}
	}

	requested := make(map[string]struct{}, len(selected))
	for _, name := range selected {
		if _, ok := byName[name]; !ok {
			return "", fmt.Errorf("unknown field %q", name)
		}
		requested[name] = struct{}{}
	}

	if len(registry) == 0 {
		return "", nil
	}

	var topLevel []Field
	containers := make(map[string][]Field)
	var containerOrder []string
	for _, f := range registry {
		if _, ok := requested[f.Name]; !ok {
			continue
		}
		if f.Container == "" {
			topLevel = append(topLevel, f)
			continue
		}
		if _, seen := containers[f.Container]; !seen {
			containerOrder = append(containerOrder, f.Container)
		}
		containers[f.Container] = append(containers[f.Container], f)
	}

	var sb strings.Builder
	for _, f := range topLevel {
		writeIndented(&sb, f.Fragment, "\t")
	}
	for _, name := range containerOrder {
		sb.WriteString("\n\t")
		sb.WriteString(name)
		sb.WriteString(" {")
		for _, f := range containers[name] {
			writeIndented(&sb, f.Fragment, "\t\t")
		}
		sb.WriteString("\n\t}")
	}
	sb.WriteString("\n")
	return sb.String(), nil
}

// writeIndented prepends indent to every line of fragment.
func writeIndented(sb *strings.Builder, fragment, indent string) {
	for line := range strings.SplitSeq(fragment, "\n") {
		sb.WriteString("\n")
		sb.WriteString(indent)
		sb.WriteString(line)
	}
}
