// Package graphql implements the parts of GraphQL upload support that need
// reflection, keeping them out of the client's public surface.
//
// A GraphQL multipart request sends its files in their own parts of the body
// and points each one at the variables it fills through a "map" field holding
// paths such as "variables.input.avatar". Those paths have to match the JSON
// the query marshals to exactly, which is what CollectUploads derives, and
// BuildMultipartBody assembles the body they describe.
package graphql

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// maxDepth caps how deep CollectUploads walks into a query's variables. The
// limit keeps a cyclic value - a map holding itself, say - from recursing
// until the stack runs out, in the way encoding/json guards its own walk.
// Query variables that are legitimately this deeply nested do not occur.
const maxDepth = 1000

var (
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
)

// Ref is an upload found in a query's variables, together with every path in
// the marshaled query at which it appears.
type Ref[T any] struct {
	Value *T
	Paths []string
}

// CollectUploads returns every value of type T reachable from variables,
// together with the paths those values occupy in the JSON the query marshals
// to, for example "variables.input.avatar" or "variables.input.files.0". The
// paths make up the "map" field of a multipart GraphQL request, so they have
// to match the marshaled operations exactly: struct fields are named by their
// json tag and embedded structs are flattened, mirroring encoding/json.
//
// Map keys are visited in sorted order, which keeps the returned order - and
// therefore the part names derived from it - deterministic. A value that is
// referenced from several paths is returned once with all of its paths, as
// the specification allows a single file to fill more than one variable.
//
// Values that encoding/json does not marshal from the Go types alone - those
// below a custom marshaler, or below a map key that is neither a string nor
// an integer - have no path that can be derived. Rather than drop them, and
// send a query that silently omits a file the caller provided, CollectUploads
// reports an error naming where the value sits.
func CollectUploads[T any](variables map[string]any) ([]Ref[T], error) {
	if len(variables) == 0 {
		return nil, nil
	}

	c := &collector[T]{
		seen:       make(map[*T]int),
		targetType: reflect.TypeFor[T](),
	}
	c.targetPtrType = reflect.PointerTo(c.targetType)

	// "variables" is the root of every path, matching the field the query
	// marshals its variables into.
	c.walk("variables", reflect.ValueOf(variables), 0)
	if c.err != nil {
		return nil, c.err
	}

	return c.refs, nil
}

type collector[T any] struct {
	refs []Ref[T]
	seen map[*T]int
	err  error

	targetType    reflect.Type
	targetPtrType reflect.Type
}

// fail records the first error hit, which stops the walk.
func (c *collector[T]) fail(format string, args ...any) {
	if c.err == nil {
		c.err = fmt.Errorf(format, args...)
	}
}

func (c *collector[T]) add(upload *T, path string) {
	if i, ok := c.seen[upload]; ok {
		c.refs[i].Paths = append(c.refs[i].Paths, path)
		return
	}

	c.seen[upload] = len(c.refs)
	c.refs = append(c.refs, Ref[T]{Value: upload, Paths: []string{path}})
}

// walk records the uploads held by v, which marshals into the query at path.
func (c *collector[T]) walk(path string, v reflect.Value, depth int) {
	if c.err != nil {
		return
	}
	if depth > maxDepth {
		c.fail("GraphQL variables are nested too deeply at %s, or hold a cycle", path)
		return
	}

	// Unwrap interfaces and pointers to reach the concrete value, stopping
	// early on an upload, since that is the value being looked for.
	for v.IsValid() {
		switch v.Type() {
		case c.targetPtrType:
			if !v.IsNil() {
				c.add(v.Interface().(*T), path)
			}
			return
		case c.targetType:
			// An upload held by value instead of by pointer: copy it out to
			// have an address to key the file part on.
			upload := v.Interface().(T)
			c.add(&upload, path)
			return
		}

		if kind := v.Kind(); kind != reflect.Interface && kind != reflect.Pointer {
			break
		}
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}

	if !v.IsValid() {
		// Defensive: Elem() above never returns an invalid Value, since it is
		// only called on a Pointer or Interface already checked non-nil, and
		// neither can hold "no value" while being non-nil.
		return
	}

	// A custom marshaler decides the JSON shape of its own value, so paths
	// below it cannot be derived from the Go types alone. An upload down
	// there would be dropped silently, so it is reported instead.
	if t := v.Type(); implementsMarshaler(t) || implementsMarshaler(reflect.PointerTo(t)) {
		if c.contains(v, 0) {
			c.fail("GraphQL upload at %s is held by %s, which marshals itself to JSON: "+
				"the upload's position in the request cannot be determined", path, t)
		}
		return
	}

	switch v.Kind() {
	case reflect.Map:
		c.walkMap(path, v, depth)
	case reflect.Slice, reflect.Array:
		// A byte slice marshals to a base64 string, not to a JSON array, and
		// cannot hold an upload either way.
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return
		}
		for i := range v.Len() {
			c.walk(path+"."+strconv.Itoa(i), v.Index(i), depth+1)
		}
	case reflect.Struct:
		c.walkStruct(path, v, depth)
	}
}

