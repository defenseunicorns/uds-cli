// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package printer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/bundlehcl"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

// BufferString returns a human-readable summary of the bundle as a buffer.
// Packages are displayed in deployment order (topological sort) when the
// dependency graph can be built. Falls back to declaration order on error.
func BufferString(ctx context.Context, streams iostreams.IOStreams, b *spec.UDSBundle) *bytes.Buffer {
	var out bytes.Buffer

	fmt.Fprint(&out, "BUNDLE METADATA\n")
	fmt.Fprintf(&out, "  Name:        %s\n", b.Metadata.Name)
	if b.Metadata.Description != "" {
		fmt.Fprintf(&out, "  Description: %s\n", b.Metadata.Description)
	}
	if b.Metadata.Version != "" {
		fmt.Fprintf(&out, "  Version:     %s\n", b.Metadata.Version)
	}

	fmt.Fprintf(&out, "\nPACKAGES (%d)\n", len(b.Packages))

	// Try to display packages in deployment order using the DAG.
	// Fall back to declaration order if the graph can't be built.
	dag, dagErr := bundlehcl.BuildDependencyGraph(ctx, streams, b)
	if dagErr == nil {
		writePackagesInDeployOrder(&out, dag)
	} else {
		writePackagesFlat(&out, b.Packages)
	}

	return &out
}

// writePackagesInDeployOrder writes packages grouped by deployment level.
func writePackagesInDeployOrder(out *bytes.Buffer, dag *bundlehcl.DAG) {
	levels, err := dag.TopologicalLevels()
	if err != nil {
		return
	}

	for levelIdx, level := range levels {
		for _, pkg := range level {
			writePackageEntry(out, pkg, levelIdx)
		}
	}
}

// writePackagesFlat writes packages in the order they appear in the slice (no level info).
func writePackagesFlat(out *bytes.Buffer, pkgs []spec.Package) {
	for i := range pkgs {
		writePackageEntry(out, &pkgs[i], -1)
	}
}

// writePackageEntry writes a single package entry. When level >= 0, a deploy
// level annotation is included.
func writePackageEntry(out *bytes.Buffer, pkg *spec.Package, level int) {
	if level >= 0 {
		fmt.Fprintf(out, "  %s (deploy level: %d)\n", pkg.Name, level)
	} else {
		fmt.Fprintf(out, "  %s\n", pkg.Name)
	}

	fmt.Fprintf(out, "    Source: %s\n", pkg.Source)
	if pkg.Namespace != "" {
		fmt.Fprintf(out, "    Namespace: %s\n", pkg.Namespace)
	}
	if len(pkg.DependsOn) > 0 {
		depNames := make([]string, len(pkg.DependsOn))
		for i, ref := range pkg.DependsOn {
			depNames[i] = ref.Name
		}
		fmt.Fprintf(out, "    DependsOn: %s\n", strings.Join(depNames, ", "))
	}
	if len(pkg.ValuesFiles) > 0 {
		fmt.Fprintf(out, "    Value Files: %s\n", strings.Join(pkg.ValuesFiles, ", "))
	}
	out.WriteString("\n")
}

var _ ResourcePrinter = &TextPrinter{}

// TextPrinter formats result objects as human-readable text using reflection
// and `text` struct tags for field labels.
//
// Supported field types: string, int, bool, float, struct, and slices of these.
// Unsupported types (map, chan, func, interface) are silently skipped.
type TextPrinter struct{}

// PrintObj writes an object in human-readable text form.
func (p *TextPrinter) PrintObj(obj any, w io.Writer) error {
	v := reflect.ValueOf(obj)
	if !v.IsValid() {
		return nil
	}
	return printValue(w, v, 0)
}

