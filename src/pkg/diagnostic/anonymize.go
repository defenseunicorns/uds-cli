package diagnostic

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strings"
)

const (
	MASKED_TEXT        = "***MASKED***"
	MASKED_DYNAMIC_FMT = "***MASKED(%d)***"
)

// Anonymizer masks sensitive content in strings and streams.
type Anonymizer interface {
	// AnonymizeOutput processes an input string and replaces sensitive data.
	AnonymizeOutput(input string, force bool) string
	// AnonymizeStream reads from r and writes the sanitized output to w.
	AnonymizeStream(r io.Reader, w io.Writer) error

	AnonymizedEntries() int
}

// Builder configures patterns and behavior before creating an Anonymizer.
type Builder struct {
	patterns      []string // regex patterns to detect sensitive data
	dynamicMode   bool     // whether to preserve match length in mask
	filePath      string   // optional file path for loading patterns
	multiLineMode bool     // enable scanning across multiple lines
	metrics       bool     // whether to count replacements
}

// NewBuilder initializes a Builder with default sensitive patterns.
func NewBuilder() *Builder {
	return &Builder{patterns: defaultPatterns, metrics: true}
}

// WithPatterns overrides the Builder's pattern list for custom detection.
func (b *Builder) WithPatterns(patts []string) *Builder {
	b.patterns = patts
	return b
}

// WithPatternFile loads regex patterns from a newline-delimited file.
func (b *Builder) WithPatternFile(path string) *Builder {
	b.filePath = path
	return b
}

// WithDynamicMask enables masks that include the length of the original match.
func (b *Builder) WithDynamicMask() *Builder {
	b.dynamicMode = true
	return b
}

// WithMultiLine allows detection of patterns that span multiple lines (e.g., PEM).
func (b *Builder) WithMultiLine() *Builder {
	b.multiLineMode = true
	return b
}

// WithMetrics turns on tracking of how many replacements have been made.
func (b *Builder) WithMetrics() *Builder {
	b.metrics = true
	return b
}

// Build compiles the regex and returns a streamingAnonymizer based on Builder settings.
func (b *Builder) Build() (Anonymizer, error) {
	// Load custom patterns from file if specified
	if b.filePath != "" {
		data, err := ioutil.ReadFile(b.filePath)
		if err != nil {
			return nil, fmt.Errorf("load patterns: %w", err)
		}
		lines := strings.Split(string(data), "\n")
		var filtered []string
		for _, l := range lines {
			if t := strings.TrimSpace(l); t != "" {
				filtered = append(filtered, t)
			}
		}
		b.patterns = filtered
	}

	// Combine all patterns into a single regex for efficient matching
	expr := strings.Join(b.patterns, "|")
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("compile regex: %w", err)
	}

	// Create the anonymizer instance with configured options
	return &streamingAnonymizer{
		r:          re,
		dynamic:    b.dynamicMode,
		metrics:    b.metrics,
		replaceCnt: 0,
		multiLine:  b.multiLineMode,
		bufferSize: bufio.MaxScanTokenSize * 16,
	}, nil
}

// streamingAnonymizer applies a single compiled regex for masking operations.
type streamingAnonymizer struct {
	r          *regexp.Regexp
	dynamic    bool
	metrics    bool
	replaceCnt int
	multiLine  bool
	bufferSize int
}

func (s *streamingAnonymizer) AnonymizedEntries() int {
	return s.replaceCnt
}

// AnonymizeOutput replaces all regex matches in the input string with masks.
func (s *streamingAnonymizer) AnonymizeOutput(input string, force bool) string {
	if force {
		s.replaceCnt = s.replaceCnt + 1
		return MASKED_TEXT
	}

	replace := func(match string) string {
		// Increment replacement count if metrics enabled
		if s.metrics {
			s.replaceCnt = s.replaceCnt + 1
		}
		// Choose mask format: dynamic preserves length, static is uniform
		if s.dynamic {
			return fmt.Sprintf(MASKED_DYNAMIC_FMT, len(match))
		}
		fmt.Printf("Anonymized: %s\n", match)
		return MASKED_TEXT
	}
	// Perform a single-pass replace using the compiled regex
	return s.r.ReplaceAllStringFunc(input, replace)
}

// AnonymizeStream scans input from r and writes masked output to w, handling
// either multi-line or line-by-line modes based on configuration.
func (s *streamingAnonymizer) AnonymizeStream(r io.Reader, w io.Writer) error {
	if s.multiLine {
		// Read entire stream for patterns that span lines
		data, err := ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(s.AnonymizeOutput(string(data), false)))
		return err
	}

	// Default: buffered line-by-line scanning to manage memory usage
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, bufio.MaxScanTokenSize), s.bufferSize)
	for sc.Scan() {
		sanitized := s.AnonymizeOutput(sc.Text(), false)
		if _, err := w.Write([]byte(sanitized + "\n")); err != nil {
			return err
		}
	}
	return sc.Err()
}

// defaultPatterns lists common regex patterns for sensitive data detection.
// Comments explain each pattern's intent.
var defaultPatterns = []string{
	`AKIA[0-9A-Z]{16}`, // AWS Access Key ID format (16-char suffix)
	//`[A-Za-z0-9/+=]{40}`, // AWS Secret Access Key (40 base64 chars)
	//`([A-Za-z0-9_-]+\.){2}[A-Za-z0-9_-]+`, // JWT token (three base64url segments)
	//`[A-Za-z0-9+/]{20,}={0,2}`,            // Generic Base64 blob (>=20 chars)
	//`\b\d{1,3}(?:\.\d{1,3}){3}\b`,         // IPv4 addresses
	//`[\w.\-]+@[\w.\-]+\.\w+`,           // Email addresses
	`https?://[^:\s]+:[^@\s]+@[^/\s]+`, // URLs with embedded credentials (user:pass@)
}
