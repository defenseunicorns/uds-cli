// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package operator

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	peprNamespace        = "pepr-system"
	flushInterval        = time.Second
	streamReconnectDelay = time.Second
)

// StreamKind identifies the operator events included in a monitor stream.
type StreamKind string

const (
	// StreamAll includes operator and policy events.
	StreamAll StreamKind = ""
	// StreamPolicies includes all policy decisions.
	StreamPolicies StreamKind = "policies"
	// StreamOperator includes UDS Core operator events.
	StreamOperator StreamKind = "operator"
	// StreamAllowed includes allowed policy decisions.
	StreamAllowed StreamKind = "allowed"
	// StreamDenied includes denied policy decisions.
	StreamDenied StreamKind = "denied"
	// StreamFailed includes denied policy decisions and operator failures.
	StreamFailed StreamKind = "failed"
	// StreamMutated includes policy mutations.
	StreamMutated StreamKind = "mutated"
)

// MonitorOptions configures operator monitoring.
type MonitorOptions struct {
	Stream     StreamKind
	Namespace  string
	Follow     bool
	Timestamps bool
	Since      time.Duration
	JSON       bool
	NoColor    bool
	LogLevel   string

	source podLogSource
}

type logEntry struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Res       struct {
		Allowed bool `json:"allowed"`
		Status  struct {
			Message string `json:"message"`
		} `json:"status"`
		Patch     *string `json:"patch,omitempty"`
		PatchType *string `json:"patchType,omitempty"`
	} `json:"res"`
	Msg      string `json:"msg"`
	Kind     string `json:"kind"`
	Metadata *struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata,omitempty"`
}

type logProcessor struct {
	formatter  Formatter
	stream     StreamKind
	namespace  string
	json       bool
	timestamps bool
	streams    iostreams.IOStreams

	mu       sync.Mutex
	wrote    bool
	pending  *Event
	repeated int
}

type podContainer struct {
	pod       string
	container string
}

type podLogSource interface {
	ListPods(context.Context) ([]corev1.Pod, error)
	StreamLogs(context.Context, string, *corev1.PodLogOptions) (io.ReadCloser, error)
}

type kubernetesPodLogSource struct {
	client kubernetes.Interface
}

type streamResult struct {
	err    error
	fatal  bool
	opened bool
}

func (s kubernetesPodLogSource) ListPods(ctx context.Context) ([]corev1.Pod, error) {
	pods, err := s.client.CoreV1().Pods(peprNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func (s kubernetesPodLogSource) StreamLogs(ctx context.Context, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	stream, err := s.client.CoreV1().Pods(peprNamespace).GetLogs(pod, opts).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("open log stream: %w", err)
	}
	return stream, nil
}

// Monitor streams UDS Core operator and policy events from Pepr controller pods.
func Monitor(ctx context.Context, streams iostreams.IOStreams, opts MonitorOptions) error {
	streams = logger.Bind(streams, opts.LogLevel)
	source := opts.source
	if source == nil {
		config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		).ClientConfig()
		if err != nil {
			return fmt.Errorf("connect to cluster: %w", err)
		}
		client, err := kubernetes.NewForConfig(config)
		if err != nil {
			return fmt.Errorf("create Kubernetes client: %w", err)
		}
		source = kubernetesPodLogSource{client: client}
	}

	pods, err := source.ListPods(ctx)
	if err != nil {
		return fmt.Errorf("list Pepr pods: %w", err)
	}
	targets := selectContainers(pods, opts.Stream)
	processor := &logProcessor{
		formatter:  Formatter{NoColor: opts.NoColor},
		stream:     opts.Stream,
		namespace:  strings.ToLower(opts.Namespace),
		json:       opts.JSON,
		timestamps: opts.Timestamps,
		streams:    streams,
	}

	if len(targets) == 0 {
		return ErrNoTargetsFound
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan streamResult, len(targets)+2)
	started := make(chan streamResult, len(targets))
	var workers sync.WaitGroup
	for _, target := range targets {
		workers.Go(func() {
			logOpts := &corev1.PodLogOptions{
				Container:  target.container,
				Follow:     opts.Follow,
				Timestamps: opts.Timestamps,
			}
			if opts.Since > 0 {
				seconds := int64(math.Ceil(opts.Since.Seconds()))
				logOpts.SinceSeconds = &seconds
			}

			opened := false
			warnedEnded := false
			for {
				logStream, err := source.StreamLogs(ctx, target.pod, logOpts)
				if err != nil {
					streamErr := fmt.Errorf("stream logs for pod %s: %w", target.pod, err)
					if !opened {
						started <- streamResult{err: streamErr}
						return
					}
					if ctx.Err() != nil {
						results <- streamResult{opened: true}
						return
					}
					streams.Warn("unable to reconnect Pepr pod logs", "error", streamErr)
					if !waitForReconnect(ctx) {
						results <- streamResult{opened: true}
						return
					}
					continue
				}
				if !opened {
					started <- streamResult{opened: true}
					opened = true
				}

				processErr := processor.process(streams.Out(), logStream)
				_ = logStream.Close()
				if processErr != nil {
					results <- streamResult{opened: true, fatal: true, err: fmt.Errorf("process logs for pod %s: %w", target.pod, processErr)}
					cancel()
					return
				}
				if !opts.Follow || ctx.Err() != nil {
					results <- streamResult{opened: true}
					return
				}
				if !warnedEnded {
					streams.Warn("Pepr pod log stream ended; reconnecting", "pod", target.pod)
					warnedEnded = true
				}
				if !waitForReconnect(ctx) {
					results <- streamResult{opened: true}
					return
				}
			}
		})
	}

	stopFlush := make(chan struct{})
	var flushWorker sync.WaitGroup
	if opts.Follow && !opts.JSON {
		flushWorker.Go(func() {
			ticker := time.NewTicker(flushInterval)
			defer ticker.Stop()
			for {
				select {
				case <-stopFlush:
					return
				case <-ticker.C:
					if err := processor.flushRepeated(streams.Out()); err != nil {
						results <- streamResult{fatal: true, err: fmt.Errorf("flush monitor output: %w", err)}
						cancel()
						return
					}
				}
			}
		})
	}

	var streamErrors []error
	opened := 0
	for range targets {
		result := <-started
		if result.opened {
			if opened == 0 {
				for _, err := range streamErrors {
					streams.Warn("unable to stream Pepr pod logs", "error", err)
				}
				streamErrors = nil
			}
			opened++
		} else if opened > 0 {
			streams.Warn("unable to stream Pepr pod logs", "error", result.err)
		} else {
			streamErrors = append(streamErrors, result.err)
		}
	}
	if opened == 0 && len(targets) > 0 {
		close(stopFlush)
		flushWorker.Wait()
		workers.Wait()
		return errors.Join(streamErrors...)
	}

	workers.Wait()
	close(stopFlush)
	flushWorker.Wait()
	if err := processor.flush(streams.Out()); err != nil {
		results <- streamResult{fatal: true, err: fmt.Errorf("flush monitor output: %w", err)}
	}
	close(results)
	return monitorResult(results)
}

