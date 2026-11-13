// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

const (
	// BundleDefaultsFileName is the name of the optional bundle-level defaults file.
	BundleDefaultsFileName = "defaults.uds.hcl"
	// UDSCacheDirName is the name of the directory containing cached UDS content.
	UDSCacheDirName = ".uds-cache"
	// MaxConcurrency is the upper bound for parallel package deploys within a level.
	MaxConcurrency = 25
)

// Variables contains user-defined configuration values. Nested objects are
// represented as Variables and list-like values as []any.
type Variables map[string]any

// UDSBundleConfig represents the parsed content of config.uds.hcl. Variables
// are decoded from the remaining free-form HCL body after structured blocks.
type UDSBundleConfig struct {
	Options               *ConfigOptions         `hcl:"options,block"`
	SignatureVerification *SignatureVerification `hcl:"signature_verification,block"`
	Variables             Variables              // populated after decode from Remain
	SensitiveVariables    map[string]bool
	Remain                hcl.Body `hcl:",remain"` // captures variables and any other unstructured top-level attributes
}

type sensitiveMark struct{}

// SignatureVerification holds consumer-owned bundle signature trust material.
type SignatureVerification struct {
	PublicKey string               `hcl:"public_key,optional"`
	Keyless   *KeylessVerification `hcl:"keyless,block"`
}

// KeylessVerification holds keyless trust constraints from config.uds.hcl.
type KeylessVerification struct {
	CertificateIdentity         string `hcl:"certificate_identity,optional"`
	CertificateIdentityRegexp   string `hcl:"certificate_identity_regexp,optional"`
	CertificateOIDCIssuer       string `hcl:"certificate_oidc_issuer,optional"`
	CertificateOIDCIssuerRegexp string `hcl:"certificate_oidc_issuer_regexp,optional"`
	TrustedRoot                 string `hcl:"trusted_root,optional"`
}

// ConfigOptions holds bundle-component CLI options from the options block.
// All fields are optional; unset values use the operation defaults.
type ConfigOptions struct {
	LogLevel              string `hcl:"log_level,optional"`
	Architecture          string `hcl:"architecture,optional"`
	PlainHTTP             bool   `hcl:"plain_http,optional"`
	SkipTLSVerify         bool   `hcl:"skip_tls_verify,optional"`
	CacheDir              string `hcl:"uds_cache,optional"`
	TmpDir                string `hcl:"tmp_dir,optional"`
	Concurrency           int    `hcl:"concurrency,optional"`
	SkipArchitectureCheck bool   `hcl:"skip_architecture_check,optional"`
}

// ParseBundleConfig reads and parses a config.uds.hcl file.
// It uses gohcl.DecodeBody to decode the options block via HCL struct tags on
// UDSBundleConfig, and hcl:",remain" to capture the free-form variables attribute
// which is then manually extracted and converted from cty.Value to Variables.
// The context parameter is currently unused as none of the HCL parsing methods supports cancellation.
func (p *HCLParser) ParseBundleConfig(_ context.Context, filePath string) (*UDSBundleConfig, error) {
	if filePath == "" {
		return nil, EmptyParameterError{Name: "filePath"}
	}
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrReadConfigFile, filePath, err)
	}

	hclFile, hclDiagnostics := hclsyntax.ParseConfig(src, filePath, hcl.Pos{Line: 1, Column: 1})
	if hclDiagnostics.HasErrors() {
		return nil, safeHCLParseError(ErrParseConfig, filePath, hclDiagnostics)
	}
	evalContext := configEvalContext(filePath)

	// Decode structured fields (options block) via gohcl; free-form content lands in Remain
	cfg := &UDSBundleConfig{}
	diags := gohcl.DecodeBody(hclFile.Body, evalContext, cfg)
	if diags.HasErrors() {
		if bodyContainsFunction(hclFile.Body, "sensitive") {
			return nil, fmt.Errorf("%w from %q: invalid structured field at %s", ErrDecodeConfig, filePath, hclDiagnosticLocation(diags))
		}
		return nil, fmt.Errorf("%w from %q: %w", ErrDecodeConfig, filePath, diags)
	}

	// Extract the free-form variables attribute from Remain
	if cfg.Remain != nil {
		vars, sensitive, err := extractVariablesFromRemain(cfg.Remain, variableEvalContext(filePath), filePath)
		if err != nil {
			return nil, err
		}
		cfg.Variables = vars
		cfg.SensitiveVariables = sensitive
	}

	return cfg, nil
}

