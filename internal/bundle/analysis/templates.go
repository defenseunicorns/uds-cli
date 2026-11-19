package analysis

import (
	"bytes"
	"io"
	"regexp"
	"strconv"

	"github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/hashicorp/hcl/v2"
)

var templatePosition = regexp.MustCompile(`:(\d+)(?::(\d+))?:`)

func (d document) checkTemplate(variables bundle.Variables, execute bool) *Diagnostic {
	tmpl, err := bundle.ParseValuesTemplate("values", string(d.text))
	message := "invalid values template syntax"
	if err == nil && execute {
		err = tmpl.Execute(io.Discard, map[string]any{"vars": variables})
		message = "values template evaluation failed; supply required variables through defaults, --config, or --set, or use --skip-template-evaluation"
	}
	if err == nil {
		return nil
	}
	result := d.at(nil, message)
	// Go templates expose positions in error text, which may also contain
	// evaluated secrets. Extract only coordinates, never publish the raw error.
	match := templatePosition.FindStringSubmatch(err.Error())
	if match == nil {
		return &result
	}
	line, _ := strconv.Atoi(match[1])
	column, _ := strconv.Atoi(match[2])
	offset := 0
	for n := 1; n < line; n++ {
		next := bytes.IndexByte(d.text[offset:], '\n')
		if next < 0 {
			return &result
		}
		offset += next + 1
	}
	if column == 0 {
		result.Line = line
		return &result
	}
	end := bytes.IndexByte(d.text[offset:], '\n')
	if end < 0 {
		end = len(d.text) - offset
	}
	offset += min(column-1, end)
	result = d.at(&hcl.Pos{Byte: offset}, message)
	return &result
}
