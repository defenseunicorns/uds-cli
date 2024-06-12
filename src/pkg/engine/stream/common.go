// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package stream contains the logic for streaming logs from from UDS Core
package stream

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/defenseunicorns/uds-cli/src/pkg/engine"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StreamReader interface {
	// PodFilter creates a map of pod and container names to pull logs from
	PodFilter(pods []corev1.Pod) map[string]string

	// LogStream processes the log stream from Pepr and writes formatted output to the writer
	LogStream(writer io.Writer, logStream io.ReadCloser) error

	// LogFlush to flush the log at a given interval and at the end of the stream
	LogFlush(writer io.Writer)
}

type Stream struct {
	writer    io.Writer
	reader    StreamReader
	follow    bool
	namespace string
}

func NewStream(writer io.Writer, reader StreamReader, namespace string) *Stream {
	return &Stream{
		writer:    writer,
		reader:    reader,
		namespace: namespace,
	}
}

// SetFollow sets the follow flag for the stream
func (s *Stream) SetFollow(follow bool) {
	s.follow = follow
}

// Start starts the stream
func (s *Stream) Start() error {
	c, err := engine.NewCluster()
	if err != nil {
		return fmt.Errorf("unable to connect to the cluster: %v", err)
	}

	pods, err := c.Clientset.CoreV1().Pods(s.namespace).List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return fmt.Errorf("unable to get pods: %v", err)
	}

	var wg sync.WaitGroup

	containers := s.reader.PodFilter(pods.Items)

	// Stream logs for each pod
	for pod, container := range containers {
		// Add a goroutine to the wait group for each pod
		wg.Add(1)
		// Run as a goroutine to stream logs for each pod without blocking
		go func(podName string, container string) {
			defer wg.Done()

			// Set up the pod log options
			podOpts := &corev1.PodLogOptions{Follow: s.follow, Container: container}

			// Get the log stream for the pod
			logStream, err := c.Clientset.CoreV1().Pods(s.namespace).GetLogs(podName, podOpts).Stream(context.TODO())
			if err != nil {
				log.Printf("Error streaming logs for pod %s: %v", podName, err)
				return
			}
			defer logStream.Close()

			if err := s.reader.LogStream(s.writer, logStream); err != nil {
				log.Printf("Error streaming logs for pod %s: %v", podName, err)
			}
		}(pod, container)
	}

	// Channel to signal the log flush goroutine to stop
	stopChan := make(chan struct{})

	// Need to flush logs if following or repeats won't be seen until the end of the stream
	if s.follow {
		go func() {
			// Final log flush when goroutine exits
			defer s.reader.LogFlush(s.writer)

			for {
				select {
				// Stop the goroutine when the stopChan is closed
				case <-stopChan:
					return
					// Flush the logs every second
				case <-time.After(time.Second):
					s.reader.LogFlush(s.writer)
				}
			}
		}()
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Signal the log flush goroutine to stop
	close(stopChan)

	return nil
}
