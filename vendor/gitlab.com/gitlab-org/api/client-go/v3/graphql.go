package gitlab

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"gitlab.com/gitlab-org/api/client-go/v3/internal/graphql"
)

const (
	// GraphQLAPIEndpoint defines the endpoint URI for the GraphQL backend
	GraphQLAPIEndpoint = "/api/graphql"

	// graphQLUploadDefaultContentType is the Content-Type sent for an
	// upload that does not specify one.
	graphQLUploadDefaultContentType = "application/octet-stream"
)

type (
	GraphQLInterface interface {
		Do(query GraphQLQuery, response any, options ...RequestOptionFunc) (*Response, error)
	}

	GraphQL struct {
		client *Client
	}

	GraphQLQuery struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables,omitempty"`
	}

	// GraphQLUpload is a file to upload as part of a GraphQL query. Create
	// one with [NewGraphQLUpload].
	//
	// Set one wherever a mutation input expects a GraphQL Upload scalar,
	// nested at any depth inside GraphQLQuery.Variables, and Do sends the
	// query as a multipart request following the GraphQL multipart request
	// specification, https://github.com/jaydenseric/graphql-multipart-request-spec,
	// which GitLab implements through apollo_upload_server. An upload always
	// marshals to null in the query itself; the file travels in its own part
	// of the multipart body.
	//
	// Uploading several files works the same way, including for arguments
	// that take a list of uploads: assign an upload to every element that
	// carries a file. Assigning the same *GraphQLUpload to more than one
	// variable sends its content once and points every variable at it.
	//
	// An upload holds its own copy of the file, so it can be sent as many
	// times as needed, and is safe to use from several queries at once.
	//
	// Do finds an upload by the position it marshals to, which it derives the
	// way encoding/json does. A type that marshals itself - one implementing
	// json.Marshaler or encoding.TextMarshaler - decides its own JSON shape,
	// so an upload held inside one has no position that can be derived, and
	// Do reports an error rather than sending the query without the file. The
	// same goes for an upload under a map key that is neither a string nor an
	// integer. Hold uploads in plain maps, slices and structs to avoid this.
	//
	// Example:
	//
	//	avatar, err := gitlab.NewGraphQLUpload(f, "avatar.png", "image/png")
	//	if err != nil {
	//		return err
	//	}
	//
	//	input := map[string]any{
	//		"namespaceId": "gid://gitlab/Namespace/1",
	//		"name":        "First Commit",
	//		"avatar":      avatar,
	//	}
	//
	// Experimental: reflection derives an upload's position from the marshaled
	// query, and an edge case it does not handle correctly may require this
	// type or its behavior to change in a breaking way to fix.
	GraphQLUpload struct {
		// content holds the file's data, buffered by NewGraphQLUpload so
		// that every request - including a retried one - can write it
		// again.
		content []byte

		// filename is reported to GitLab as the uploaded file's name.
		// GitLab does not treat a part without a filename as an uploaded
		// file, so NewGraphQLUpload requires it.
		filename string

		// contentType is sent as the file part's Content-Type.
		contentType string
	}

	GenericGraphQLErrors struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	GraphQLResponseError struct {
		Err    error
		Errors GenericGraphQLErrors
	}
)

var _ GraphQLInterface = (*GraphQL)(nil)

func (e *GraphQLResponseError) Error() string {
	if len(e.Errors.Errors) == 0 {
		return fmt.Sprintf("%s (no additional error messages)", e.Err)
	}

	var sb strings.Builder
	sb.WriteString(e.Err.Error())
	sb.WriteString(" (GraphQL errors: ")

	for i, err := range e.Errors.Errors {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(err.Message)
	}
	sb.WriteString(")")

	return sb.String()
}

