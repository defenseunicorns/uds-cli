package gitlab

import (
	"regexp"
	"slices"
	"strings"
	"sync"
)

// paramPlaceholder stands in for every path parameter. The client does not
// know the documented names for them, so they all share one placeholder.
const paramPlaceholder = ":id"

var verbPattern = regexp.MustCompile(`%[sdv]`)

// Route is an API route this client knows how to call, with its parameters
// replaced by a placeholder, for example "/projects/:id/merge_requests/:id".
//
// Routes are registered when the package is loaded, so [Routes] and
// [MatchRoute] describe the whole client, not just the calls made so far.
//
// Note: This API is experimental and may change or be removed in
// future versions.
type Route struct {
	path string
}

// String returns the route with its parameters replaced by a placeholder.
//
// Note: This API is experimental and may change or be removed in
// future versions.
func (r Route) String() string {
	return r.path
}

var (
	registryMu sync.Mutex

	// Keyed by normalized path, so templates differing only in their verbs
	// register as one route.
	registry = map[string]Route{}

	// Built on first use, once every route has registered.
	matcher = sync.OnceValue(buildMatcher)
)

// route registers a template and returns it unchanged, so it can be assigned to
// a package-level variable and still used as a format string:
//
//	var routeProjectsIDIssues = route("projects/%s/issues")
//
// Registering at package level rather than at call time is what lets [Routes]
// report routes that have never been called.
func route(template string) string {
	normalized := normalizeTemplate(template)

	registryMu.Lock()
	defer registryMu.Unlock()
	registry[normalized] = Route{path: normalized}

	return template
}

// normalizeTemplate replaces each parameter with paramPlaceholder. A verb
// embedded in a segment, as in "archive%s", is dropped so the literal part
// still identifies the route.
func normalizeTemplate(template string) string {
	segments := strings.Split(strings.Trim(template, "/"), "/")
	for i, segment := range segments {
		if verbPattern.MatchString(segment) {
			if verbPattern.FindString(segment) == segment {
				segments[i] = paramPlaceholder
			} else {
				segments[i] = verbPattern.ReplaceAllString(segment, "")
			}
		}
	}

	return "/" + strings.Join(segments, "/")
}

// Routes returns every API route this client knows how to call, sorted by path.
//
// Parameters are replaced by a placeholder, so the result is a bounded set of
// route shapes rather than concrete paths. It is suitable for grouping or
// labeling requests, such as reporting which endpoints an application uses
// without recording the identifiers in them.
//
// Note: This API is experimental and may change or be removed in
// future versions.
func Routes() []Route {
	registryMu.Lock()
	defer registryMu.Unlock()

	routes := make([]Route, 0, len(registry))
	for _, r := range registry {
		routes = append(routes, r)
	}

	slices.SortFunc(routes, func(a, b Route) int {
		return strings.Compare(a.path, b.path)
	})
	return routes
}

// MatchRoute reports the route a concrete API path belongs to, for example
// "projects/278964/merge_requests/1/notes" matches
// "/projects/:id/merge_requests/:id/notes".
//
// A literal segment is preferred over a parameter at every position, mirroring
// how the API itself routes. The path is matched case-insensitively and may be
// given with or without a leading slash. Query strings and fragments are not
// accepted; trim them first.
//
// The second return value reports whether a route matched. Paths this client
// cannot call do not match, which includes endpoints the API serves but the
// client has not implemented.
//
// Note: This API is experimental and may change or be removed in
// future versions.
func MatchRoute(path string) (Route, bool) {
	return matcher().match(splitPath(path))
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

// routeNode indexes routes by path segment, preferring literal children over
// the parameter child.
type routeNode struct {
	children map[string]*routeNode
	param    *routeNode
	route    Route
	terminal bool
}

func buildMatcher() *routeNode {
	registryMu.Lock()
	defer registryMu.Unlock()

	root := &routeNode{}
	for _, r := range registry {
		root.insert(r)
	}
	return root
}

func (n *routeNode) insert(r Route) {
	node := n
	for segment := range strings.SplitSeq(strings.Trim(r.path, "/"), "/") {
		if segment == paramPlaceholder {
			if node.param == nil {
				node.param = &routeNode{}
			}
			node = node.param
			continue
		}

		if node.children == nil {
			node.children = map[string]*routeNode{}
		}
		child, ok := node.children[segment]
		if !ok {
			child = &routeNode{}
			node.children[segment] = child
		}
		node = child
	}

	node.route = r
	node.terminal = true
}

func (n *routeNode) match(segments []string) (Route, bool) {
	if len(segments) == 0 {
		return n.route, n.terminal
	}

	head, rest := strings.ToLower(segments[0]), segments[1:]

	if child, ok := n.children[head]; ok {
		if r, matched := child.match(rest); matched {
			return r, true
		}
	}
	if n.param != nil {
		return n.param.match(rest)
	}

	return Route{}, false
}