// extractVariablesFromRemain extracts the optional "variables" attribute from the
// remaining HCL body. Variables are free-form (arbitrary nesting of scalars and objects),
// so they can't be decoded via struct tags and must be manually converted from cty.Value.
func extractVariablesFromRemain(body hcl.Body, evalContext *hcl.EvalContext, source string) (Variables, map[string]bool, error) {
	schema := &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: "variables", Required: false},
		},
	}

	content, _, diags := body.PartialContent(schema)
	if diags.HasErrors() {
		if bodyContainsFunction(body, "sensitive") {
			return nil, nil, fmt.Errorf("%w from %q: invalid variable body at %s", ErrReadVariables, source, hclDiagnosticLocation(diags))
		}
		return nil, nil, fmt.Errorf("%w from %q: %w", ErrReadVariables, source, diags)
	}

	attr, ok := content.Attributes["variables"]
	if !ok {
		return nil, nil, nil
	}

	val, diags := attr.Expr.Value(evalContext)
	if diags.HasErrors() {
		if expressionContainsFunction(attr.Expr, "sensitive") {
			return nil, nil, fmt.Errorf("evaluating sensitive variables in %q: %w", source, ErrEvaluateVariables)
		}
		return nil, nil, fmt.Errorf("%w from %q: %w", ErrEvaluateVariables, source, diags)
	}
	sensitive := markedTopLevelVariables(val)
	val, _ = val.UnmarkDeep()

	goVal, err := ctyValueToGo(val)
	if err != nil {
		return nil, nil, fmt.Errorf("%w from %q", ErrConvertVariables, source)
	}

	m, ok := goVal.(Variables)
	if !ok {
		return nil, nil, fmt.Errorf("variables must be an object, got %T: %w", goVal, ErrInvalidVariables)
	}

	return m, sensitive, nil
}

func markedTopLevelVariables(val cty.Value) map[string]bool {
	if !val.IsKnown() || val.IsNull() || !val.Type().IsObjectType() {
		return nil
	}
	sensitive := make(map[string]bool)
	for name := range val.Type().AttributeTypes() {
		if val.GetAttr(name).ContainsMarked() {
			sensitive[name] = true
		}
	}
	if len(sensitive) == 0 {
		return nil
	}
	return sensitive
}

func safeHCLParseError(kind error, path string, diags hcl.Diagnostics) error {
	return fmt.Errorf("%w %q: invalid HCL at %s", kind, path, hclDiagnosticLocation(diags))
}

func hclDiagnosticLocation(diags hcl.Diagnostics) string {
	for _, diag := range diags {
		if diag.Subject != nil {
			return fmt.Sprintf("line %d, column %d", diag.Subject.Start.Line, diag.Subject.Start.Column)
		}
	}
	return "an unknown location"
}

func bodyContainsFunction(body hcl.Body, name string) bool {
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return false
	}
	found := false
	_ = hclsyntax.VisitAll(syntaxBody, func(node hclsyntax.Node) hcl.Diagnostics {
		call, ok := node.(*hclsyntax.FunctionCallExpr)
		if ok && call.Name == name {
			found = true
		}
		return nil
	})
	return found
}

