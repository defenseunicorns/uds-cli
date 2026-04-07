// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package printer

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// testResult is a sample result struct for testing printers.
type testResult struct {
	Name    string       `json:"name"    yaml:"name"    text:"Name"`
	Version string       `json:"version" yaml:"version" text:"Version,omitempty"`
	Hidden  string       `json:"hidden"  yaml:"hidden"  text:"-"`
	Items   []testItem   `json:"items"   yaml:"items"   text:"ITEMS"`
	Tags    []string     `json:"tags"    yaml:"tags"    text:"Tags,omitempty"`
	Count   int          `json:"count"   yaml:"count"   text:"Count"`
	Empty   string       `json:"empty"   yaml:"empty"   text:"Empty,omitempty"`
	Nested  testNested   `json:"nested"  yaml:"nested"  text:"Nested"`
}

type testItem struct {
	Label  string `json:"label"  yaml:"label"  text:"Label"`
	Source string `json:"source" yaml:"source" text:"Source"`
}

type testNested struct {
	Key string `json:"key" yaml:"key" text:"Key"`
}

func sampleResult() testResult {
	return testResult{
		Name:    "my-bundle",
		Version: "1.0.0",
		Hidden:  "should-not-appear",
		Items: []testItem{
			{Label: "db", Source: "oci://registry/db:1"},
			{Label: "api", Source: "oci://registry/api:2"},
		},
		Tags:   []string{"prod", "stable"},
		Count:  42,
		Nested: testNested{Key: "value"},
	}
}

func TestTextPrinter_PrintObj(t *testing.T) {
	var buf bytes.Buffer
	p := &TextPrinter{}

	err := p.PrintObj(sampleResult(), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Name:")
	assert.Contains(t, out, "my-bundle")
	assert.Contains(t, out, "Version:")
	assert.Contains(t, out, "1.0.0")
	assert.NotContains(t, out, "should-not-appear", "text:\"-\" fields should be hidden")
	assert.Contains(t, out, "ITEMS (2)")
	assert.Contains(t, out, "Label:")
	assert.Contains(t, out, "db")
	assert.Contains(t, out, "api")
	assert.Contains(t, out, "Tags:")
	assert.Contains(t, out, "prod, stable")
	assert.Contains(t, out, "Count:")
	assert.Contains(t, out, "42")
	assert.NotContains(t, out, "Empty:", "omitempty zero-value fields should be hidden")
	assert.Contains(t, out, "Nested:")
	assert.Contains(t, out, "Key:")
	assert.Contains(t, out, "value")
}

func TestTextPrinter_Omitempty(t *testing.T) {
	var buf bytes.Buffer
	p := &TextPrinter{}

	obj := testResult{
		Name:  "minimal",
		Count: 0,
	}
	err := p.PrintObj(obj, &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Name:")
	assert.NotContains(t, out, "Version:", "omitempty zero-value string should be hidden")
	assert.NotContains(t, out, "Tags:", "omitempty empty slice should be hidden")
	assert.NotContains(t, out, "Empty:")
}

func TestTextPrinter_Pointer(t *testing.T) {
	var buf bytes.Buffer
	p := &TextPrinter{}

	obj := &testResult{Name: "ptr-test", Count: 1}
	err := p.PrintObj(obj, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "ptr-test")
}

func TestTextPrinter_NilPointer(t *testing.T) {
	var buf bytes.Buffer
	p := &TextPrinter{}

	var obj *testResult
	err := p.PrintObj(obj, &buf)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestTextPrinter_PointerScalarSlice(t *testing.T) {
	type withPtrSlice struct {
		Names []*string `text:"Names"`
	}

	a, b := "alpha", "beta"
	obj := withPtrSlice{Names: []*string{&a, &b}}

	var buf bytes.Buffer
	p := &TextPrinter{}
	require.NoError(t, p.PrintObj(obj, &buf))
	out := buf.String()
	assert.Contains(t, out, "alpha")
	assert.Contains(t, out, "beta")
	assert.NotContains(t, out, "0x", "should not contain pointer addresses")
}

func TestJSONPrinter_PrintObj(t *testing.T) {
	var buf bytes.Buffer
	p := &JSONPrinter{}

	err := p.PrintObj(sampleResult(), &buf)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Equal(t, "my-bundle", decoded["name"])
	assert.Equal(t, "1.0.0", decoded["version"])
	assert.Equal(t, "should-not-appear", decoded["hidden"])

	items, ok := decoded["items"].([]any)
	require.True(t, ok)
	assert.Len(t, items, 2)
}

func TestYAMLPrinter_PrintObj(t *testing.T) {
	var buf bytes.Buffer
	p := &YAMLPrinter{}

	err := p.PrintObj(sampleResult(), &buf)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, yaml.Unmarshal(buf.Bytes(), &decoded))
	assert.Equal(t, "my-bundle", decoded["name"])
	assert.Equal(t, "1.0.0", decoded["version"])

	items, ok := decoded["items"].([]any)
	require.True(t, ok)
	assert.Len(t, items, 2)
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input      string
		wantFormat Format
		wantErr    bool
	}{
		{input: "text", wantFormat: FormatText},
		{input: "TEXT", wantFormat: FormatText},
		{input: "", wantFormat: FormatText},
		{input: "json", wantFormat: FormatJSON},
		{input: "JSON", wantFormat: FormatJSON},
		{input: "yaml", wantFormat: FormatYAML},
		{input: "YAML", wantFormat: FormatYAML},
		{input: "xml", wantErr: true},
		{input: "table", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			f, err := ParseFormat(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantFormat, f)
			}
		})
	}
}

func TestTextPrinter_UnsupportedTypes(t *testing.T) {
	type withUnsupported struct {
		Name    string         `text:"Name"`
		Data    map[string]any `text:"Data"`
		Channel chan int        `text:"Channel"`
	}

	var buf bytes.Buffer
	p := &TextPrinter{}

	obj := withUnsupported{Name: "test"}
	err := p.PrintObj(obj, &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Name:")
	assert.Contains(t, out, "test")
	assert.NotContains(t, out, "Data:", "map fields should be silently skipped")
	assert.NotContains(t, out, "Channel:", "chan fields should be silently skipped")
}

func TestTextPrinter_NilInput(t *testing.T) {
	var buf bytes.Buffer
	p := &TextPrinter{}

	err := p.PrintObj(nil, &buf)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestNewPrinter(t *testing.T) {
	tests := []struct {
		format  Format
		wantErr bool
	}{
		{format: FormatText},
		{format: FormatJSON},
		{format: FormatYAML},
		{format: Format("invalid"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			p, err := NewPrinter(tt.format)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, p)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, p)
			}
		})
	}
}
