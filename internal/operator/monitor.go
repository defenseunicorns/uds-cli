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
	"k8s.io/apimachinery/pkg/types"
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
	// refreshInterval is overridden by tests to exercise reconciliation without waiting on the production delay.
	refreshInterval time.Duration
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
	uid       types.UID
}

type podLogSource interface {
	ListPods(context.Context) ([]corev1.Pod, error)
	StreamLogs(context.Context, string, *corev1.PodLogOptions) (io.ReadCloser, error)
}

type kubernetesPodLogSource struct {
	client kubernetes.Interface
}

type targetResult struct {
	target        podContainer
	workerID      uint64
	lastTimestamp time.Time
	err           error
	fatal         bool
	opened        bool
	canceled      bool
}

type followWorker struct {
	id     uint64
	cancel context.CancelFunc
}

// targetMonitor holds the immutable dependencies shared by one-shot and follow workers.
type targetMonitor struct {
	streams   iostreams.IOStreams
	source    podLogSource
	processor *logProcessor
	opts      MonitorOptions
}

// followMonitor owns reconciliation state. Only its supervisor goroutine mutates the maps below;
// workers report state transitions through started and results.
type followMonitor struct {
	targetMonitor
	ctx    context.Context
	cancel context.CancelFunc

	results chan targetResult
	started chan targetResult

	active map[podContainer]followWorker
	// retiring prevents a replacement worker for the same pod UID from starting before its cursor is captured.
	retiring map[podContainer]struct{}
	cursors  map[podContainer]time.Time
	// desired is the latest successful Kubernetes pod-list snapshot.
	desired map[podContainer]struct{}
	workers sync.WaitGroup
	nextID  uint64
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
		return nil, fmt.Errorf("%w: %w", ErrOpenLogStream, err)
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
			return fmt.Errorf("%w: %w", ErrConnectCluster, err)
		}
		client, err := kubernetes.NewForConfig(config)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrCreateKubernetesClient, err)
		}
		source = kubernetesPodLogSource{client: client}
	}

	pods, err := source.ListPods(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrListPeprPods, err)
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
	runner := targetMonitor{streams: streams, source: source, processor: processor, opts: opts}
	if opts.Follow {
		return monitorFollowing(ctx, runner, targets)
	}
	return monitorOnce(ctx, runner, targets)
}

// monitorOnce makes one request per target. It tolerates individual stream-open failures when at least one
// target succeeds, but cancels sibling requests when output processing fails.
func monitorOnce(ctx context.Context, runner targetMonitor, targets []podContainer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan targetResult, len(targets))
	var workers sync.WaitGroup
	for _, target := range targets {
		workers.Go(func() {
			results <- runner.runTarget(ctx, target, time.Time{}, nil)
		})
	}

	var streamErrors []error
	var fatalErrors []error
	opened := 0
	for range targets {
		result := <-results
		if result.fatal {
			cancel()
		}
		if result.opened {
			opened++
		}
		if result.fatal {
			fatalErrors = append(fatalErrors, result.err)
		} else if result.err != nil {
			streamErrors = append(streamErrors, result.err)
		}
	}
	workers.Wait()
	close(results)

	if opened == 0 {
		return errors.Join(streamErrors...)
	}
	for _, err := range streamErrors {
		runner.streams.Warn("unable to stream Pepr pod logs", "error", err)
	}
	if err := runner.processor.flush(runner.streams.Out()); err != nil {
		fatalErrors = append(fatalErrors, fmt.Errorf("%w: %w", ErrFlushMonitorOutput, err))
	}
	return errors.Join(fatalErrors...)
}

// runTarget owns exactly one log-stream lifecycle. It reports the last observed timestamp but leaves retry
// and replacement decisions to the calling monitor mode.
func (m targetMonitor) runTarget(
	ctx context.Context,
	target podContainer,
	lastTimestamp time.Time,
	onStart func(targetResult),
) targetResult {
	result := targetResult{target: target}
	logOpts := &corev1.PodLogOptions{
		Container: target.container,
		Follow:    m.opts.Follow,
		// Follow mode always requests timestamps so reconnects can resume without exposing them unless requested.
		Timestamps: m.opts.Timestamps || m.opts.Follow,
	}
	// SinceSeconds applies only to the first connection. Subsequent connections use the last observed log timestamp.
	if lastTimestamp.IsZero() && m.opts.Since > 0 {
		seconds := int64(math.Ceil(m.opts.Since.Seconds()))
		logOpts.SinceSeconds = &seconds
	} else if !lastTimestamp.IsZero() {
		sinceTime := metav1.NewTime(lastTimestamp)
		logOpts.SinceTime = &sinceTime
	}

	logStream, err := m.source.StreamLogs(ctx, target.pod, logOpts)
	if err != nil {
		result.canceled = ctx.Err() != nil
		if !result.canceled {
			result.err = fmt.Errorf("%w %s: %w", ErrStreamPodLogs, target.pod, err)
		}
		if onStart != nil {
			onStart(result)
		}
		return result
	}
	result.opened = true
	if onStart != nil {
		onStart(result)
	}
	result.lastTimestamp, err = m.processor.processStream(m.streams.Out(), logStream, logOpts.Timestamps)
	_ = logStream.Close()
	if err != nil {
		result.canceled = ctx.Err() != nil
		if !result.canceled {
			result.err = fmt.Errorf("%w %s: %w", ErrProcessPodLogs, target.pod, err)
			result.fatal = true
		}
	}
	return result
}

