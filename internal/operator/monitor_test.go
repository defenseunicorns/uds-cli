// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package operator

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/internal/logger"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type fakePodLogSource struct {
	pods    []corev1.Pod
	logs    map[string]string
	errors  map[string]error
	options map[string]corev1.PodLogOptions
	mu      sync.Mutex
}

func (f *fakePodLogSource) ListPods(context.Context) ([]corev1.Pod, error) {
	return f.pods, nil
}

func (f *fakePodLogSource) StreamLogs(_ context.Context, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	f.options[pod] = *opts
	f.mu.Unlock()
	if err := f.errors[pod]; err != nil {
		return nil, err
	}
	logs := f.logs[pod]
	if opts.Timestamps {
		lines := strings.Split(logs, "\n")
		for i := range lines {
			lines[i] = "2026-08-31T12:00:00Z " + lines[i]
		}
		logs = strings.Join(lines, "\n")
	}
	return io.NopCloser(bytes.NewBufferString(logs)), nil
}

type partialFollowSource struct{}

func (partialFollowSource) ListPods(context.Context) ([]corev1.Pod, error) {
	return testPods(), nil
}

func (partialFollowSource) StreamLogs(ctx context.Context, pod string, _ *corev1.PodLogOptions) (io.ReadCloser, error) {
	if pod == "watcher" {
		return nil, errors.New("forbidden")
	}
	return blockingLogStream(ctx, ""), nil
}

type recoveringFollowSource struct {
	mu             sync.Mutex
	admissionCalls int
	options        []corev1.PodLogOptions
}

func (s *recoveringFollowSource) ListPods(context.Context) ([]corev1.Pod, error) {
	return testPods(), nil
}

func (s *recoveringFollowSource) StreamLogs(ctx context.Context, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	if pod == "watcher" {
		return blockingLogStream(ctx, ""), nil
	}

	s.mu.Lock()
	s.admissionCalls++
	calls := s.admissionCalls
	s.options = append(s.options, *opts)
	s.mu.Unlock()
	if calls == 1 {
		return io.NopCloser(strings.NewReader("2026-08-31T12:00:00Z " + allowedLog("tenant"))), nil
	}
	logs := "2026-08-31T12:00:01Z " + deniedLog("tenant")
	if opts.SinceTime == nil {
		logs = "2026-08-31T12:00:00Z " + allowedLog("tenant") + "\n" + logs
	}
	return blockingLogStream(ctx, logs), nil
}

func (s *recoveringFollowSource) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.admissionCalls
}

func (s *recoveringFollowSource) streamOptions() []corev1.PodLogOptions {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]corev1.PodLogOptions(nil), s.options...)
}

// replacingFollowSource recreates a pod under the same name with a new UID while the original stream is open.
// This models StatefulSet-style replacement and verifies that UID, not name, defines worker identity.
type replacingFollowSource struct {
	mu          sync.Mutex
	replaced    bool
	newOpened   bool
	streamCalls int
}

func (s *replacingFollowSource) ListPods(context.Context) ([]corev1.Pod, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	uid := "old-admission"
	if s.replaced {
		uid = "new-admission"
	}
	return []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{
		Name:   "admission",
		UID:    types.UID(uid),
		Labels: map[string]string{"pepr.dev/controller": "admission"},
	}}}, nil
}

func (s *replacingFollowSource) StreamLogs(ctx context.Context, _ string, _ *corev1.PodLogOptions) (io.ReadCloser, error) {
	s.mu.Lock()
	s.streamCalls++
	call := s.streamCalls
	if call == 1 {
		s.replaced = true
	} else {
		s.newOpened = true
	}
	s.mu.Unlock()
	if call == 1 {
		return blockingLogStream(ctx, "2026-08-31T12:00:00Z "+allowedLog("tenant")), nil
	}
	return blockingLogStream(ctx, "2026-08-31T12:00:01Z "+deniedLog("tenant")), nil
}

func (s *replacingFollowSource) replacementOpened() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.newOpened
}

// cancelOnFatalSource keeps one stream blocked so the test can prove a sibling output failure cancels it.
type cancelOnFatalSource struct {
	canceled chan struct{}
}

func (s cancelOnFatalSource) ListPods(context.Context) ([]corev1.Pod, error) {
	return testPods()[:2], nil
}

