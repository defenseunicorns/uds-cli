// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package stream contains the logic for streaming logs from from UDS Core
package stream

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/defenseunicorns/uds-cli/src/pkg/engine/k8s"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Reader interface {
	// PodFilter creates a map of pod and container names to pull logs from
	PodFilter(pods []corev1.Pod) map[string]string

	// LogStream processes the log stream from Pepr and writes formatted output to the writer
	LogStream(writer io.Writer, logStream io.ReadCloser, timestamp bool) error

	// LogFlush to flush the log at a given interval and at the end of the stream
	LogFlush(writer io.Writer)
}

type Stream struct {
	writer     io.Writer
	reader     Reader
	Follow     bool
	Timestamps bool
	Namespace  string
	Since      time.Duration
	// Adding for testability :-<
	Client kubernetes.Interface
}

func NewStream(writer io.Writer, reader Reader, namespace string) *Stream {
	return &Stream{
		writer:    writer,
		reader:    reader,
		Namespace: namespace,
	}
}

// Start starts the stream with the provided context
func (s *Stream) Start(ctx context.Context) error {
	// Create a new client if one is not provided (usually for testing)
	if s.Client == nil {
		c, _, err := k8s.NewClient()
		if err != nil {
			return fmt.Errorf("unable to connect to the cluster: %v", err)
		}
		s.Client = c
	}

	// List the pods in the specified namespace
	pods, err := s.Client.CoreV1().Pods(s.Namespace).List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return fmt.Errorf("unable to get pods: %v", err)
	}

	var wg sync.WaitGroup

	// Filter the pods and containers to stream logs from
	containers := s.reader.PodFilter(pods.Items)

	// Stream logs for each pod
	for pod, container := range containers {
		// Add a goroutine to the wait group for each pod
		wg.Add(1)
		// Run as a goroutine to stream logs for each pod without blocking
		go func(podName, container string) {
			defer wg.Done()

			message.Warnf("Spawning goroutine for pod %s", podName)

			// Set up the pod log options
			podOpts := &corev1.PodLogOptions{
				Follow:     s.Follow,
				Container:  container,
				Timestamps: s.Timestamps,
			}

			// Set the sinceSeconds option if provided
			if s.Since != 0 {
				// round up to the nearest second
				sec := int64(s.Since.Round(time.Second).Seconds())
				podOpts.SinceSeconds = &sec
			}

			// Get the log stream for the pod
			logStream, err := s.Client.CoreV1().Pods(s.Namespace).GetLogs(podName, podOpts).Stream(ctx)
			if err != nil {
				message.WarnErrf(err, "Error streaming logs for pod %s", podName)
				return
			}
			defer logStream.Close()

			// Process the log stream
			if err := s.reader.LogStream(s.writer, logStream, s.Timestamps); err != nil {
				message.WarnErrf(err, "Error streaming logs for pod %s", podName)
			}
		}(pod, container)
	}

	// Channel to signal the log flush goroutine to stop
	stopChan := make(chan struct{})

	// Need to flush logs if following or repeats won't be seen until the end of the stream
	if s.Follow {
		go func() {
			// Final log flush when goroutine exits
			defer s.reader.LogFlush(s.writer)

			for {
				select {
				// Stop the goroutine when the stopChan is closed
				case <-stopChan:
					return
				// Stop the goroutine when the context is done
				case <-ctx.Done():
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
