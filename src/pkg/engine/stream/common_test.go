package stream

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8sTesting "k8s.io/client-go/testing"
)

// MockReader is a mock implementation of the Reader interface
type MockReader struct {
	mock.Mock
}

func (m *MockReader) PodFilter(pods []corev1.Pod) map[string]string {
	args := m.Called(pods)
	return args.Get(0).(map[string]string)
}

func (m *MockReader) LogStream(writer io.Writer, logStream io.ReadCloser) error {
	args := m.Called(writer, logStream)
	return args.Error(0)
}

func (m *MockReader) LogFlush(writer io.Writer) {
	m.Called(writer)
}

func TestStream_Start(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		pods          *corev1.PodList
		filterResult  map[string]string
		logStreamData string
		follow        bool
		since         time.Duration
		expectError   bool
	}{
		{
			name:      "Successful log streaming",
			namespace: "default",
			pods: &corev1.PodList{
				Items: []corev1.Pod{
					{ObjectMeta: v1.ObjectMeta{Name: "pod1"}},
				},
			},
			filterResult:  map[string]string{"pod1": "container1"},
			logStreamData: "log data",
			follow:        false,
			since:         0,
			expectError:   false,
		},
		{
			name:        "Error getting pods",
			namespace:   "default",
			pods:        &corev1.PodList{},
			expectError: true,
		},
		{
			name:      "Error streaming logs",
			namespace: "default",
			pods: &corev1.PodList{
				Items: []corev1.Pod{
					{ObjectMeta: v1.ObjectMeta{Name: "pod1"}},
				},
			},
			filterResult: map[string]string{"pod1": "container1"},
			follow:       false,
			since:        0,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(tt.pods)

			// Simulate error for the "Error getting pods" case
			if tt.name == "Error getting pods" {
				client.PrependReactor("list", "pods", func(action k8sTesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, errors.New("unable to get pods")
				})
			}

			reader := new(MockReader)
			if tt.name != "Error getting pods" {
				reader.On("PodFilter", mock.Anything).Return(tt.filterResult)
				reader.On("LogStream", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
					writer := args.Get(0).(io.Writer)
					_, _ = writer.Write([]byte(tt.logStreamData))
				})
			}

			var writer bytes.Buffer
			stream := NewStream(&writer, reader, tt.namespace)
			stream.Client = client
			stream.Follow = tt.follow
			stream.Since = tt.since

			err := stream.Start()
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.pods != nil && tt.name == "Successful log streaming" {
					require.Contains(t, writer.String(), tt.logStreamData)
				}
			}

			reader.AssertExpectations(t)
		})
	}
}