func (s cancelOnFatalSource) StreamLogs(ctx context.Context, pod string, _ *corev1.PodLogOptions) (io.ReadCloser, error) {
	if pod == "admission" {
		return io.NopCloser(strings.NewReader(allowedLog("tenant"))), nil
	}
	reader, writer := io.Pipe()
	go func() {
		<-ctx.Done()
		close(s.canceled)
		_ = writer.Close()
	}()
	return reader, nil
}

// flappingFollowSource briefly removes a target and delays worker shutdown. The overlap exposes stale-result
// races and verifies that the next generation resumes from the retiring worker's cursor.
type flappingFollowSource struct {
	mu              sync.Mutex
	listCalls       int
	streamCalls     int
	secondSinceTime *metav1.Time
}

func (s *flappingFollowSource) ListPods(context.Context) ([]corev1.Pod, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls++
	if s.listCalls == 2 {
		return nil, nil
	}
	return testPods()[:1], nil
}

func (s *flappingFollowSource) StreamLogs(ctx context.Context, _ string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	s.mu.Lock()
	s.streamCalls++
	call := s.streamCalls
	hasSinceTime := opts.SinceTime != nil
	if call > 1 && hasSinceTime {
		sinceTime := *opts.SinceTime
		s.secondSinceTime = &sinceTime
	}
	s.mu.Unlock()
	if call > 1 {
		logs := "2026-08-31T12:00:01Z " + deniedLog("tenant")
		if !hasSinceTime {
			logs = "2026-08-31T12:00:00Z " + allowedLog("tenant") + "\n" + logs
		}
		return blockingLogStream(ctx, logs), nil
	}
	reader, writer := io.Pipe()
	go func() {
		_, _ = io.WriteString(writer, "2026-08-31T12:00:00Z "+allowedLog("tenant")+"\n")
		<-ctx.Done()
		time.Sleep(30 * time.Millisecond)
		_ = writer.Close()
	}()
	return reader, nil
}

func (s *flappingFollowSource) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streamCalls
}

func (s *flappingFollowSource) sinceTime() *metav1.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.secondSinceTime.DeepCopy()
}

func blockingLogStream(ctx context.Context, logs string) io.ReadCloser {
	reader, writer := io.Pipe()
	go func() {
		if logs != "" {
			_, _ = io.WriteString(writer, logs+"\n")
		}
		<-ctx.Done()
		_ = writer.Close()
	}()
	return reader
}

// lockedBuffer supports assertions while follow workers are still writing concurrently.
type lockedBuffer struct {
	bytes.Buffer
	mu sync.Mutex
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

// errorWriter deterministically exercises fatal output paths without depending on OS pipe behavior.
type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestSelectContainers(t *testing.T) {
	pods := testPods()

	assert.Equal(t, []podContainer{{pod: "admission", container: "server"}, {pod: "watcher", container: "watcher"}}, selectContainers(pods, StreamAll))
	assert.Equal(t, []podContainer{{pod: "admission", container: "server"}}, selectContainers(pods, StreamPolicies))
	assert.Equal(t, []podContainer{{pod: "watcher", container: "watcher"}}, selectContainers(pods, StreamOperator))
}

func TestPruneInactiveCursors(t *testing.T) {
	obsolete := podContainer{pod: "old", container: "server", uid: "old"}
	desiredTarget := podContainer{pod: "desired", container: "server", uid: "desired"}
	activeTarget := podContainer{pod: "active", container: "server", uid: "active"}
	retiringTarget := podContainer{pod: "retiring", container: "server", uid: "retiring"}
	cursor := time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)
	cursors := map[podContainer]time.Time{
		obsolete:       cursor,
		desiredTarget:  cursor,
		activeTarget:   cursor,
		retiringTarget: cursor,
	}

	pruneInactiveCursors(
		cursors,
		map[podContainer]struct{}{desiredTarget: {}},
		map[podContainer]followWorker{activeTarget: {}},
		map[podContainer]struct{}{retiringTarget: {}},
	)

	assert.NotContains(t, cursors, obsolete)
	assert.Contains(t, cursors, desiredTarget)
	assert.Contains(t, cursors, activeTarget)
	assert.Contains(t, cursors, retiringTarget)
}