func monitorResult(results <-chan streamResult) error {
	var fatalErrors []error
	for result := range results {
		if result.fatal && result.err != nil {
			fatalErrors = append(fatalErrors, result.err)
		}
	}
	return errors.Join(fatalErrors...)
}

func waitForReconnect(ctx context.Context) bool {
	timer := time.NewTimer(streamReconnectDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func selectContainers(pods []corev1.Pod, stream StreamKind) []podContainer {
	includeAdmission := stream != StreamOperator
	includeWatcher := stream == StreamAll || stream == StreamOperator || stream == StreamFailed
	targets := make([]podContainer, 0, len(pods))
	for _, pod := range pods {
		switch pod.Labels["pepr.dev/controller"] {
		case "admission":
			if includeAdmission {
				targets = append(targets, podContainer{pod: pod.Name, container: "server"})
			}
		case "watcher":
			if includeWatcher {
				targets = append(targets, podContainer{pod: pod.Name, container: "watcher"})
			}
		}
	}
	return targets
}

func (p *logProcessor) process(w io.Writer, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 5*1024*1024)
	for scanner.Scan() {
		if err := p.processLine(w, scanner.Text()); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (p *logProcessor) processLine(w io.Writer, line string) error {
	timestamp, payload := splitLogLine(line, p.timestamps)
	var entry logEntry
	if err := json.Unmarshal([]byte(payload), &entry); err != nil {
		if isRelevantLog(payload) {
			p.warn("parse Pepr event", err)
		}
		return nil
	}

	event, ok, err := classifyEvent(entry, p.stream)
	if err != nil {
		p.warn("parse Pepr event", err)
		return nil
	}
	if !ok || p.skipResource(entry) {
		return nil
	}
	event.Timestamp = timestamp

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.json {
		if p.timestamps && timestamp != "" {
			payload = strings.TrimSuffix(payload, "}") + fmt.Sprintf(",\"ts\":%q}", timestamp)
		}
		_, err := fmt.Fprintln(w, payload)
		return err
	}
	if p.pending != nil && eventsEqual(*p.pending, event) {
		p.repeated++
		return nil
	}
	if err := p.flushLocked(w); err != nil {
		return err
	}
	if p.wrote {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	if err := p.formatter.writeEventOpen(w, event); err != nil {
		return err
	}
	p.pending = &event
	p.wrote = true
	return nil
}

func (p *logProcessor) flush(w io.Writer) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.flushLocked(w)
}

func (p *logProcessor) flushRepeated(w io.Writer) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.repeated == 0 {
		return nil
	}
	return p.flushLocked(w)
}

func (p *logProcessor) flushLocked(w io.Writer) error {
	if p.pending == nil {
		return nil
	}
	if p.repeated > 0 {
		separator := " "
		if p.pending.Message != "" || len(p.pending.Patch) > 0 {
			separator = "\n  "
		}
		if _, err := fmt.Fprintf(w, "%srepeated=%d", separator, p.repeated); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	p.pending = nil
	p.repeated = 0
	return nil
}

func (p *logProcessor) warn(message string, err error) {
	p.streams.Warn(message, "error", err)
}

func eventsEqual(a, b Event) bool {
	return a.Kind == b.Kind && a.Resource == b.Resource && a.Message == b.Message && slices.Equal(a.Patch, b.Patch)
}

func isRelevantLog(payload string) bool {
	return strings.Contains(payload, "Check response") ||
		strings.Contains(payload, "Processing") ||
		strings.Contains(payload, "Updating status") ||
		strings.Contains(payload, "Writing event:")
}

func splitLogLine(line string, timestamps bool) (string, string) {
	if !timestamps {
		return "", line
	}
	parts := strings.SplitN(line, " ", 2)
	if len(parts) != 2 {
		return "", line
	}
	return parts[0], parts[1]
}

func (p *logProcessor) skipResource(entry logEntry) bool {
	if p.namespace == "" {
		return false
	}
	namespace := entry.Namespace
	if entry.Metadata != nil && entry.Metadata.Namespace != "" {
		namespace = entry.Metadata.Namespace
	}
	return !strings.EqualFold(namespace, p.namespace)
}

func classifyEvent(entry logEntry, stream StreamKind) (Event, bool, error) {
	isAdmission := entry.Msg == "Check response"
	isProcessing := entry.Kind == "Package" && strings.HasPrefix(entry.Msg, "Processing")
	isStatus := strings.HasPrefix(entry.Msg, "Updating status")
	isOperatorFailure := strings.HasPrefix(entry.Msg, "Writing event:") || (isStatus && strings.Contains(entry.Msg, "Failed"))

	if isProcessing || isStatus || strings.HasPrefix(entry.Msg, "Writing event:") {
		if stream != StreamAll && stream != StreamOperator && (stream != StreamFailed || !isOperatorFailure) {
			return Event{}, false, nil
		}
		resource := resourceName(entry.Namespace, entry.Name)
		if entry.Metadata != nil {
			resource = resourceName(entry.Metadata.Namespace, entry.Metadata.Name)
		}
		return Event{Kind: EventOperator, Resource: resource, Message: entry.Msg}, true, nil
	}
	if !isAdmission {
		return Event{}, false, nil
	}

	resource := resourceName(entry.Namespace, entry.Name)
	if entry.Res.PatchType != nil {
		if stream != StreamAll && stream != StreamPolicies && stream != StreamMutated {
			return Event{}, false, nil
		}
		patch, err := decodePatch(entry.Res.Patch)
		if err != nil {
			return Event{}, false, err
		}
		return Event{Kind: EventMutated, Resource: resource, Patch: patch}, true, nil
	}
	if entry.Res.Allowed {
		if stream != StreamAll && stream != StreamPolicies && stream != StreamAllowed {
			return Event{}, false, nil
		}
		return Event{Kind: EventAllowed, Resource: resource}, true, nil
	}
	if stream != StreamAll && stream != StreamPolicies && stream != StreamDenied && stream != StreamFailed {
		return Event{}, false, nil
	}
	return Event{Kind: EventDenied, Resource: resource, Message: entry.Res.Status.Message}, true, nil
}

func resourceName(namespace, name string) string {
	name = strings.TrimPrefix(name, "/")
	if namespace == "" {
		return name
	}
	if name == "" {
		return namespace
	}
	return namespace + "/" + name
}

func decodePatch(encoded *string) ([]PatchOperation, error) {
	if encoded == nil {
		return nil, errors.New("mutation patch is missing")
	}
	decoded, err := base64.StdEncoding.DecodeString(*encoded)
	if err != nil {
		return nil, fmt.Errorf("decode mutation patch: %w", err)
	}
	var operations []struct {
		Kind  string          `json:"op"`
		Path  string          `json:"path"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(decoded, &operations); err != nil {
		return nil, fmt.Errorf("parse mutation patch: %w", err)
	}
	patches := make([]PatchOperation, 0, len(operations))
	for _, operation := range operations {
		patches = append(patches, PatchOperation{
			Kind:  patchKind(operation.Kind),
			Path:  operation.Path,
			Value: string(operation.Value),
		})
	}
	return patches, nil
}

func patchKind(kind string) string {
	switch strings.ToLower(kind) {
	case "add":
		return "ADDED"
	case "remove":
		return "REMOVED"
	case "replace":
		return "REPLACED"
	default:
		return strings.ToUpper(kind)
	}
}