// printValue renders a supported reflected value at the requested indentation.
func printValue(w io.Writer, v reflect.Value, indent int) error {
	// Dereference pointer
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	// Skip unsupported types
	if !isSupportedKind(v.Kind()) {
		return nil
	}

	if v.Kind() != reflect.Struct {
		_, err := fmt.Fprintf(w, "%v\n", v.Interface())
		return err
	}

	t := v.Type()
	prefix := strings.Repeat("  ", indent)
	lw := computeLabelWidth(t, v)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := v.Field(i)

		tag := field.Tag.Get("text")
		if tag == "-" || !field.IsExported() {
			continue
		}

		// Skip fields with unsupported types
		if !isSupportedFieldKind(fv) {
			continue
		}

		label, opts := parseTextTag(tag, field.Name)
		omitempty := strings.Contains(opts, "omitempty")

		if omitempty && fv.IsZero() {
			continue
		}

		// Dereference pointer fields so the switch handles the underlying type.
		for fv.Kind() == reflect.Pointer {
			if fv.IsNil() {
				break
			}
			fv = fv.Elem()
		}
		if fv.Kind() == reflect.Pointer {
			// Still a Pointer means it was nil — skip (omitempty already handled above)
			continue
		}

		switch fv.Kind() {
		case reflect.Slice:
			if fv.Len() == 0 && omitempty {
				continue
			}
			elemType := fv.Type().Elem()
			// Dereference pointer element type (e.g. []*T → T)
			if elemType.Kind() == reflect.Pointer {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct {
				// Slice of structs — print header and recurse
				if _, err := fmt.Fprintf(w, "\n%s%s (%d)\n", prefix, label, fv.Len()); err != nil {
					return err
				}
				for j := 0; j < fv.Len(); j++ {
					if err := printValue(w, fv.Index(j), indent+1); err != nil {
						return err
					}
					if _, err := fmt.Fprintln(w); err != nil {
						return err
					}
				}
			} else if isSupportedKind(elemType.Kind()) {
				// Slice of scalars — comma-separated
				items := make([]string, fv.Len())
				for j := 0; j < fv.Len(); j++ {
					elem := fv.Index(j)
					// Dereference pointer elements (e.g. []*string → string)
					for elem.Kind() == reflect.Pointer {
						if elem.IsNil() {
							break
						}
						elem = elem.Elem()
					}
					items[j] = fmt.Sprintf("%v", elem.Interface())
				}
				if _, err := fmt.Fprintf(w, "%s%-*s %s\n", prefix, lw, label+":", strings.Join(items, ", ")); err != nil {
					return err
				}
			}
			// Skip slices of unsupported element types

		case reflect.Struct:
			// Nested struct — recurse
			if _, err := fmt.Fprintf(w, "%s%s:\n", prefix, label); err != nil {
				return err
			}
			if err := printValue(w, fv, indent+1); err != nil {
				return err
			}

		default:
			// Scalar
			if _, err := fmt.Fprintf(w, "%s%-*s %v\n", prefix, lw, label+":", fv.Interface()); err != nil {
				return err
			}
		}
	}
	return nil
}

// isSupportedKind returns true for types the TextPrinter can render.
func isSupportedKind(k reflect.Kind) bool {
	switch k {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.Struct, reflect.Slice:
		return true
	default:
		return false
	}
}

// isSupportedFieldKind checks if a field value's type is renderable.
// Pointers are dereferenced to check the underlying type.
func isSupportedFieldKind(fv reflect.Value) bool {
	k := fv.Kind()
	if k == reflect.Pointer {
		k = fv.Type().Elem().Kind()
	}
	return isSupportedKind(k)
}

// parseTextTag splits "Label,omitempty" into ("Label", "omitempty").
// If tagStr is empty, fieldName is used as the label.
func parseTextTag(tagStr, fieldName string) (label, opts string) {
	if tagStr == "" {
		return fieldName, ""
	}
	parts := strings.SplitN(tagStr, ",", 2)
	label = parts[0]
	if label == "" {
		label = fieldName
	}
	if len(parts) > 1 {
		opts = parts[1]
	}
	return label, opts
}

// computeLabelWidth calculates the column width needed to align all labels
// in a struct based on the longest label derived from text tags.
// Fields tagged with omitempty are excluded from the calculation when their
// value is zero, since they won't be printed.
// Returns a minimum of 12 to keep short-label structs readable.
func computeLabelWidth(t reflect.Type, v reflect.Value) int {
	maxLen := 0
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("text")
		if tag == "-" || !field.IsExported() {
			continue
		}
		label, opts := parseTextTag(tag, field.Name)
		if strings.Contains(opts, "omitempty") && v.Field(i).IsZero() {
			continue
		}
		// +1 for the colon suffix
		if len(label)+1 > maxLen {
			maxLen = len(label) + 1
		}
	}
	if maxLen < 12 {
		return 12
	}
	return maxLen + 1 // +1 for padding after colon
}
