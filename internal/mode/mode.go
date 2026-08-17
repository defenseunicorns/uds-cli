// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package mode resolves the product mode before a Cobra tree is constructed.
package mode

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/zarf-dev/zarf/src/pkg/feature"
)

const FeaturesEnv = "UDS_FEATURES"

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
	os.Args = append(append([]string(nil), os.Args[:1]...), stripBootstrapArgs(processArgs)...)
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
	seenFeatures := false
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		if arg == "--" {
			remaining = append(remaining, arg)
			remaining = append(remaining, args...)
			break
		}
		value := ""
		switch {
		case strings.HasPrefix(arg, "--features="):
			value = strings.TrimPrefix(arg, "--features=")
		case arg == "--features":
			if len(args) == 0 {
				return "", nil, nil, errors.New("--features requires a value")
			}
			value = args[0]
			args = args[1:]
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
	remaining = addZarfFeatures(remaining, features)
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
		name, raw, ok := strings.Cut(pair, "=")
		if !ok || name == "" || raw == "" {
			return nil, fmt.Errorf("expected name=true or name=false, got %q", pair)
		}
		if !knownFeature(name) {
			return nil, fmt.Errorf("unknown feature %q", name)
		}
		enabled, err := strconv.ParseBool(raw)
		if err != nil || (raw != "true" && raw != "false") {
			return nil, fmt.Errorf("%s must be true or false", name)
		}
		if _, exists := features[name]; exists {
			return nil, fmt.Errorf("duplicate feature %q", name)
		}
		features[name] = enabled
	}
	return features, nil
}

func knownFeature(name string) bool {
	if name == "NextMode" {
		return true
	}
	for _, f := range feature.AllDefault() {
		if string(f.Name) == name {
			return true
		}
	}
	return false
}

func addZarfFeatures(args []string, features FeatureSet) []string {
	zarfFeatures := FeatureSet{}
	for name, enabled := range features {
		if name != "NextMode" {
			zarfFeatures[name] = enabled
		}
	}
	if len(zarfFeatures) == 0 {
		return args
	}
	for i, arg := range args {
		if arg == "zarf" {
			return slices.Insert(args, i+1, "--features="+zarfFeatures.String())
		}
	}
	return args
}

func stripBootstrapArgs(args []string) []string {
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
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