func TestMonitor(t *testing.T) {
	source := &fakePodLogSource{
		pods:    testPods(),
		logs:    map[string]string{"admission": allowedLog("tenant"), "watcher": operatorLog("tenant")},
		errors:  map[string]error{},
		options: map[string]corev1.PodLogOptions{},
	}
	streams, _, out, _ := iostreams.NewTestIOStreams()

	err := Monitor(context.Background(), streams, MonitorOptions{
		source:     source,
		Timestamps: true,
		Since:      1500 * time.Millisecond,
		NoColor:    true,
	})

	require.NoError(t, err)
	assert.Contains(t, out.String(), "ALLOWED")
	assert.Contains(t, out.String(), "OPERATOR")
	for _, opts := range source.options {
		assert.False(t, opts.Follow)
		assert.True(t, opts.Timestamps)
		require.NotNil(t, opts.SinceSeconds)
		assert.Equal(t, int64(2), *opts.SinceSeconds)
	}
	assert.Equal(t, "server", source.options["admission"].Container)
	assert.Equal(t, "watcher", source.options["watcher"].Container)
}

func TestMonitorErrors(t *testing.T) {
	streamErr := errors.New("forbidden")
	tests := []struct {
		name        string
		source      *fakePodLogSource
		out         io.Writer
		wantErr     string
		wantTarget  error
		wantWarning string
	}{
		{
			name: "all streams fail",
			source: &fakePodLogSource{
				pods: testPods(), errors: map[string]error{"admission": streamErr, "watcher": streamErr}, options: map[string]corev1.PodLogOptions{},
			},
			wantErr:    "forbidden",
			wantTarget: ErrStreamPodLogs,
		},
		{
			name: "partial stream failure warns",
			source: &fakePodLogSource{
				pods: testPods(), logs: map[string]string{"admission": allowedLog("tenant")}, errors: map[string]error{"watcher": streamErr}, options: map[string]corev1.PodLogOptions{},
			},
			wantWarning: "streaming logs for pod watcher: forbidden",
		},
		{
			name: "output failure",
			source: &fakePodLogSource{
				pods: testPods()[:1], logs: map[string]string{"admission": allowedLog("tenant")}, errors: map[string]error{}, options: map[string]corev1.PodLogOptions{},
			},
			out:        errorWriter{err: errors.New("broken pipe")},
			wantErr:    "broken pipe",
			wantTarget: ErrProcessPodLogs,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if tt.out == nil {
				tt.out = &out
			}
			var errOut bytes.Buffer
			streams := iostreams.New(nil, tt.out, &errOut)
			err := Monitor(context.Background(), streams, MonitorOptions{source: tt.source, NoColor: true})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.ErrorIs(t, err, tt.wantTarget)
			} else {
				require.NoError(t, err)
			}
			require.Contains(t, errOut.String(), tt.wantWarning)
		})
	}
}

func TestMonitorCancelsSiblingStreamsOnFatalError(t *testing.T) {
	source := cancelOnFatalSource{canceled: make(chan struct{})}
	streams := iostreams.New(nil, errorWriter{err: errors.New("broken pipe")}, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Monitor(ctx, streams, MonitorOptions{source: source, NoColor: true})
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("monitor did not return after a fatal stream error")
	}
	cancel()
	require.ErrorIs(t, err, ErrProcessPodLogs)
	select {
	case <-source.canceled:
	case <-time.After(time.Second):
		t.Fatal("sibling stream was not canceled")
	}
}

func TestMonitorFollowReturnsWhenAllStreamsFailToOpen(t *testing.T) {
	source := &fakePodLogSource{
		pods:    testPods()[:1],
		errors:  map[string]error{"admission": errors.New("forbidden")},
		options: map[string]corev1.PodLogOptions{},
	}
	streams := iostreams.New(nil, io.Discard, io.Discard)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := Monitor(ctx, streams, MonitorOptions{source: source, Follow: true})

	require.ErrorIs(t, err, ErrStreamPodLogs)
}

func TestMonitorFollowWarnsWhenOneStreamFails(t *testing.T) {
	var errOut lockedBuffer
	streams := iostreams.New(nil, io.Discard, &errOut)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Monitor(ctx, streams, MonitorOptions{source: partialFollowSource{}, Follow: true})
	}()

	require.Eventually(t, func() bool {
		return strings.Contains(errOut.String(), "streaming logs for pod watcher: forbidden")
	}, time.Second, 10*time.Millisecond)
	cancel()
	require.NoError(t, <-done)
}

