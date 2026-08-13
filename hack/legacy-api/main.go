// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Command legacy-api records and checks the exported Legacy library surface.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const modulePath = "github.com/defenseunicorns/uds-cli/"

func main() {
	checkPath := flag.String("check", "", "compare the API with this file")
	writePath := flag.String("write", "", "write the API to this file")
	sourceLayout := flag.Bool("source-layout", false, "load the pre-move src layout")
	flag.Parse()

	api, err := fingerprint(*sourceLayout)
	if err != nil {
		fatal(err)
	}
	switch {
	case *checkPath != "" && *writePath == "":
		want, err := os.ReadFile(*checkPath)
		if err != nil {
			fatal(err)
		}
		if missing := missingLines(want, api); len(missing) > 0 {
			fatal(fmt.Errorf("legacy API changed, missing %q", missing[0]))
		}
	case *writePath != "" && *checkPath == "":
		if err := os.WriteFile(*writePath, api, 0o600); err != nil {
			fatal(err)
		}
	default:
		fatal(errors.New("set exactly one of -check or -write"))
	}
}

func fingerprint(sourceLayout bool) ([]byte, error) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps}
	patterns := []string{"./pkg/legacy/..."}
	if sourceLayout {
		patterns = []string{"./src/config/...", "./src/types/...", "./src/pkg/..."}
	}
	loaded, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	if packages.PrintErrors(loaded) > 0 {
		return nil, errors.New("unable to load Legacy packages")
	}
	sort.Slice(loaded, func(i, j int) bool { return loaded[i].PkgPath < loaded[j].PkgPath })

	var output strings.Builder
	for _, pkg := range loaded {
		fmt.Fprintf(&output, "package %s\n", canonicalPath(pkg.PkgPath))
		entries := packageEntries(pkg.Types)
		for _, entry := range entries {
			fmt.Fprintf(&output, "%s\n", entry)
		}
		output.WriteByte('\n')
	}
	return []byte(output.String()), nil
}

func packageEntries(pkg *types.Package) []string {
	qualifier := func(other *types.Package) string {
		return canonicalPath(other.Path())
	}
	entries := []string{}
	for _, name := range pkg.Scope().Names() {
		object := pkg.Scope().Lookup(name)
		if !object.Exported() {
			continue
		}
		entries = append(entries, types.ObjectString(object, qualifier))
		typeName, ok := object.(*types.TypeName)
		if !ok {
			continue
		}
		entries = append(entries, fmt.Sprintf("underlying %s %s", name, types.TypeString(typeName.Type().Underlying(), qualifier)))
		entries = append(entries, methodEntries(typeName.Type(), qualifier)...)
	}
	sort.Strings(entries)
	return entries
}

func canonicalPath(path string) string {
	path = strings.TrimPrefix(path, modulePath)
	switch {
	case path == "src/config" || strings.HasPrefix(path, "src/config/"):
		return strings.Replace(path, "src/config", "pkg/legacy/config", 1)
	case path == "src/types" || strings.HasPrefix(path, "src/types/"):
		return strings.Replace(path, "src/types", "pkg/legacy/types", 1)
	case path == "src/pkg" || strings.HasPrefix(path, "src/pkg/"):
		return strings.Replace(path, "src/pkg", "pkg/legacy", 1)
	default:
		return path
	}
}

func missingLines(want, got []byte) []string {
	available := map[string]struct{}{}
	for _, line := range strings.Split(string(got), "\n") {
		available[line] = struct{}{}
	}
	missing := []string{}
	for _, line := range strings.Split(string(want), "\n") {
		if _, found := available[line]; !found {
			missing = append(missing, line)
		}
	}
	return missing
}

func methodEntries(value types.Type, qualifier types.Qualifier) []string {
	entries := map[string]struct{}{}
	for _, receiver := range []types.Type{value, types.NewPointer(value)} {
		set := types.NewMethodSet(receiver)
		for i := 0; i < set.Len(); i++ {
			method := set.At(i).Obj()
			if method.Exported() {
				entries[types.ObjectString(method, qualifier)] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(entries))
	for entry := range entries {
		result = append(result, entry)
	}
	return result
}

func fatal(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