// ctyValueToGo recursively converts a cty.Value to a Go value.
// Supported cty types: String → string, Number → float64, Bool → bool,
// Object → Variables (recursive), Tuple/List/Set → []any (recursive).
// Null and unknown values return an error at any depth. Set iteration order
// is stable per cty value but not user-controlled; prefer lists when order matters.
//
// cty.Map is intentionally not supported. HCL literal `{a=1,b=2}` always
// produces a cty.Object, never a cty.Map; reaching the Map branch would
// require functions like tomap() or typed inputs, neither of which our
// parser enables. A Map
// surfacing here is therefore a contract violation, and we let the loud-fail
// default branch report it as an unsupported variable type.
func ctyValueToGo(val cty.Value) (any, error) {
	val, _ = val.Unmark()
	if val.IsNull() {
		return nil, fmt.Errorf("null values are not supported in config: %w", ErrInvalidVariables)
	}
	if !val.IsKnown() {
		return nil, fmt.Errorf("unknown values are not supported in config: %w", ErrInvalidVariables)
	}

	ty := val.Type()
	switch {
	case ty == cty.String:
		return val.AsString(), nil
	case ty == cty.Number:
		f, _ := val.AsBigFloat().Float64()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, fmt.Errorf("number is outside the supported float64 range: %w", ErrInvalidVariables)
		}
		return f, nil
	case ty == cty.Bool:
		return val.True(), nil
	case ty.IsObjectType():
		// Iterate attributes in sorted order so error messages on multi-attribute
		// failures are deterministic (Go map iteration order is randomized).
		out := make(Variables)
		names := slices.Sorted(maps.Keys(ty.AttributeTypes()))
		for _, k := range names {
			child, err := ctyValueToGo(val.GetAttr(k))
			if err != nil {
				return nil, fmt.Errorf("%q: %w", k, err)
			}
			out[k] = child
		}
		return out, nil
	case ty.IsTupleType(), ty.IsListType(), ty.IsSetType():
		out := make([]any, 0, val.LengthInt())
		it := val.ElementIterator()
		i := 0
		for it.Next() {
			_, elem := it.Element()
			child, err := ctyValueToGo(elem)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out = append(out, child)
			i++
		}
		return out, nil
	default:
		// Loud-fail backstop — silent drops must not occur.
		return nil, fmt.Errorf("unsupported variable type: %s: %w", ty.FriendlyName(), ErrUnsupportedVariableType)
	}
}

// ParseSetVariables parses key=value CLI entries into deploy-time variables.
// Booleans, numbers, lists, objects, and explicitly quoted strings use HCL
// syntax; all other values are treated as unquoted strings.
func ParseSetVariables(entries []string) (Variables, error) {
	variables := make(Variables, len(entries))
	for _, entry := range entries {
		key, rawValue, found := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		rawValue = strings.TrimSpace(rawValue)
		if !found || key == "" || rawValue == "" {
			return nil, errors.New("invalid --set value; expected key=value")
		}

		value, err := parseSetValue(rawValue)
		if err != nil {
			return nil, fmt.Errorf("parsing --set variable %q: %w", key, err)
		}
		variables[key] = value
	}
	return variables, nil
}

func parseSetValue(raw string) (any, error) {
	requiresHCL := raw == "true" || raw == "false" || raw == "null" ||
		strings.HasPrefix(raw, `"`) || strings.HasPrefix(raw, "[") || strings.HasPrefix(raw, "{")

	expr, diags := hclsyntax.ParseExpression([]byte(raw), "--set", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		if !requiresHCL {
			return raw, nil
		}
		return nil, fmt.Errorf("parsing HCL --set value: %w", ErrInvalidVariables)
	}
	value, diags := expr.Value(&hcl.EvalContext{})
	if diags.HasErrors() {
		if !requiresHCL {
			return raw, nil
		}
		return nil, fmt.Errorf("evaluating HCL --set value: %w", ErrInvalidVariables)
	}
	if !requiresHCL && value.Type() != cty.Number {
		return raw, nil
	}

	converted, err := ctyValueToGo(value)
	if err != nil {
		return nil, fmt.Errorf("converting --set value: %w", ErrInvalidVariables)
	}
	return converted, nil
}

// ParseDefaults reads a defaults file from disk and validates it.
// A valid defaults file contains at most one top-level attribute named "variables"
// and no blocks. Returns the parsed Variables, or nil if the file has no variables.
// The context parameter is currently unused as none of the HCL parsing methods supports cancellation.
func ParseDefaults(ctx context.Context, path string) (Variables, error) {
	variables, _, err := ParseDefaultsWithSensitivity(ctx, path)
	return variables, err
}