func TestMonitorFollowReconnectsEndedStream(t *testing.T) {
	source := &recoveringFollowSource{}
	var out lockedBuffer
	streams := iostreams.New(nil, &out, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Monitor(ctx, streams, MonitorOptions{source: source, Follow: true, NoColor: true})
	}()

	require.Eventually(t, func() bool {
		return source.calls() >= 2 && strings.Contains(out.String(), "DENIED")
	}, 3*time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, strings.Count(out.String(), "ALLOWED"))
	assert.NotContains(t, out.String(), "repeated=")
	assert.NotContains(t, out.String(), "2026-08-31")
	cancel()
	require.NoError(t, <-done)

	options := source.streamOptions()
	require.Len(t, options, 2)
	assert.True(t, options[0].Timestamps)
	require.NotNil(t, options[1].SinceTime)
	assert.Equal(t, "2026-08-31T12:00:00Z", options[1].SinceTime.Format(time.RFC3339))
}

func TestMonitorFollowIgnoresStaleWorkerResults(t *testing.T) {
	source := &flappingFollowSource{}
	var out lockedBuffer
	streams := iostreams.New(nil, &out, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Monitor(ctx, streams, MonitorOptions{
			source:          source,
			Follow:          true,
			refreshInterval: 10 * time.Millisecond,
		})
	}()

	require.Eventually(t, func() bool {
		return source.calls() == 2 && strings.Contains(out.String(), "DENIED")
	}, time.Second, 10*time.Millisecond)
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, 2, source.calls())
	sinceTime := source.sinceTime()
	require.NotNil(t, sinceTime)
	assert.Equal(t, "2026-08-31T12:00:00Z", sinceTime.Format(time.RFC3339))
	assert.Equal(t, 1, strings.Count(out.String(), "ALLOWED"))
	cancel()
	require.NoError(t, <-done)
}

func TestMonitorFollowDiscoversReplacementPod(t *testing.T) {
	source := &replacingFollowSource{}
	var out lockedBuffer
	streams := iostreams.New(nil, &out, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Monitor(ctx, streams, MonitorOptions{source: source, Follow: true, NoColor: true})
	}()

	require.Eventually(t, func() bool {
		return source.replacementOpened() && strings.Contains(out.String(), "DENIED")
	}, 3*time.Second, 10*time.Millisecond)
	assert.Contains(t, out.String(), "ALLOWED")
	cancel()
	require.NoError(t, <-done)
}

func TestLogProcessor(t *testing.T) {
	patch := base64.StdEncoding.EncodeToString([]byte(`[{"op":"add","path":"/spec/securityContext/runAsNonRoot","value":true}]`))
	logs := fmt.Sprintf(`{"namespace":"tenant","name":"/pod","res":{"allowed":true},"msg":"Check response"}
{"namespace":"other","name":"/pod","res":{"allowed":false,"status":{"message":"denied"}},"msg":"Check response"}
{"namespace":"tenant","name":"/pod","res":{"allowed":true,"patchType":"JSONPatch","patch":%q},"msg":"Check response"}`, patch)
	processor := logProcessor{formatter: Formatter{NoColor: true}, stream: StreamPolicies, namespace: "tenant"}
	var out bytes.Buffer

	_, err := processor.processStream(&out, bytes.NewBufferString(logs), false)
	require.NoError(t, err)
	require.NoError(t, processor.flush(&out))
	assert.Contains(t, out.String(), "ALLOWED  resource=tenant/pod")
	assert.Contains(t, out.String(), "MUTATED  resource=tenant/pod")
	assert.Contains(t, out.String(), "ADDED path=/spec/securityContext/runAsNonRoot value=true")
	assert.NotContains(t, out.String(), "DENIED")
}

