// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	skillPath = ".agents/skills/migrate-legacy-bundle-to-next/SKILL.md"
	assetPath = "internal/cli/bundle/assets/migration-prompt.md"
)

func main() {
	check := flag.Bool("check", false, "check for drift without writing the generated asset")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./hack/migrationprompt [--check]")
		os.Exit(1)
	}
	if err := generate(".", *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(root string, check bool) error {
	skill, err := os.ReadFile(filepath.Join(root, skillPath))
	if err != nil {
		return fmt.Errorf("read migration skill: %w", err)
	}
	prompt, err := assemble(root, string(skill))
	if err != nil {
		return err
	}
	output := filepath.Join(root, assetPath)
	if check {
		current, err := os.ReadFile(output)
		if err != nil {
			return fmt.Errorf("read generated migration prompt (run uds run generate:migration-prompt): %w", err)
		}
		if !bytes.Equal(current, []byte(prompt)) {
			return fmt.Errorf("migration prompt is stale; run uds run generate:migration-prompt and commit %s", assetPath)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("create migration prompt asset directory: %w", err)
	}
	// #nosec G703 -- root is the repository working directory (or a test temp directory); assetPath is constant, not user input.
	if err := os.WriteFile(output, []byte(prompt), 0o600); err != nil {
		return fmt.Errorf("write migration prompt: %w", err)
	}
	return nil
}

var markdownLink = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)\)`)

// assemble includes only documentation explicitly linked by the skill, in first-use
// order. Website cross-links to those documents are redirected to the same sections.
// Other documentation links remain optional references, not additional dependencies.
func assemble(root, skill string) (string, error) {
	body, err := skillBody(skill)
	if err != nil {
		return "", err
	}
	type document struct {
		path string
		body string
		hash string
	}
	var documents []document
	links := make(map[string]string)
	for _, match := range markdownLink.FindAllStringSubmatch(body, -1) {
		path := match[2]
		if !strings.HasPrefix(path, "docs/") {
			continue
		}
		if !filepath.IsLocal(path) || filepath.ToSlash(filepath.Clean(path)) != path || !strings.HasSuffix(path, ".mdx") {
			return "", fmt.Errorf("unsupported migration documentation path %q; use a repository-root docs/*.mdx path", path)
		}
		if _, exists := links[path]; exists {
			continue
		}
		// #nosec G703 -- only clean, repository-local docs paths from the canonical skill are accepted.
		content, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return "", fmt.Errorf("read migration documentation %s: %w", path, err)
		}
		docBody, err := skillBody(string(content))
		if err != nil {
			return "", fmt.Errorf("read migration documentation %s: %w", path, err)
		}
		id := "migration-doc-" + strings.NewReplacer("/", "-", ".", "-").Replace(path)
		links[path] = "#" + id
		route := "/cli/" + strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".mdx") + "/"
		links[route] = "#" + id
		documents = append(documents, document{path: path, body: docBody, hash: fmt.Sprintf("%x", sha256.Sum256(content))})
	}
	var prompt strings.Builder
	// Hash complete sources as well as including their bodies, so frontmatter-only
	// changes also require regeneration. No timestamp or environment data is emitted.
	fmt.Fprintf(&prompt, "<!-- Generated from the canonical skill and its explicit documentation dependencies. Do not edit. Skill SHA256: %x -->\n\n", sha256.Sum256([]byte(skill)))
	prompt.WriteString(rewriteLinks(body, links))
	for _, doc := range documents {
		fmt.Fprintf(&prompt, "\n<a id=%q></a>\n\n## Included documentation: %s\n\n", strings.TrimPrefix(links[doc.path], "#"), doc.path)
		fmt.Fprintf(&prompt, "<!-- Source SHA256: %s -->\n\n", doc.hash)
		prompt.WriteString(rewriteLinks(doc.body, links))
	}
	return prompt.String(), nil
}

func rewriteLinks(body string, links map[string]string) string {
	return markdownLink.ReplaceAllStringFunc(body, func(link string) string {
		match := markdownLink.FindStringSubmatch(link)
		if target, ok := links[match[2]]; ok {
			return "[" + match[1] + "](" + target + ")"
		}
		return link
	})
}

func skillBody(skill string) (string, error) {
	skill = strings.ReplaceAll(skill, "\r\n", "\n")
	frontmatter, ok := strings.CutPrefix(skill, "---\n")
	if !ok {
		return "", errors.New("migration skill must begin with YAML frontmatter")
	}
	_, body, ok := strings.Cut(frontmatter, "\n---\n")
	if !ok {
		return "", errors.New("migration skill frontmatter must have a closing delimiter")
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return "", errors.New("migration skill body must not be empty")
	}
	return body + "\n", nil
}