// ParseDefaultsWithSensitivity reads defaults variables and their top-level sensitivity metadata.
func ParseDefaultsWithSensitivity(_ context.Context, path string) (Variables, map[string]bool, error) {
	if path == "" {
		return nil, nil, EmptyParameterError{Name: "path"}
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%w %q: %w", ErrReadDefaultsFile, path, err)
	}
	return parseDefaultsContentWithSensitivity(src, path, true)
}

// ParseDefaultsBytes parses defaults HCL without enabling file-backed expressions.
func ParseDefaultsBytes(ctx context.Context, src []byte) (Variables, error) {
	variables, _, err := ParseDefaultsBytesWithSensitivity(ctx, src)
	return variables, err
}

// ParseDefaultsBytesWithSensitivity parses defaults HCL and returns top-level sensitivity metadata.
func ParseDefaultsBytesWithSensitivity(_ context.Context, src []byte) (Variables, map[string]bool, error) {
	if len(src) == 0 {
		return nil, nil, EmptyParameterError{Name: "src"}
	}
	return parseDefaultsContentWithSensitivity(src, BundleDefaultsFileName, false)
}

// parseDefaultsContent decodes variables from defaults HCL content.
func parseDefaultsContent(src []byte, path string) (Variables, error) {
	variables, _, err := parseDefaultsContentWithSensitivity(src, path, true)
	return variables, err
}

func parseDefaultsContentWithSensitivity(src []byte, path string, allowFile bool) (Variables, map[string]bool, error) {
	hclFile, diags := hclsyntax.ParseConfig(src, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, nil, safeHCLParseError(ErrParseDefaults, path, diags)
	}
	if !allowFile {
		if err := rejectFileFunctionWithoutSourcePath(hclFile, "defaults.uds.hcl"); err != nil {
			return nil, nil, err
		}
	}

	// JustAttributes rejects any blocks (e.g. options {}).
	attrs, diags := hclFile.Body.JustAttributes()
	if diags.HasErrors() {
		if bodyContainsFunction(hclFile.Body, "sensitive") {
			return nil, nil, fmt.Errorf("%w %q at %s", ErrInvalidDefaults, path, hclDiagnosticLocation(diags))
		}
		return nil, nil, fmt.Errorf("%w %q: %w", ErrInvalidDefaults, path, diags)
	}

	// Only "variables" is allowed at the top level.
	for name := range attrs {
		if name != "variables" {
			return nil, nil, fmt.Errorf("defaults file contains unexpected attribute %q; only \"variables\" is allowed: %w", name, ErrInvalidDefaults)
		}
	}

	attr, ok := attrs["variables"]
	if !ok {
		return nil, nil, nil
	}

	evalContext := &hcl.EvalContext{Functions: map[string]function.Function{"sensitive": sensitiveFunction()}}
	if allowFile {
		evalContext = variableEvalContext(path)
	}
	val, diags := attr.Expr.Value(evalContext)
	if diags.HasErrors() {
		if expressionContainsFunction(attr.Expr, "sensitive") {
			return nil, nil, fmt.Errorf("evaluating sensitive variables in %q: %w", path, ErrEvaluateVariables)
		}
		return nil, nil, fmt.Errorf("%w in %q: %w", ErrEvaluateVariables, path, diags)
	}
	sensitive := markedTopLevelVariables(val)
	hasSensitive := val.ContainsMarked()
	val, _ = val.UnmarkDeep()

	goVal, err := ctyValueToGo(val)
	if err != nil {
		if hasSensitive {
			return nil, nil, fmt.Errorf("converting sensitive variables from %q: %w", path, ErrConvertVariables)
		}
		return nil, nil, fmt.Errorf("%w from %q: %w", ErrConvertVariables, path, err)
	}

	m, ok := goVal.(Variables)
	if !ok {
		return nil, nil, fmt.Errorf("variables must be an object, got %T: %w", goVal, ErrInvalidVariables)
	}

	return m, sensitive, nil
}

