// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package modify

import (
	"bytes"
	"fmt"
	"os"

	"github.com/defenseunicorns/pkg/helpers/v2"
	goyaml "github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/zarf-dev/zarf/src/api"
	"github.com/zarf-dev/zarf/src/api/convert"
	"github.com/zarf-dev/zarf/src/api/v1beta1"
)

func writeSourceDefinition(path string, pkg api.Package) error {
	// Write only the source API version. Zarf's multi-version writer projects beta
	// service components into an alpha init package that cannot pass source validation.
	var definition any = convert.PackageToV1alpha1(pkg)
	if pkg.GetAPIVersion() == v1beta1.APIVersion {
		definition = convert.PackageToV1beta1(pkg)
	}
	contents, err := goyaml.MarshalWithOptions(
		definition,
		goyaml.IndentSequence(true),
		goyaml.UseLiteralStyleIfMultiline(true),
	)
	if err != nil {
		return fmt.Errorf("marshaling source package definition: %w", err)
	}
	contents, err = spaceSourceDefinition(contents)
	if err != nil {
		return fmt.Errorf("spacing source package definition: %w", err)
	}
	return os.WriteFile(path, contents, helpers.ReadWriteUser)
}

func spaceSourceDefinition(contents []byte) ([]byte, error) {
	file, err := parser.ParseBytes(contents, 0)
	if err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	if len(file.Docs) != 1 {
		return nil, fmt.Errorf("expected one YAML document, got %d", len(file.Docs))
	}
	root, ok := file.Docs[0].Body.(*ast.MappingNode)
	if !ok {
		return nil, fmt.Errorf("expected a top-level YAML mapping")
	}

	// Get the separators to add spacing after each top level key (except for kind and metadata)
	// and between each component in the components list for enhanced source readability
	separators := make(map[int]bool)
	for i, field := range root.Values {
		key := field.Key.GetToken().Value
		if i > 0 && (key != "metadata" || root.Values[i-1].Key.GetToken().Value != "kind") {
			separators[field.Key.GetToken().Position.Line] = true
		}
		if key == "components" {
			if components, ok := field.Value.(*ast.SequenceNode); ok {
				for j, entry := range components.Entries {
					if j > 0 {
						separators[entry.GetToken().Position.Line] = true
					}
				}
			}
		}
	}

	// Keep the marshaled bytes intact: scalar text can resemble structural YAML,
	// and adding a blank after a keep-chomp scalar changes its trailing newlines.
	lines := bytes.SplitAfter(contents, []byte("\n"))
	var spaced bytes.Buffer
	for i, line := range lines {
		if separators[i+1] && i > 0 && len(bytes.TrimSpace(lines[i-1])) != 0 {
			spaced.WriteByte('\n')
		}
		spaced.Write(line)
	}
	return spaced.Bytes(), nil
}