// monitorFollowing supervises long-lived workers, periodically reconciling them against the current pod list.
// Healthy workers remain connected while removed or completed workers are replaced independently.
func monitorFollowing(ctx context.Context, runner targetMonitor, targets []podContainer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := &followMonitor{
		targetMonitor: runner,
		ctx:           ctx,
		cancel:        cancel,
		results:       make(chan targetResult, max(4, len(targets)*2)),
		started:       make(chan targetResult, len(targets)),
		active:        map[podContainer]followWorker{},
		retiring:      map[podContainer]struct{}{},
		cursors:       map[podContainer]time.Time{},
		desired:       make(map[podContainer]struct{}, len(targets)),
	}
	for _, target := range targets {
		m.desired[target] = struct{}{}
	}
	ready, err := m.startInitialWorkers(targets)
	if !ready {
		return err
	}

	refreshInterval := m.opts.refreshInterval
	if refreshInterval <= 0 {
		refreshInterval = streamReconnectDelay
	}
	refreshTicker := time.NewTicker(refreshInterval)
	defer refreshTicker.Stop()
	flushTicker := time.NewTicker(flushInterval)
	defer flushTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.stopWorkers()
			if err := m.processor.flush(m.streams.Out()); err != nil {
				return fmt.Errorf("%w: %w", ErrFlushMonitorOutput, err)
			}
			return nil
		case result := <-m.results:
			if err := m.handleResult(result); err != nil {
				return err
			}
		case <-refreshTicker.C:
			pods, err := m.source.ListPods(ctx)
			if err != nil {
				m.streams.Warn("unable to refresh Pepr pods", "error", fmt.Errorf("%w: %w", ErrListPeprPods, err))
				continue
			}
			m.reconcile(selectContainers(pods, m.opts.Stream))
		case <-flushTicker.C:
			if m.opts.JSON {
				continue
			}
			if err := m.processor.flushRepeated(m.streams.Out()); err != nil {
				cancel()
				m.stopWorkers()
				return fmt.Errorf("%w: %w", ErrFlushMonitorOutput, err)
			}
		}
	}
}

// startWorker launches a uniquely identified worker generation so delayed results cannot mutate newer state.
// reportStart is used only during initial readiness evaluation.
func (m *followMonitor) startWorker(target podContainer, reportStart bool) {
	workerCtx, workerCancel := context.WithCancel(m.ctx)
	m.nextID++
	workerID := m.nextID
	m.active[target] = followWorker{id: workerID, cancel: workerCancel}
	lastTimestamp := m.cursors[target]
	m.workers.Go(func() {
		var onStart func(targetResult)
		if reportStart {
			onStart = func(result targetResult) {
				select {
				case m.started <- result:
				case <-m.ctx.Done():
				}
			}
		}
		result := m.runTarget(workerCtx, target, lastTimestamp, onStart)
		result.workerID = workerID
		select {
		case m.results <- result:
		case <-m.ctx.Done():
		}
	})
}

// startInitialWorkers requires at least one stream to open before reconciliation begins. This prevents
// permanent startup failures, such as missing log permissions, from becoming an endless retry loop.
func (m *followMonitor) startInitialWorkers(targets []podContainer) (bool, error) {
	for _, target := range targets {
		m.startWorker(target, true)
	}
	var startErrors []error
	var earlyResults []targetResult
	opened := 0
	for pending := len(targets); pending > 0; {
		select {
		case <-m.ctx.Done():
			m.stopWorkers()
			return false, nil
		case result := <-m.started:
			pending--
			if result.opened {
				opened++
			} else if result.err != nil {
				startErrors = append(startErrors, result.err)
			}
		case result := <-m.results:
			// A worker can finish immediately after reporting that it opened. Drain these results during startup
			// so workers cannot block and fatal output errors are returned without waiting for every open attempt.
			if result.fatal {
				m.cancel()
				m.stopWorkers()
				return false, result.err
			}
			earlyResults = append(earlyResults, result)
		}
	}
	if opened == 0 {
		m.cancel()
		m.stopWorkers()
		return false, errors.Join(startErrors...)
	}
	for _, result := range earlyResults {
		m.results <- result
	}
	return true, nil
}