// MergeVariables deep-merges variables from overrides into base, returning a new Variables map.
// The base is deep-copied so callers can mutate the result without affecting their inputs.
// Nested Variables are deep-merged; everything else (scalars, lists) is replaced wholesale,
// matching Helm overlay conventions.
func MergeVariables(base, overrides Variables) Variables {
	if base == nil && overrides == nil {
		return nil
	}
	result := deepCopyVariables(base)
	if result == nil {
		result = make(Variables)
	}
	deepMerge(result, overrides)
	return result
}

// MergeSensitiveVariables unions top-level sensitivity marks; true marks remain sticky through overrides.
func MergeSensitiveVariables(base, overrides map[string]bool) map[string]bool {
	if len(base)+len(overrides) == 0 {
		return nil
	}
	merged := make(map[string]bool, len(base)+len(overrides))
	for name, sensitive := range base {
		merged[name] = sensitive
	}
	for name, sensitive := range overrides {
		merged[name] = merged[name] || sensitive
	}
	return merged
}

// deepCopyVariables returns a deep copy of v. nil maps copy to nil.
func deepCopyVariables(v Variables) Variables {
	if v == nil {
		return nil
	}
	out := make(Variables, len(v))
	for k, val := range v {
		out[k] = deepCopyAny(val)
	}
	return out
}

// deepCopySlice returns a deep copy of s. nil slices copy to nil.
func deepCopySlice(s []any) []any {
	if s == nil {
		return nil
	}
	out := make([]any, len(s))
	for i, val := range s {
		out[i] = deepCopyAny(val)
	}
	return out
}

// deepCopyAny dispatches on the dynamic type of an element drawn from a Variables
// or []any. Scalars are value-typed and shared safely.
//
// The HCL parser only produces Variables for nested objects (never bare
// map[string]any). A bare map[string]any here means a non-HCL caller violated
// that contract: aliasing it would silently propagate the bad shape into
// merge/flatten/templating, so we panic at the copy site to surface the bug
// at its origin rather than letting it tunnel further downstream.
func deepCopyAny(v any) any {
	switch t := v.(type) {
	case Variables:
		return deepCopyVariables(t)
	case []any:
		return deepCopySlice(t)
	case map[string]any:
		panic(fmt.Sprintf("bare map[string]any in Variables (key contract violation); use Variables — got %v", t))
	default:
		return t
	}
}

// deepMerge recursively merges src into dst. Nested Variables are deep-merged;
// any other type (incl. []any) is replaced wholesale (Helm overlay convention).
// dst is mutated in place; src is not modified, and src-side nested maps/slices
// are deep-copied so the result never aliases src.
func deepMerge(dst, src Variables) {
	for k, sv := range src {
		if dv, exists := dst[k]; exists {
			if dm, dOK := dv.(Variables); dOK {
				if sm, sOK := sv.(Variables); sOK {
					deepMerge(dm, sm)
					continue
				}
			}
		}
		dst[k] = deepCopyAny(sv)
	}
}

// Flatten returns the top-level values as an uppercased string map suitable for
// Zarf's SetVariables passthrough. Only scalar types (string, float64, bool) are
// included; non-scalar values (lists, nested Variables, other types) are silently
// skipped and must be passed to Zarf via values_files instead.
//
// Complex types are excluded because values_files is the proper channel for them.
// Templates already handle nested Variables natively, and skipping non-scalars
// steers authors toward the values_files path rather than introducing a footgun
// where chart authors would need to JSON-decode Zarf var tokens.
//
// Example:
//
//	Input:  Variables{"domain": "uds.dev", "ports": []any{1.0, 2.0}, "nested": Variables{"key": "val"}}
//	Output: map[string]string{"DOMAIN": "uds.dev"}  (ports and nested skipped)
func (v Variables) Flatten() map[string]string {
	out := make(map[string]string, len(v))
	for k, val := range v {
		upper := strings.ToUpper(k)
		switch s := val.(type) {
		case string:
			out[upper] = s
		case float64:
			out[upper] = strconv.FormatFloat(s, 'f', -1, 64)
		case bool:
			out[upper] = strconv.FormatBool(s)
		default:
			// Skip non-scalar types ([]any, Variables, etc.) silently.
			// They belong in values_files where templates can process them natively.
		}
	}
	return out
}
