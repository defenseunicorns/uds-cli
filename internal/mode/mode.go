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
)

const FeatureGatesEnv = "UDS_FEATURE_GATES"

type Mode string

const (
	Legacy Mode = "legacy"
	Next   Mode = "next"
)

type GateSet map[string]bool

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

func (g GateSet) String() string {
	names := make([]string, 0, len(g))
	for name := range g {
		names = append(names, name)
	}
	sort.Strings(names)
	pairs := make([]string, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, fmt.Sprintf("%s=%t", name, g[name]))
	}
	return strings.Join(pairs, ",")
}

// Resolve merges Alpha defaults, the environment, then command line gates.
// It returns the arguments that Cobra should receive.
func Resolve(args []string, lookupEnv func(string) (string, bool)) (Mode, GateSet, []string, error) {
	gates := GateSet{"NextMode": false}
	if value, ok := lookupEnv(FeatureGatesEnv); ok {
		parsed, err := parse(value)
		if err != nil {
			return "", nil, nil, fmt.Errorf("invalid %s: %w", FeatureGatesEnv, err)
		}
		for name, enabled := range parsed {
			gates[name] = enabled
		}
	}
	remaining := make([]string, 0, len(args))
	seenFeatureGates := false
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
		case strings.HasPrefix(arg, "--feature-gates="):
			value = strings.TrimPrefix(arg, "--feature-gates=")
		case arg == "--feature-gates":
			if len(args) == 0 {
				return "", nil, nil, errors.New("--feature-gates requires a value")
			}
			value = args[0]
			args = args[1:]
		default:
			remaining = append(remaining, arg)
			continue
		}
		if seenFeatureGates {
			return "", nil, nil, errors.New("duplicate --feature-gates option")
		}
		seenFeatureGates = true
		parsed, err := parse(value)
		if err != nil {
			return "", nil, nil, fmt.Errorf("invalid --feature-gates: %w", err)
		}
		for name, enabled := range parsed {
			gates[name] = enabled
		}
	}
	if gates["NextMode"] {
		return Next, gates, remaining, nil
	}
	return Legacy, gates, remaining, nil
}

func parse(value string) (GateSet, error) {
	if value == "" {
		return nil, errors.New("a value is required")
	}
	gates := GateSet{}
	for _, pair := range strings.Split(value, ",") {
		name, raw, ok := strings.Cut(pair, "=")
		if !ok || name == "" || raw == "" {
			return nil, fmt.Errorf("expected name=true or name=false, got %q", pair)
		}
		if name != "NextMode" {
			return nil, fmt.Errorf("unknown gate %q", name)
		}
		enabled, err := strconv.ParseBool(raw)
		if err != nil || (raw != "true" && raw != "false") {
			return nil, fmt.Errorf("%s must be true or false", name)
		}
		if _, exists := gates[name]; exists {
			return nil, fmt.Errorf("duplicate gate %q", name)
		}
		gates[name] = enabled
	}
	return gates, nil
}

func stripBootstrapArgs(args []string) []string {
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return append(remaining, args[i:]...)
		}
		switch {
		case strings.HasPrefix(arg, "--feature-gates="):
			continue
		case arg == "--feature-gates":
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