// Do sends a GraphQL query and returns the response in the given response argument
// The response must be JSON serializable. The *Response return value is the HTTP response
// and must be used to retrieve additional HTTP information, like status codes and also
// error messages from failed queries.
//
// Example:
//
//	var response struct {
//		Data struct {
//			Project struct {
//				ID string `json:"id"`
//			} `json:"project"`
//		} `json:"data"`
//	}
//	_, err := client.GraphQL.Do(GraphQLQuery{Query: `query { project(fullPath: "gitlab-org/gitlab") { id } }`}, &response, gitlab.WithContext(ctx))
//
// When the query's variables contain one or more GraphQLUpload values, the
// query is sent as a multipart request that carries the files alongside it.
//
// Attention: This API is experimental and may be subject to breaking changes to improve the API in the future.
func (g *GraphQL) Do(query GraphQLQuery, response any, options ...RequestOptionFunc) (*Response, error) {
	var (
		request *retryablehttp.Request
		err     error
	)

	uploads, err := graphql.CollectUploads[GraphQLUpload](query.Variables)
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL request: %w", err)
	}

	if len(uploads) > 0 {
		request, err = g.newUploadRequest(query, uploads, options)
	} else {
		request, err = g.client.NewRequest(http.MethodPost, "", query, options)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL request: %w", err)
	}

	// Overwrite the path of the existing request, as otherwise client-go appends /api/v4 instead.
	request.URL.Path = strings.Replace(request.URL.Path, "/"+apiVersionPath, GraphQLAPIEndpoint, 1)
	resp, err := g.client.Do(request, response)
	if err != nil {
		// return error, details can be read from Response
		if errResp, ok := err.(*ErrorResponse); ok { //nolint:errorlint
			var v GenericGraphQLErrors
			if json.Unmarshal(errResp.Body, &v) == nil {
				return resp, &GraphQLResponseError{
					Err:    err,
					Errors: v,
				}
			}
		}
		return resp, fmt.Errorf("failed to execute GraphQL query: %w", err)
	}
	return resp, nil
}

// NewGraphQLUpload returns a GraphQLUpload that sends the data of content
// under the given filename, which is required as GitLab does not treat a
// part without a filename as an uploaded file. contentType is sent as the
// file part's Content-Type and defaults to application/octet-stream.
//
// content is read in full and buffered in memory, so the returned upload is
// independent of it: the query it is used in can be sent repeatedly and
// retried, and a failure to build one part of a query does not leave the
// other uploads half consumed. content is not closed, so a caller that
// opened a file keeps ownership of it.
//
// Experimental: see [GraphQLUpload].
func NewGraphQLUpload(content io.Reader, filename, contentType string) (*GraphQLUpload, error) {
	if content == nil {
		return nil, fmt.Errorf("GraphQL upload %q has no content", filename)
	}
	if filename == "" {
		return nil, errors.New("GraphQL upload has no filename")
	}
	if contentType == "" {
		contentType = graphQLUploadDefaultContentType
	}

	buf, err := io.ReadAll(content)
	if err != nil {
		return nil, fmt.Errorf("failed to read content of GraphQL upload %q: %w", filename, err)
	}

	return &GraphQLUpload{
		content:     buf,
		filename:    filename,
		contentType: contentType,
	}, nil
}

// MarshalJSON implements json.Marshaler and always returns a JSON null. The
// GraphQL multipart request specification requires the query to hold null at
// every upload's position, with the file sent as a separate part of the
// multipart body.
func (GraphQLUpload) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

// newUploadRequest builds the multipart/form-data request carrying query and
// its uploads, following the GraphQL multipart request specification
// (https://github.com/jaydenseric/graphql-multipart-request-spec).
func (g *GraphQL) newUploadRequest(query GraphQLQuery, uploads []graphql.Ref[GraphQLUpload], options []RequestOptionFunc) (*retryablehttp.Request, error) {
	files := make([]graphql.File, 0, len(uploads))
	for _, ref := range uploads {
		// An upload that did not come from NewGraphQLUpload holds no content
		// and no filename, and cannot be sent as a file.
		if ref.Value.filename == "" {
			return nil, fmt.Errorf("GraphQL upload at %s was not created with NewGraphQLUpload", strings.Join(ref.Paths, ", "))
		}

		files = append(files, graphql.File{
			Filename:    ref.Value.filename,
			ContentType: ref.Value.contentType,
			Content:     ref.Value.content,
			Paths:       ref.Paths,
		})
	}

	operations, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL operations: %w", err)
	}

	body, contentType, err := graphql.BuildMultipartBody(operations, files)
	if err != nil {
		return nil, err
	}

	request, err := g.client.NewRequest(http.MethodPost, "", nil, options)
	if err != nil {
		return nil, err
	}

	// Set the body from a byte slice rather than a buffer, so that the
	// request stays replayable when it is retried.
	if err := request.SetBody(body); err != nil {
		return nil, fmt.Errorf("failed to set GraphQL multipart body: %w", err)
	}
	request.Header.Set("Content-Type", contentType)

	return request, nil
}