func (c *collector[T]) walkMap(path string, v reflect.Value, depth int) {
	if v.IsNil() {
		return
	}

	names := make([]string, 0, v.Len())
	values := make(map[string]reflect.Value, v.Len())
	for _, key := range v.MapKeys() {
		name, ok := objectKey(key)
		if !ok {
			// encoding/json names such a key through its TextMarshaler, which
			// the Go types alone do not give, so an upload under it would be
			// dropped silently.
			if c.contains(v.MapIndex(key), 0) {
				c.fail("GraphQL upload at %s is held under a map key of type %s, "+
					"which is neither a string nor an integer: the upload's position "+
					"in the request cannot be determined", path, key.Type())
				return
			}
			continue
		}
		names = append(names, name)
		values[name] = v.MapIndex(key)
	}
	slices.Sort(names)

	for _, name := range names {
		c.walk(path+"."+name, values[name], depth+1)
	}
}

// walkStruct walks a struct's fields the way encoding/json marshals them:
// unexported and `json:"-"` fields are skipped, fields are named by their json
// tag, anonymous fields without a tag name are flattened into the enclosing
// object, and a zero-valued `omitempty` or `omitzero` field is skipped since
// it marshals to nothing.
func (c *collector[T]) walkStruct(path string, v reflect.Value, depth int) {
	t := v.Type()

	for i := range t.NumField() {
		field := t.Field(i)
		if field.Anonymous {
			elemType := field.Type
			if elemType.Kind() == reflect.Pointer {
				elemType = elemType.Elem()
			}
			if field.PkgPath != "" && elemType.Kind() != reflect.Struct {
				// Ignore embedded fields of unexported non-struct types,
				// mirroring encoding/json. Embedded fields of unexported
				// struct types are not ignored, since they may have
				// exported fields.
				continue
			}
		} else if field.PkgPath != "" {
			continue
		}

		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")

		value := v.Field(i)
		if field.Anonymous && name == "" {
			embedded := value
			for embedded.Kind() == reflect.Pointer && !embedded.IsNil() {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct {
				c.walkStruct(path, embedded, depth+1)
				continue
			}
		}

		// A zero-valued omitzero field marshals to nothing at all, so there is
		// no path for an upload held by it to occupy. omitempty is treated the
		// same way, which is stricter than encoding/json - it has no effect on
		// a struct field, and skips a field that is empty rather than zero -
		// but the difference is only ever reached by a zero-valued upload,
		// which holds no file to send in the first place.
		if options := strings.Split(opts, ","); slices.Contains(options, "omitempty") ||
			slices.Contains(options, "omitzero") {
			if value.IsZero() {
				continue
			}
		}

		if name == "" {
			name = field.Name
		}
		c.walk(path+"."+name, value, depth+1)
	}
}

// contains reports whether an upload is reachable from v at all, ignoring the
// paths it sits at. It backs the errors raised for values whose marshaled
// shape cannot be derived, so it looks at every field rather than only the
// ones encoding/json would visit: a custom marshaler is free to emit an
// unexported field, and dropping the caller's file is worse than refusing a
// query that could have been sent.
func (c *collector[T]) contains(v reflect.Value, depth int) bool {
	if depth > maxDepth {
		return false
	}

	for v.IsValid() {
		switch v.Type() {
		case c.targetPtrType:
			return !v.IsNil()
		case c.targetType:
			return true
		}

		if kind := v.Kind(); kind != reflect.Interface && kind != reflect.Pointer {
			break
		}
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}

	if !v.IsValid() {
		// Defensive, see the identical check in walk.
		return false
	}

	switch v.Kind() {
	case reflect.Map:
		for _, key := range v.MapKeys() {
			if c.contains(v.MapIndex(key), depth+1) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return false
		}
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return false
		}
		for i := range v.Len() {
			if c.contains(v.Index(i), depth+1) {
				return true
			}
		}
	case reflect.Struct:
		for _, field := range v.Fields() {
			if c.contains(field, depth+1) {
				return true
			}
		}
	}

	return false
}

