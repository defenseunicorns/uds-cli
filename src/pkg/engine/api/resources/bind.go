package resources

import (
	"net/http"

	"github.com/defenseunicorns/uds-cli/src/pkg/engine/api/sse"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Bind is a helper function to bind a cache to an SSE handler
func Bind[T metav1.Object](getData func() []T, changes <-chan struct{}) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		sse.Handler(w, r, getData, changes)
	}
}
