package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
	prompt, err := skillBody(string(skill))
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