// objectKey returns the JSON object key encoding/json uses for a map key,
// which is the key itself for strings and its decimal form for integers. Any
// other key is named through its TextMarshaler, which the Go types alone do
// not give, so it is reported as unsupported.
func objectKey(key reflect.Value) (string, bool) {
	// A string key is used as it is, even when its type marshals to text,
	// while an integer key marshals through its TextMarshaler when it has
	// one. Map keys are not addressable, so only the value method set counts.
	if key.Kind() == reflect.String {
		return key.String(), true
	}
	if key.Type().Implements(textMarshalerType) {
		return "", false
	}

	switch key.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(key.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(key.Uint(), 10), true
	default:
		return "", false
	}
}

func implementsMarshaler(t reflect.Type) bool {
	return t.Implements(jsonMarshalerType) || t.Implements(textMarshalerType)
}

// File is one file part of a multipart GraphQL request, bound to the variable
// paths it fills.
type File struct {
	Filename    string
	ContentType string
	Content     []byte
	Paths       []string
}

// quoteEscaper escapes the values interpolated into a part's
// Content-Disposition header, mirroring mime/multipart.
var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

// BuildMultipartBody assembles the multipart/form-data body carrying
// operations and files, as described by the GraphQL multipart request
// specification (https://github.com/jaydenseric/graphql-multipart-request-spec):
// an "operations" field holding the marshaled query, a "map" field pointing
// each file part at the variable paths it fills, and one part per file, named
// after its key in the map. It returns the body and the Content-Type header
// that describes it.
func BuildMultipartBody(operations []byte, files []File) ([]byte, string, error) {
	fileMap := make(map[string][]string, len(files))
	for i, file := range files {
		fileMap[strconv.Itoa(i)] = file.Paths
	}

	fileMapJSON, err := json.Marshal(fileMap)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal GraphQL upload map: %w", err)
	}

	body := new(bytes.Buffer)
	contentType, err := writeMultipartBody(body, operations, fileMapJSON, files)
	if err != nil {
		return nil, "", err
	}

	return body.Bytes(), contentType, nil
}

// writeMultipartBody writes operations, the file map and every file to dst as
// multipart/form-data, and returns the Content-Type header that describes it.
// It is split out from BuildMultipartBody so tests can drive it with a
// destination that fails on demand, rather than the bytes.Buffer
// BuildMultipartBody always succeeds with.
func writeMultipartBody(dst io.Writer, operations, fileMapJSON []byte, files []File) (string, error) {
	w := multipart.NewWriter(dst)

	if err := writeOperationsField(w, operations); err != nil {
		return "", err
	}
	if err := writeMapField(w, fileMapJSON); err != nil {
		return "", err
	}
	for i, file := range files {
		if err := writeFilePart(w, i, file); err != nil {
			return "", err
		}
	}
	if err := closeMultipartWriter(w); err != nil {
		return "", err
	}

	return w.FormDataContentType(), nil
}

// writeOperationsField and the helpers below each wrap one write to w with
// the error message identifying which part of the body it belongs to, so
// writeMultipartBody's failure paths are exercised through the same code
// BuildMultipartBody runs, rather than relying on the exact number and size
// of writes mime/multipart makes internally, which is not part of its
// documented behavior.

func writeOperationsField(w *multipart.Writer, operations []byte) error {
	if err := w.WriteField("operations", string(operations)); err != nil {
		return fmt.Errorf("failed to write GraphQL operations field: %w", err)
	}
	return nil
}

func writeMapField(w *multipart.Writer, fileMapJSON []byte) error {
	if err := w.WriteField("map", string(fileMapJSON)); err != nil {
		return fmt.Errorf("failed to write GraphQL upload map field: %w", err)
	}
	return nil
}

// createFilePart opens the part file i is sent in, named after its key in the
// map field.
func createFilePart(w *multipart.Writer, i int, file File) (io.Writer, error) {
	// The values are escaped, rather than quoted with %q, so that a filename
	// keeps any non-ASCII characters it has.
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="`+strconv.Itoa(i)+
		`"; filename="`+quoteEscaper.Replace(file.Filename)+`"`)
	header.Set("Content-Type", file.ContentType)

	return w.CreatePart(header)
}

func writeFilePart(w *multipart.Writer, i int, file File) error {
	part, err := createFilePart(w, i, file)
	if err != nil {
		return fmt.Errorf("failed to create GraphQL file part for %q: %w", file.Filename, err)
	}
	if _, err := part.Write(file.Content); err != nil {
		return fmt.Errorf("failed to write GraphQL file part for %q: %w", file.Filename, err)
	}
	return nil
}

func closeMultipartWriter(w *multipart.Writer) error {
	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close GraphQL multipart body: %w", err)
	}
	return nil
}
