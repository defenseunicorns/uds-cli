package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Handler is a generic SSE handler that sends data to the client
func Handler[T any](w http.ResponseWriter, r *http.Request, getData func() T, changes <-chan struct{}) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	sendData := func() {
		data, err := json.Marshal(getData())
		if err != nil {
			fmt.Fprintf(w, "data: Error: %v\n\n", err)
			flusher.Flush()
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	sendData()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-changes:
			sendData()
		}
	}
}