// reconcile treats a successful pod list as authoritative. Missing workers enter retirement until their
// cursors are captured, preventing overlapping generations and replay when a pod UID reappears.
func (m *followMonitor) reconcile(discovered []podContainer) {
	m.desired = make(map[podContainer]struct{}, len(discovered))
	for _, target := range discovered {
		m.desired[target] = struct{}{}
		if _, stopping := m.retiring[target]; stopping {
			continue
		}
		if _, running := m.active[target]; !running {
			m.startWorker(target, false)
		}
	}
	for target, worker := range m.active {
		if _, found := m.desired[target]; !found {
			worker.cancel()
			delete(m.active, target)
			m.retiring[target] = struct{}{}
		}
	}
	pruneInactiveCursors(m.cursors, m.desired, m.active, m.retiring)
}

// handleResult applies cursor progress before lifecycle changes so stale workers cannot unregister a newer
// generation while still preserving events they emitted before cancellation.
func (m *followMonitor) handleResult(result targetResult) error {
	if result.lastTimestamp.After(m.cursors[result.target]) {
		m.cursors[result.target] = result.lastTimestamp
	}
	worker, current := m.active[result.target]
	if !current || worker.id != result.workerID {
		delete(m.retiring, result.target)
		pruneInactiveCursors(m.cursors, m.desired, m.active, m.retiring)
		return nil
	}
	delete(m.active, result.target)
	pruneInactiveCursors(m.cursors, m.desired, m.active, m.retiring)
	if result.canceled {
		return nil
	}
	if result.fatal {
		m.cancel()
		m.stopWorkers()
		return result.err
	}
	if result.err != nil {
		m.streams.Warn("unable to stream Pepr pod logs", "error", result.err)
	} else {
		m.streams.Warn("Pepr pod log stream ended; waiting to reconnect", "pod", result.target.pod)
	}
	return nil
}

// stopWorkers cancels every tracked worker and waits until no worker can write further output.
func (m *followMonitor) stopWorkers() {
	for _, worker := range m.active {
		worker.cancel()
	}
	m.workers.Wait()
}

// pruneInactiveCursors bounds cursor state after pod churn while retaining cursors needed by active,
// desired, or not-yet-finished workers.
func pruneInactiveCursors(
	cursors map[podContainer]time.Time,
	desired map[podContainer]struct{},
	active map[podContainer]followWorker,
	retiring map[podContainer]struct{},
) {
	for target := range cursors {
		_, isDesired := desired[target]
		_, isActive := active[target]
		_, isRetiring := retiring[target]
		if !isDesired && !isActive && !isRetiring {
			delete(cursors, target)
		}
	}
}

// selectContainers maps Pepr controller labels to their fixed log-producing container names.
func selectContainers(pods []corev1.Pod, stream StreamKind) []podContainer {
	includeAdmission := stream != StreamOperator
	includeWatcher := stream == StreamAll || stream == StreamOperator || stream == StreamFailed
	targets := make([]podContainer, 0, len(pods))
	for _, pod := range pods {
		switch pod.Labels["pepr.dev/controller"] {
		case "admission":
			if includeAdmission {
				targets = append(targets, podContainer{pod: pod.Name, container: "server", uid: pod.UID})
			}
		case "watcher":
			if includeWatcher {
				targets = append(targets, podContainer{pod: pod.Name, container: "watcher", uid: pod.UID})
			}
		}
	}
	return targets
}

// processStream records the latest source timestamp independently from whether timestamps are rendered.
// Follow mode relies on the returned cursor to resume subsequent Kubernetes log requests.
func (p *logProcessor) processStream(w io.Writer, r io.Reader, inputTimestamps bool) (time.Time, error) {
	var lastTimestamp time.Time
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 5*1024*1024)
	for scanner.Scan() {
		timestamp, payload := splitLogLine(scanner.Text(), inputTimestamps)
		if timestamp != "" {
			parsed, err := time.Parse(time.RFC3339Nano, timestamp)
			if err == nil && parsed.After(lastTimestamp) {
				lastTimestamp = parsed
			}
		}
		if err := p.processPayload(w, timestamp, payload); err != nil {
			return lastTimestamp, err
		}
	}
	return lastTimestamp, scanner.Err()
}

// processPayload parses and filters one Pepr log payload, serializing writes and repeat detection shared by
// concurrent admission and watcher streams.
func (p *logProcessor) processPayload(w io.Writer, timestamp, payload string) error {
	var entry logEntry
	if err := json.Unmarshal([]byte(payload), &entry); err != nil {
		if isRelevantLog(payload) {
			p.streams.Warn("parse Pepr event", "error", err)
		}
		return nil
	}

	event, ok, err := classifyEvent(entry, p.stream)
	if err != nil {
		p.streams.Warn("parse Pepr event", "error", err)
		return nil
	}
	if !ok || p.skipResource(entry) {
		return nil
	}
	if p.timestamps {
		event.Timestamp = timestamp
	}

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
		return nil, ErrMutationPatchMissing
	}
	decoded, err := base64.StdEncoding.DecodeString(*encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDecodeMutationPatch, err)
	}
	var operations []struct {
		Kind  string          `json:"op"`
		Path  string          `json:"path"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(decoded, &operations); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParseMutationPatch, err)
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
