// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package mode resolves the product mode before a Cobra tree is constructed.
package mode

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	zarfconfig "github.com/zarf-dev/zarf/src/config"
)

const FeaturesEnv = "CLI_FEATURES"

type Mode string

const (
	Legacy Mode = "legacy"
	Next   Mode = "next"
)

type FeatureSet map[string]bool

// processArgs preserves bootstrap options while dependent command packages see
// the cleaned arguments they require during package initialization.
var processArgs = append([]string(nil), os.Args[1:]...)

func init() {
	os.Args = append(append([]string(nil), os.Args[:1]...), prepareBootstrapArgs(processArgs)...)
}

// ProcessArgs returns the original process arguments captured before dependent
// command packages inspect them during package initialization.
func ProcessArgs() []string {
	return append([]string(nil), processArgs...)
}

func (f FeatureSet) String() string {
	names := make([]string, 0, len(f))
	for name := range f {
		names = append(names, name)
	}
	sort.Strings(names)
	pairs := make([]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, fmt.Sprintf("%s=%t", name, f[name]))
	}
	return strings.Join(pairs, ",")
}

// Resolve merges Alpha defaults, the environment, then command line features.
// It returns the arguments that Cobra should receive.
func Resolve(args []string, lookupEnv func(string) (string, bool)) (Mode, FeatureSet, []string, error) {
	features := FeatureSet{"NextMode": false}
	if value, ok := lookupEnv(FeaturesEnv); ok {
		parsed, err := parse(value)
		if err != nil {
			return "", nil, nil, fmt.Errorf("invalid %s: %w", FeaturesEnv, err)
		}
		for name, enabled := range parsed {
			features[name] = enabled
		}
	}
	remaining := make([]string, 0, len(args))
	zarfIndex := zarfCommandIndex(args)
	seenFeatures := false
	for i := 0; i < len(args); i++ {
		if zarfIndex >= 0 && i >= zarfIndex {
			remaining = append(remaining, args[i:]...)
			break
		}
		arg := args[i]
		if arg == "--" {
			remaining = append(remaining, arg)
			remaining = append(remaining, args[i+1:]...)
			break
		}
		value := ""
		switch {
		case strings.HasPrefix(arg, "--features="):
			value = strings.TrimPrefix(arg, "--features=")
		case arg == "--features":
			if i+1 >= len(args) {
				return "", nil, nil, errors.New("--features requires a value")
			}
			i++
			value = args[i]
		default:
			remaining = append(remaining, arg)
			continue
		}
		if seenFeatures {
			return "", nil, nil, errors.New("duplicate --features option")
		}
		seenFeatures = true
		parsed, err := parse(value)
		if err != nil {
			return "", nil, nil, fmt.Errorf("invalid --features: %w", err)
		}
		for name, enabled := range parsed {
			features[name] = enabled
		}
	}
	if features["NextMode"] {
		return Next, features, remaining, nil
	}
	return Legacy, features, remaining, nil
}

func parse(value string) (FeatureSet, error) {
	if value == "" {
		return nil, errors.New("a value is required")
	}
	features := FeatureSet{}
	for _, pair := range strings.Split(value, ",") {
		name, raw, explicit := strings.Cut(pair, "=")
		if name == "" || (explicit && raw == "") {
			return nil, fmt.Errorf("expected name, name=true, or name=false, got %q", pair)
		}
		if !knownFeature(name) {
			return nil, fmt.Errorf("unknown feature %q", name)
		}
		enabled := true
		if explicit {
			parsed, err := strconv.ParseBool(raw)
			if err != nil || (raw != "true" && raw != "false") {
				return nil, fmt.Errorf("%s must be true or false", name)
			}
			enabled = parsed
		}
		if _, exists := features[name]; exists {
			return nil, fmt.Errorf("duplicate feature %q", name)
		}
		features[name] = enabled
	}
	return features, nil
}

func knownFeature(name string) bool {
	return name == "NextMode"
}

func zarfCommandIndex(args []string) int {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return -1
		}
		if arg == "zarf" || arg == "z" {
			return i
		}
		if arg == "tools" {
			return nestedZarfCommandIndex(args, i+1)
		}
		if !strings.HasPrefix(arg, "-") {
			return -1
		}
		if !strings.Contains(arg, "=") && zarfRootFlagTakesValue(strings.TrimLeft(arg, "-")) {
			i++
		}
	}
	return -1
}

func nestedZarfCommandIndex(args []string, start int) int {
	for i := start; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return -1
		}
		if arg == "zarf" || arg == "z" {
			return i
		}
		if !strings.HasPrefix(arg, "-") {
			return -1
		}
		if !strings.Contains(arg, "=") && zarfRootFlagTakesValue(strings.TrimLeft(arg, "-")) {
			i++
		}
	}
	return -1
}

func zarfRootFlagTakesValue(name string) bool {
	switch name {
	case "features", "log-level", "l", "architecture", "a", "uds-cache", "tmpdir", "oci-concurrency":
		return true
	default:
		return false
	}
}

func prepareBootstrapArgs(args []string) []string {
	return normalizeRootZarfToolsArgs(stripBootstrapArgs(args))
}

func stripBootstrapArgs(args []string) []string {
	zarfIndex := zarfCommandIndex(args)
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if zarfIndex >= 0 && i >= zarfIndex {
			return append(remaining, args[i:]...)
		}
		arg := args[i]
		if arg == "--" {
			return append(remaining, args[i:]...)
		}
		switch {
		case strings.HasPrefix(arg, "--features="):
			continue
		case arg == "--features":
			if i+1 < len(args) {
				i++
			}
			continue
		default:
			remaining = append(remaining, arg)
		}
	}
	return remaining
}

func normalizeRootZarfToolsArgs(args []string) []string {
	index := rootZarfToolsCommandIndex(args)
	if index < 0 {
		return args
	}
	if zarfconfig.ActionsCommandZarfPrefix != "" {
		return append([]string{"zarf"}, args[index+1:]...)
	}
	return append([]string(nil), args[index+1:]...)
}

func rootZarfToolsCommandIndex(args []string) int {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return -1
		}
		if arg == "zarf" || arg == "z" {
			zarfIndex := i
			for i = zarfIndex + 1; i < len(args); i++ {
				arg = args[i]
				if arg == cobra.ShellCompRequestCmd || arg == cobra.ShellCompNoDescRequestCmd {
					continue
				}
				if arg == "tools" || arg == "t" {
					return zarfIndex
				}
				if arg == "--" || !strings.HasPrefix(arg, "-") {
					return -1
				}
				if !strings.Contains(arg, "=") && zarfRootFlagTakesValue(strings.TrimLeft(arg, "-")) {
					i++
				}
			}
			return -1
		}
		if !strings.HasPrefix(arg, "-") {
			return -1
		}
		if !strings.Contains(arg, "=") && zarfRootFlagTakesValue(strings.TrimLeft(arg, "-")) {
			i++
		}
	}
	return -1
}
