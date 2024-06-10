package pepr

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewStreamReader(t *testing.T) {
	timestamp := true
	filterNamespace := "test-namespace"
	filterName := "test-name"

	reader := NewStreamReader(context.TODO(), timestamp, filterNamespace, filterName)

	require.True(t, reader.showTimestamp)
	require.Equal(t, "                     ", reader.indent)
	require.Equal(t, "test-namespace", reader.filterNamespace)
	require.Equal(t, "test-name", reader.filterName)
}

func TestPodFilter(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pepr-admission-controller",
				Labels: map[string]string{
					"pepr.dev/controller": "admission",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pepr-watcher",
				Labels: map[string]string{
					"pepr.dev/controller": "watcher",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "other-pod",
				Labels: map[string]string{
					"app": "other",
				},
			},
		},
	}

	tests := []struct {
		name         string
		filterStream StreamKind
		expected     map[string]string
	}{
		{
			name:         "AnyStream",
			filterStream: AnyStream,
			expected: map[string]string{
				"pepr-admission-controller": "server",
				"pepr-watcher":              "watcher",
			},
		},
		{
			name:         "PolicyStream",
			filterStream: PolicyStream,
			expected: map[string]string{
				"pepr-admission-controller": "server",
			},
		},
		{
			name:         "OperatorStream",
			filterStream: OperatorStream,
			expected: map[string]string{
				"pepr-watcher": "watcher",
			},
		},
		{
			name:         "FailureStream",
			filterStream: FailureStream,
			expected: map[string]string{
				"pepr-admission-controller": "server",
				"pepr-watcher":              "watcher",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &StreamReader{FilterStream: tt.filterStream}
			result := reader.PodFilter(pods)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestLogStream(t *testing.T) {
	testCases := []struct {
		name            string
		filterStream    StreamKind
		logs            string
		expected        string
		filterName      string
		filterNamespace string
	}{
		{
			name:         "OperatorProcessing",
			filterStream: OperatorStream,
			logs:         `{"level":30,"time":1718253072766,"pid":16,"hostname":"pepr-uds-core-watcher-54bdf86f7d-2r75t","apiVersion":"uds.dev/v1alpha1","kind":"Package","metadata":{"annotations":{"meta.helm.sh/release-name":"zarf-771a1524217aba2462fb2313567606ed2f45a76a","meta.helm.sh/release-namespace":"default"},"creationTimestamp":"2024-06-12T08:05:10Z","generation":1,"labels":{"app.kubernetes.io/managed-by":"Helm"},"managedFields":[{"apiVersion":"uds.dev/v1alpha1","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:annotations":{".":{},"f:meta.helm.sh/release-name":{},"f:meta.helm.sh/release-namespace":{}},"f:labels":{".":{},"f:app.kubernetes.io/managed-by":{}}},"f:spec":{".":{},"f:network":{".":{},"f:expose":{}}}},"manager":"zarf","operation":"Update","time":"2024-06-12T08:05:10Z"},{"apiVersion":"uds.dev/v1alpha1","fieldsType":"FieldsV1","fieldsV1":{"f:status":{".":{},"f:endpoints":{},"f:monitors":{},"f:networkPolicyCount":{},"f:observedGeneration":{},"f:phase":{},"f:retryAttempt":{},"f:ssoClients":{}}},"manager":"kubernetes-fluent-client","operation":"Update","subresource":"status","time":"2024-06-13T04:31:12Z"}],"name":"httpbin","namespace":"test-admin-app","resourceVersion":"889512","uid":"3c787c80-41ec-4be0-aaac-3c9c5b71e3e6"},"spec":{"network":{"expose":[{"advancedHTTP":{"match":[{"method":{"regex":"GET"},"name":"test-get-and-prefix","uri":{"prefix":"/status/2"}},{"name":"test-exact","uri":{"exact":"/status/410"}}]},"gateway":"admin","host":"demo","port":8000,"selector":{"app":"httpbin"},"service":"httpbin","targetPort":80}]}},"status":{"endpoints":["demo.admin.uds.dev"],"monitors":[],"networkPolicyCount":5,"observedGeneration":1,"phase":"Ready","retryAttempt":0,"ssoClients":[]},"msg":"Processing Package test-admin-app/httpbin"}`,
			expected:     "\n\n ⚙️ OPERATOR  test-admin-app/httpbin\n                     Processing Package test-admin-app/httpbin",
		},
		{
			name:         "OperatorStatus",
			filterStream: OperatorStream,
			logs:         `{"level":20,"time":1718179510454,"pid":16,"hostname":"pepr-uds-core-watcher-54bdf86f7d-5dm5v","namespace":"test-admin-app","name":"httpbin","msg":"Updating status to Ready"}`,
			expected:     "\n\n ⚙️ OPERATOR  test-admin-app/httpbin\n                     Updating status to Ready",
		},
		{
			name:         "PolicyMutate",
			filterStream: MutateStream,
			logs:         `{"level":30,"time":1718179626761,"pid":16,"hostname":"pepr-uds-core-57cfb74897-wxj95","uid":"aaa8f8ef-c0cc-4719-aac2-4e37dc6d9629","namespace":"policy-tests","name":"/network-node-port","res":{"uid":"aaa8f8ef-c0cc-4719-aac2-4e37dc6d9629","allowed":true,"patchType":"JSONPatch","patch":"W3sib3AiOiJhZGQiLCJwYXRoIjoiL21ldGFkYXRhL2Fubm90YXRpb25zIiwidmFsdWUiOnsidWRzLWNvcmUucGVwci5kZXYvdWRzLWNvcmUtcG9saWNpZXMiOiJzdWNjZWVkZWQifX1d"},"msg":"Check response"}`,
			expected:     "\n\n ✎ MUTATED   policy-tests/network-node-port\n                       ADDED:\n                     /metadata/annotations={\"uds-core.pepr.dev/uds-core-policies\":\"succeeded\"}",
		},
		{
			name:         "PolicyAllow",
			filterStream: AllowStream,
			logs:         `{"level":30,"time":1718179626856,"pid":16,"hostname":"pepr-uds-core-57cfb74897-wxj95","uid":"a3cb1a50-cf6b-4a2e-929b-c7755c56ec5c","namespace":"policy-tests","name":"/security-capabilities-drop","res":{"uid":"a3cb1a50-cf6b-4a2e-929b-c7755c56ec5c","allowed":true},"msg":"Check response"}`,
			expected:     "\n\n ✓ ALLOWED   policy-tests/security-capabilities-drop",
		},
		{
			name:         "PolicyDeny",
			filterStream: DenyStream,
			logs:         `{"level":30,"time":1718179626867,"pid":16,"hostname":"pepr-uds-core-57cfb74897-wxj95","uid":"6e5f7670-3117-4b59-b6b3-6cbb152b04ef","namespace":"policy-tests","name":"/security-capabilities-add","res":{"uid":"6e5f7670-3117-4b59-b6b3-6cbb152b04ef","allowed":false,"status":{"code":400,"message":"Unauthorized container capabilities in securityContext.capabilities.add. Authorized: [NET_BIND_SERVICE] Found: {\"name\":\"test\",\"ctx\":{\"capabilities\":{\"add\":[\"NET_ADMIN\"],\"drop\":[\"ALL\"]}}}"}},"msg":"Check response"}`,
			expected:     "\n\n ✗ DENIED    policy-tests/security-capabilities-add\n                     Unauthorized container capabilities in securityContext.capabilities.add.\n\n                     Authorized:\n                     [NET_BIND_SERVICE]\n\n                     Found:\n                     {\"name\":\"test\",\"ctx\":{\"capabilities\":{\"add\":[\"NET_ADMIN\"],\"drop\":[\"ALL\"]}}}",
		},
		{
			name:            "FilterNamespace",
			filterStream:    AnyStream,
			logs:            `{"level":30,"time":1718179626867,"pid":16,"hostname":"pepr-uds-core-57cfb74897-wxj95","uid":"6e5f7670-3117-4b59-b6b3-6cbb152b04ef","namespace":"policy-tests","name":"/security-capabilities-add","res":{"uid":"6e5f7670-3117-4b59-b6b3-6cbb152b04ef","allowed":false,"status":{"code":400,"message":"Unauthorized container capabilities in securityContext.capabilities.add. Authorized: [NET_BIND_SERVICE] Found: {\"name\":\"test\",\"ctx\":{\"capabilities\":{\"add\":[\"NET_ADMIN\"],\"drop\":[\"ALL\"]}}}}},"msg":"Check response"}`,
			expected:        "",
			filterNamespace: "other-namespace",
		},
		{
			name:         "FilterName",
			filterStream: AnyStream,
			logs:         `{"level":30,"time":1718179626867,"pid":16,"hostname":"pepr-uds-core-57cfb74897-wxj95","uid":"6e5f7670-3117-4b59-b6b3-6cbb152b04ef","namespace":"policy-tests","name":"/security-capabilities-add","res":{"uid":"6e5f7670-3117-4b59-b6b3-6cbb152b04ef","allowed":false,"status":{"code":400,"message":"Unauthorized container capabilities in securityContext.capabilities.add. Authorized: [NET_BIND_SERVICE] Found: {\"name\":\"test\",\"ctx\":{\"capabilities\":{\"add\":[\"NET_ADMIN\"],\"drop\":[\"ALL\"]}}}}},"msg":"Check response"}`,
			expected:     "",
			filterName:   "other-name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := NewStreamReader(context.TODO(), false, tc.filterNamespace, tc.filterName)
			reader.FilterStream = tc.filterStream

			var buf bytes.Buffer
			err := reader.LogStream(&buf, io.NopCloser(strings.NewReader(tc.logs)))
			require.NoError(t, err)

			expected := normalizeWhitespace(tc.expected)
			actual := normalizeWhitespace(buf.String())

			require.Equal(t, expected, actual)
		})
	}
}

func TestLogFlush(t *testing.T) {
	reader := &StreamReader{
		lastEntryHeader: " ✓ ALLOWED   policy-tests/security-capabilities-drop",
		repeatCount:     3,
	}

	var buf bytes.Buffer
	reader.LogFlush(&buf)

	expected := " (repeated 3 times)"
	require.Equal(t, expected, buf.String())
}

func TestSkipResource(t *testing.T) {
	testCases := []struct {
		name            string
		filterNamespace string
		filterName      string
		event           LogEntry
		expected        bool
	}{
		{
			name:            "MatchingNamespaceAndName",
			filterNamespace: "policy-tests",
			filterName:      "security-capabilities-add",
			event: LogEntry{
				Namespace: "policy-tests",
				Name:      "/security-capabilities-add",
			},
			expected: false,
		},
		{
			name:            "MatchingNamespace",
			filterNamespace: "policy-tests",
			filterName:      "",
			event: LogEntry{
				Namespace: "policy-tests",
				Name:      "/security-capabilities-add",
			},
			expected: false,
		},
		{
			name:            "MatchingName",
			filterNamespace: "",
			filterName:      "security-capabilities-add",
			event: LogEntry{
				Namespace: "policy-tests",
				Name:      "/security-capabilities-add",
			},
			expected: false,
		},
		{
			name:            "NonMatchingNamespace",
			filterNamespace: "other-namespace",
			filterName:      "",
			event: LogEntry{
				Namespace: "policy-tests",
				Name:      "/security-capabilities-add",
			},
			expected: true,
		},
		{
			name:            "NonMatchingName",
			filterNamespace: "",
			filterName:      "other-name",
			event: LogEntry{
				Namespace: "policy-tests",
				Name:      "/security-capabilities-add",
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := &StreamReader{
				filterNamespace: tc.filterNamespace,
				filterName:      tc.filterName,
			}

			result := reader.skipResource(tc.event)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestRenderDenied(t *testing.T) {
	testCases := []struct {
		name     string
		event    LogEntry
		expected string
	}{
		{
			name: "ValidDeniedMessage",
			event: LogEntry{
				Res: struct {
					Uid     string "json:\"uid\""
					Allowed bool   "json:\"allowed\""
					Status  struct {
						Code    int    "json:\"code\""
						Message string "json:\"message\""
					} "json:\"status\""
					Patch     *string "json:\"patch,omitempty\""
					PatchType *string "json:\"patchType,omitempty\""
				}{
					Status: struct {
						Code    int    "json:\"code\""
						Message string "json:\"message\""
					}{
						Message: "Unauthorized container capabilities in securityContext.capabilities.add. Authorized: [NET_BIND_SERVICE] Found: {\"name\":\"test\",\"ctx\":{\"capabilities\":{\"add\":[\"NET_ADMIN\"],\"drop\":[\"ALL\"]}}}",
					},
				},
			},
			expected: "\x1b[31m\n                     Unauthorized container capabilities in securityContext.capabilities.add.\x1b[0m\n\n\x1b[1m                     Authorized:\x1b[0m\n\x1b[38;5;246m                     [NET_BIND_SERVICE]\x1b[0m\n\n\x1b[1m                     Found:\x1b[0m\n\x1b[38;5;246m                     {\"name\":\"test\",\"ctx\":{\"capabilities\":{\"add\":[\"NET_ADMIN\"],\"drop\":[\"ALL\"]}}}\x1b[0m",
		},
		{
			name: "InvalidDeniedMessage",
			event: LogEntry{
				Res: struct {
					Uid     string "json:\"uid\""
					Allowed bool   "json:\"allowed\""
					Status  struct {
						Code    int    "json:\"code\""
						Message string "json:\"message\""
					} "json:\"status\""
					Patch     *string "json:\"patch,omitempty\""
					PatchType *string "json:\"patchType,omitempty\""
				}{
					Status: struct {
						Code    int    "json:\"code\""
						Message string "json:\"message\""
					}{
						Message: "Invalid message format",
					},
				},
			},
			expected: "\x1b[31m\n                     Invalid message format\x1b[0m",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := &StreamReader{}
			result := reader.renderDenied(tc.event)

			expected := normalizeWhitespace(tc.expected)
			actual := normalizeWhitespace(result)

			require.Equal(t, expected, actual)
		})
	}
}

func TestRenderMutation(t *testing.T) {
	testCases := []struct {
		name     string
		event    LogEntry
		expected string
	}{
		{
			name: "ValidMutation",
			event: LogEntry{
				Res: struct {
					Uid     string "json:\"uid\""
					Allowed bool   "json:\"allowed\""
					Status  struct {
						Code    int    "json:\"code\""
						Message string "json:\"message\""
					} "json:\"status\""
					Patch     *string "json:\"patch,omitempty\""
					PatchType *string "json:\"patchType,omitempty\""
				}{
					Patch: func(s string) *string { return &s }("W3sib3AiOiJhZGQiLCJwYXRoIjoiL21ldGFkYXRhL2Fubm90YXRpb25zIiwidmFsdWUiOnsidWRzLWNvcmUucGVwci5kZXYvdWRzLWNvcmUtcG9saWNpZXMiOiJzdWNjZWVkZWQifX1d"),
				},
			},
			expected: "\x1b[1m\n                       ADDED:\x1b[0m\n                     \x1b[38;5;246m/metadata/annotations\x1b[0m=\x1b[36m{\"uds-core.pepr.dev/uds-core-policies\":\"succeeded\"}\x1b[0m",
		},
		{
			name: "InvalidMutation",
			event: LogEntry{
				Res: struct {
					Uid     string "json:\"uid\""
					Allowed bool   "json:\"allowed\""
					Status  struct {
						Code    int    "json:\"code\""
						Message string "json:\"message\""
					} "json:\"status\""
					Patch     *string "json:\"patch,omitempty\""
					PatchType *string "json:\"patchType,omitempty\""
				}{},
			},
			expected: "No patch available",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := &StreamReader{}
			result := reader.renderMutation(tc.event)

			expected := normalizeWhitespace(tc.expected)
			actual := normalizeWhitespace(result)

			require.Equal(t, expected, actual)
		})
	}
}

// normalizeWhitespace removes leading/trailing whitespace and replaces multiple whitespace characters with a single space
func normalizeWhitespace(str string) string {
	// Remove ANSI escape sequences
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	str = re.ReplaceAllString(str, "")

	// Replace multiple whitespace characters with a single space
	re = regexp.MustCompile(`\s+`)
	str = re.ReplaceAllString(str, " ")

	// Trim leading and trailing whitespace
	return strings.TrimSpace(str)
}