// gidGQL is a global ID. It is used by GraphQL to uniquely identify resources.
type gidGQL struct {
	Type  string
	Int64 int64
}

// newGIDStrings creates a slice of global IDs from a type and a slice of IDs.
// This is useful when getting a slice of int64 ids from the user and having to
// populate a string slice for GraphQL.
func newGIDStrings(typ string, ids ...int64) []string {
	ret := make([]string, 0, len(ids))

	for _, id := range ids {
		ret = append(ret, gidGQL{typ, id}.String())
	}

	return ret
}

var gidGQLRegex = regexp.MustCompile(`^gid://gitlab/([^/]+)/(\d+)$`)

func (id *gidGQL) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	m := gidGQLRegex.FindStringSubmatch(s)
	if len(m) != 3 {
		return fmt.Errorf("invalid global ID format: %q", s)
	}

	i, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return fmt.Errorf("failed parsing %q as numeric ID: %w", s, err)
	}

	id.Type = m[1]
	id.Int64 = i

	return nil
}

func (id gidGQL) IsZero() bool {
	return id.Type == "" && id.Int64 == 0
}

func (id gidGQL) String() string {
	return fmt.Sprintf("gid://gitlab/%s/%d", id.Type, id.Int64)
}

// iidGQL represents an int64 ID that is encoded by GraphQL as a string.
// This type is used unmarshal the string response into an int64 type.
type iidGQL int64

func (id *iidGQL) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("failed parsing %q as numeric ID: %w", s, err)
	}

	*id = iidGQL(i)
	return nil
}

// PageInfo contains cursor-based pagination metadata for GraphQL connections following the Relay
// cursor pagination specification. Use EndCursor and HasNextPage for forward pagination
// (most common), or StartCursor and HasPreviousPage for backward pagination.
//
// Cursors are opaque strings that should not be parsed or constructed manually - always
// use the cursors returned by the API.
//
// Note: GraphQL cursor pagination differs from GitLab's REST API keyset pagination.
// In REST, the pagination link points to the first item of the next page. In GraphQL,
// EndCursor points to the last item of the current page - you pass this to the "after"
// parameter to fetch items after it (essentially an off-by-one difference in semantics).
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#pageinfo
type PageInfo struct {
	EndCursor       string `json:"endCursor"`       // Cursor of the last item in this page (pass to "after" for next page)
	HasNextPage     bool   `json:"hasNextPage"`     // True if more items exist after this page
	StartCursor     string `json:"startCursor"`     // Cursor of the first item in this page (pass to "before" for previous page)
	HasPreviousPage bool   `json:"hasPreviousPage"` // True if items exist before this page
}

// connectionGQL represents a paginated GraphQL connection response following the Relay
// cursor pagination specification. It wraps a list of nodes of any type T along with
// pagination metadata. This type is used internally to unmarshal GraphQL responses from
// GitLab's API, which consistently uses this connection pattern for all paginated fields.
//
// The PageInfo field provides cursors and flags for iterating through pages, while Nodes
// contains the actual data items for the current page.
//
// GitLab API docs: https://docs.gitlab.com/api/graphql/reference/#connection-fields
type connectionGQL[T any] struct {
	PageInfo PageInfo `json:"pageInfo"`
	Nodes    []T      `json:"nodes"`
}