func TestLogProcessorCollapsesRepeatedEvents(t *testing.T) {
	processor := logProcessor{formatter: Formatter{NoColor: true}, stream: StreamAllowed}
	var out bytes.Buffer
	logs := allowedLog("tenant") + "\n" + allowedLog("tenant") + "\n" + allowedLog("tenant")

	_, err := processor.processStream(&out, bytes.NewBufferString(logs), false)
	require.NoError(t, err)
	require.NoError(t, processor.flush(&out))
	assert.Equal(t, 1, bytes.Count(out.Bytes(), []byte("ALLOWED")))
	assert.Contains(t, out.String(), "repeated=2")
}

func TestLogProcessorRepeatedEventStaysSingleAcrossFlushes(t *testing.T) {
	processor := logProcessor{formatter: Formatter{NoColor: true}, stream: StreamAllowed}
	var out bytes.Buffer

	require.NoError(t, processor.processPayload(&out, "", allowedLog("tenant")))
	assert.Equal(t, 1, bytes.Count(out.Bytes(), []byte("ALLOWED")))
	require.NoError(t, processor.flushRepeated(&out))

	require.NoError(t, processor.processPayload(&out, "", allowedLog("tenant")))
	assert.Equal(t, 1, bytes.Count(out.Bytes(), []byte("ALLOWED")))
	require.NoError(t, processor.flushRepeated(&out))
	assert.Equal(t, 1, bytes.Count(out.Bytes(), []byte("ALLOWED")))
	assert.Contains(t, out.String(), "repeated=1")
}

func TestLogProcessorShowsFirstEventDuringContinuousRepeats(t *testing.T) {
	processor := logProcessor{formatter: Formatter{NoColor: true}, stream: StreamDenied}
	var out bytes.Buffer
	line := `{"namespace":"tenant","name":"/pod","res":{"allowed":false},"msg":"Check response"}`

	require.NoError(t, processor.processPayload(&out, "", line))
	for range 10 {
		require.NoError(t, processor.processPayload(&out, "", line))
	}
	assert.Contains(t, out.String(), "DENIED")
	assert.Equal(t, 1, bytes.Count(out.Bytes(), []byte("DENIED")))

	require.NoError(t, processor.flushRepeated(&out))
	assert.Contains(t, out.String(), "repeated=10")
}

func TestLogProcessorWarnsForMalformedEvents(t *testing.T) {
	var errOut bytes.Buffer
	streams := logger.Bind(iostreams.New(nil, nil, &errOut), "info")
	processor := logProcessor{stream: StreamAllowed, streams: streams}

	require.NoError(t, processor.processPayload(io.Discard, "", `{"msg":"Check response"`))
	assert.Contains(t, errOut.String(), "parse Pepr event")
}

func TestLogProcessor_JSONTimestamp(t *testing.T) {
	processor := logProcessor{stream: StreamAllowed, json: true, timestamps: true}
	var out bytes.Buffer
	line := `2026-08-31T12:00:00Z {"namespace":"tenant","name":"/pod","res":{"allowed":true},"msg":"Check response"}`

	timestamp, payload := splitLogLine(line, true)
	require.NoError(t, processor.processPayload(&out, timestamp, payload))
	assert.JSONEq(t, `{"namespace":"tenant","name":"/pod","res":{"allowed":true},"msg":"Check response","ts":"2026-08-31T12:00:00Z"}`, out.String())
}

func TestClassifyEvent_Failed(t *testing.T) {
	entry := logEntry{Namespace: "tenant", Name: "package", Msg: "Updating status to Failed"}
	event, ok, err := classifyEvent(entry, StreamFailed)

	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, EventOperator, event.Kind)
	assert.Equal(t, "tenant/package", event.Resource)
}

func testPods() []corev1.Pod {
	return []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "admission", Labels: map[string]string{"pepr.dev/controller": "admission"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "watcher", Labels: map[string]string{"pepr.dev/controller": "watcher"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "other", Labels: map[string]string{"app": "other"}}},
	}
}

func allowedLog(namespace string) string {
	return fmt.Sprintf(`{"namespace":%q,"name":"/pod","res":{"allowed":true},"msg":"Check response"}`, namespace)
}

func deniedLog(namespace string) string {
	return fmt.Sprintf(`{"namespace":%q,"name":"/pod","res":{"allowed":false},"msg":"Check response"}`, namespace)
}

func operatorLog(namespace string) string {
	return fmt.Sprintf(`{"namespace":%q,"name":"package","msg":"Updating status to Ready"}`, namespace)
}
