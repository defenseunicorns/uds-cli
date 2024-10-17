// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package pepr contains the logic monitoring Pepr operations in a UDS Cluster
package pepr

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/defenseunicorns/uds-cli/src/pkg/style"
	"github.com/zarf-dev/zarf/src/pkg/message"
	corev1 "k8s.io/api/core/v1"
)

// StreamKind represents the type of Pepr stream
type StreamKind string

type StreamReader struct {
	JSON            bool
	FilterStream    StreamKind
	filterNamespace string
	filterName      string
	indent          string
	lastEntryHeader string
	lastEntryBody   string
	repeatCount     int
	mutex           sync.Mutex
}

// LogEntry represents a log entry from Pepr
type LogEntry struct {
	Level     int    `json:"level"`
	Time      int64  `json:"time"`
	Pid       int    `json:"pid"`
	Hostname  string `json:"hostname"`
	Uid       string `json:"uid"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Res       struct {
		Uid     string `json:"uid"`
		Allowed bool   `json:"allowed"`
		Status  struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"status"`
		// Patch and PatchType are only present for mutations
		Patch     *string `json:"patch,omitempty"`
		PatchType *string `json:"patchType,omitempty"`
	} `json:"res"`
	Msg string `json:"msg"`
	// Metadata is only present for operator logs
	Metadata *struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
}

// operation represents a JSON Patch operation
type operation struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

const (
	// AnyStream represents all Pepr logs
	AnyStream StreamKind = ""
	// PolicyStream represents all Pepr admission controller logs
	PolicyStream StreamKind = "policies"
	// OperatorStream represents all UDS Operator logs
	OperatorStream StreamKind = "operator"
	// AllowStream represents all Pepr admission controller allow logs
	AllowStream StreamKind = "allowed"
	// DenyStream represents all Pepr admission controller deny logs
	DenyStream StreamKind = "denied"
	// MutateStream represents all Pepr admission controller mutation logs
	MutateStream StreamKind = "mutated"
	// FailureStream represents all admission controller deny logs and operator failure logs
	FailureStream StreamKind = "failed"
)

// NewStreamReader creates a new PeprStreamReader
func NewStreamReader(filterNamespace, filterName string) *StreamReader {
	return &StreamReader{
		filterNamespace: strings.ToLower(filterNamespace),
		filterName:      strings.ToLower(filterName),
	}
}

// PodFilter creates a map of pod and container names to pull logs from
func (p *StreamReader) PodFilter(pods []corev1.Pod) map[string]string {
	containers := make(map[string]string)

	// By default include the admission controller logs and exclude the operator logs
	includeAdmissionStream := true
	includeWatchStream := false

	switch p.FilterStream {
	// For OperatorStream, include only the operator logs
	case OperatorStream:
		includeAdmissionStream = false
		includeWatchStream = true

	// For FailureStream, include both the admission controller logs and operator logs
	case FailureStream:
		includeWatchStream = true

	// Include all logs for AllStream (empty string)
	case AnyStream:
		includeWatchStream = true
	}

	for _, pod := range pods {
		// Determine the container to stream logs from
		switch pod.Labels["pepr.dev/controller"] {
		case "admission":
			if includeAdmissionStream {
				containers[pod.Name] = "server"
			}

		case "watcher":
			if includeWatchStream {
				containers[pod.Name] = "watcher"
			}

		default:
			// Skip pods that are not part of the Pepr system
			continue
		}
	}

	return containers
}

// LogStream processes the log stream from Pepr and writes formatted output to the writer
func (p *StreamReader) LogStream(writer io.Writer, logStream io.ReadCloser, timestamp bool) error {
	// Use a longer indent when timestamps are enabled
	if timestamp {
		p.indent = strings.Repeat(" ", 32)
	}

	// Process logs line by line.
	scanner := bufio.NewScanner(logStream)
	buf := make([]byte, 0, 5*1024*1024) // Allocate a 5 MB buffer to handle large log lines
	scanner.Buffer(buf, cap(buf))       // Set the maximum token size

	var (
		enableLogAny             = p.FilterStream == AnyStream
		enableLogFailureAny      = p.FilterStream == FailureStream
		enableLogOperatorAny     = p.FilterStream == OperatorStream || enableLogAny
		enableLogOperatorFailure = p.FilterStream == FailureStream || enableLogOperatorAny
		enableLogAdmissionAny    = p.FilterStream == PolicyStream || enableLogAny
		enableLogMutate          = p.FilterStream == MutateStream || enableLogAdmissionAny
		enableLogAllow           = p.FilterStream == AllowStream || enableLogAdmissionAny
		enableLogDeny            = p.FilterStream == DenyStream || enableLogAdmissionAny || enableLogFailureAny
	)

	for scanner.Scan() {
		line := scanner.Text()

		isLogAdmission := strings.Contains(line, `"msg":"Check response"`)
		isLogOperatorProcessing := strings.Contains(line, `"kind":"Package"`) && strings.Contains(line, `"msg":"Processing`)
		isLogOperatorStatus := strings.Contains(line, `"msg":"Updating status`)
		isLogOperatorEvent := strings.Contains(line, `"msg":"Writing event:`)

		// Ignore any unmatched log lines
		if !isLogAdmission && !isLogOperatorProcessing && !isLogOperatorStatus && !isLogOperatorEvent {
			continue
		}

		var msgTimestamp, msgPayload string
		if timestamp {
			// Split the timestamp and payload
			split := strings.SplitN(line, " ", 2)
			if len(split) == 2 {
				msgTimestamp = split[0]
				msgPayload = split[1]
			} else {
				message.Warnf("Error splitting log line: %s", line)
				continue
			}
		} else {
			msgPayload = line
		}

		if p.JSON {
			// If timestamps are enabled, append the timestamp to the JSON payload
			// Replacing the last closing brace with a comma and the timestamp
			if timestamp {
				msgPayload = strings.TrimSuffix(msgPayload, "}")
				msgPayload = fmt.Sprintf("%s, \"ts\": \"%s\"}", msgPayload, msgTimestamp)
			}

			if _, err := writer.Write([]byte("\n" + msgPayload)); err != nil {
				message.WarnErr(err, "Error writing newline")
			}

			continue
		}

		// JSON parse the line
		var event LogEntry
		if err := json.Unmarshal([]byte(msgPayload), &event); err != nil {
			// Log the error and continue to the next line
			message.WarnErr(err, "Error parsing JSON")
			continue
		}

		// Filter by namespace if set
		if p.skipResource(event) {
			continue
		}

		// Process the JSON and generate formatted output
		name := fmt.Sprintf("%v%v", event.Namespace, event.Name)

		var header string
		var body string

		switch {
		// Handle operator processing
		case enableLogOperatorAny && isLogOperatorProcessing:
			name = fmt.Sprintf("%v/%v", event.Metadata.Namespace, event.Metadata.Name)
			header = style.RenderFmt(style.Purple, " ⚙️ OPERATOR  %s", name)
			body = style.RenderFmt(style.WarmGray, "\n%s             %v", p.indent, event.Msg)

		// Handle operator status updates
		case isLogOperatorStatus:
			failed := strings.Contains(event.Msg, "Failed")
			name = fmt.Sprintf("%v/%v", event.Namespace, event.Name)
			header = style.RenderFmt(style.Purple, " ⚙️ OPERATOR  %s", name)
			// Red if the status update is a failure
			if failed {
				body = style.RenderFmt(style.Red, "\n%s             %v", p.indent, event.Msg)
			} else {
				// Skip if operator events are not enabled for non-failures
				if !enableLogOperatorAny {
					continue
				}
				body = style.RenderFmt(style.WarmGray, "\n%s             %v", p.indent, event.Msg)
			}

		// Handle operator events
		case enableLogOperatorFailure && isLogOperatorEvent:
			name = fmt.Sprintf("%v/%v", event.Namespace, event.Name)
			header = style.RenderFmt(style.Purple, " ⚙️ OPERATOR  %s", name)
			// Red because events are failures
			body = "\n" + style.RenderFmt(style.Red, "%s             %v", p.indent, event.Msg)

		// Handle mutations
		case enableLogMutate && isLogAdmission && event.Res.PatchType != nil:
			header = style.RenderFmt(style.Cyan, " ✎ MUTATED   %s", name)
			body = p.renderMutation(event)

		// Handle validation success
		case enableLogAllow && isLogAdmission && event.Res.Allowed:
			header = style.RenderFmt(style.Green, " ✓ ALLOWED   %s", name)

		// Handle validation failure and override the formatting
		case enableLogDeny && isLogAdmission && !event.Res.Allowed:
			header = style.RenderFmt(style.Red, " ✗ DENIED    %s", name)
			body = p.renderDenied(event)

		default:
			// Unmatched log line (should not happen)
			continue
		}

		// Handle repeated events
		if p.lastEntryHeader == header && p.lastEntryBody == body {
			p.updateRepeatCount(p.repeatCount + 1)
		} else {
			p.writeRepeatedEvent(writer)
			p.updateLastEntry(header, body)

			// If timestamps are enabled, write the timestamp before the header
			if timestamp {
				_, err := writer.Write([]byte(fmt.Sprintf("\n\n%s  %v%v", msgTimestamp, style.Bold.Render(header), body)))
				if err != nil {
					return err
				}
			} else {
				// Otherwise, write the header and body
				_, err := writer.Write([]byte(fmt.Sprintf("\n\n%v%v", style.Bold.Render(header), body)))
				if err != nil {
					return err
				}
			}
		}
	}

	return scanner.Err()
}

// LogFlush write any remaining repeated events to the writer
func (p *StreamReader) LogFlush(writer io.Writer) {
	p.writeRepeatedEvent(writer)
}

func (p *StreamReader) updateRepeatCount(count int) {
	// Use a mutex to avoid conncurrent writes from multiple goroutines
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.repeatCount = count
}

func (p *StreamReader) updateLastEntry(header, body string) {
	// Use a mutex to avoid conncurrent writes from multiple goroutines
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.lastEntryHeader = header
	p.lastEntryBody = body
}

func (p *StreamReader) writeRepeatedEvent(writer io.Writer) {
	// Print the last entry if it's not the first and there were repeats for ALLOWED events
	if p.repeatCount > 0 {
		// Handle pluralization
		plural := ""
		if p.repeatCount > 1 {
			plural = "s"
		}

		// Start on a new line if there was a previous entry body so the repeated message isn't lost in the body text
		offset := " "
		if p.lastEntryBody != "" {
			offset = fmt.Sprintf("\n\n%s             ", p.indent)
		}

		countMsg := offset + style.RenderFmt(style.Gray, "(repeated %d time%s)", p.repeatCount, plural)
		_, err := writer.Write([]byte(countMsg))
		if err != nil {
			message.WarnErr(err, "Error writing repeated event")
		}

		// Reset the counter and last entry
		p.updateRepeatCount(0)
		p.updateLastEntry("", "")
	}
}

func (p *StreamReader) skipResource(event LogEntry) bool {
	if p.filterNamespace != "" {
		namespace := strings.ToLower(event.Namespace)
		if !strings.Contains(namespace, p.filterNamespace) {
			return true
		}
	}

	if p.filterName != "" {
		name := strings.ToLower(event.Name)
		if !strings.Contains(name, p.filterName) {
			return true
		}
	}

	return false
}

func (p *StreamReader) renderDenied(event LogEntry) string {
	var msg strings.Builder

	// Get the failure message
	splitMsg := strings.Split(event.Res.Status.Message, " Authorized: ")

	// Render the failure message in red
	msg.WriteString(style.RenderFmt(style.Red, "\n%s             %v", p.indent, splitMsg[0]))

	// If the message is not in the expected format, return the failure message as is
	if len(splitMsg) != 2 {
		return msg.String()
	}

	// Prepend `Authorized:` and `Found:` with newlines
	failureIndent := fmt.Sprintf("%s             ", p.indent)

	// Break the second part of the message into authorized and found messages
	splitMsg = strings.Split(splitMsg[1], " Found: ")

	// If the message is not in the expected format, return the failure message as is
	if len(splitMsg) != 2 {
		return msg.String()
	}

	// Write the authorized and found messages, separate "\n" due to lipgloss rendering issues
	msg.WriteString("\n\n" + style.RenderFmt(style.Bold, "%sAuthorized:", failureIndent))
	msg.WriteString("\n" + style.RenderFmt(style.CoolGray, "%s%v", failureIndent, splitMsg[0]))

	msg.WriteString("\n\n" + style.RenderFmt(style.Bold, "%sFound:", failureIndent))
	msg.WriteString("\n" + style.RenderFmt(style.CoolGray, "%s%v", failureIndent, splitMsg[1]))

	return msg.String()
}

func (p *StreamReader) renderMutation(event LogEntry) string {
	if event.Res.Patch != nil {
		decodedPatch, _ := base64.StdEncoding.DecodeString(*event.Res.Patch)

		var ops []operation

		if err := json.Unmarshal(decodedPatch, &ops); err != nil {
			message.WarnErr(err, "Error parsing JSON patch")
			return ""
		}

		// Format the JSON patch
		var formattedPatch strings.Builder

		// Group by operation type
		groups := make(map[string][]operation)
		for _, op := range ops {
			groups[op.Op] = append(groups[op.Op], op)
		}

		opMap := map[string]string{
			"add":     "ADDED",
			"remove":  "REMOVED",
			"replace": "REPLACED",
		}

		// Write the patch for each operation type
		for name, ops := range groups {
			format := "\n%s             %v=%v"

			if name == "remove" {
				format = "\n%s              %v"
			}

			// Write the subheader for the operation type
			formattedPatch.WriteString(style.RenderFmt(style.Bold, "\n%s   %s:", p.indent, opMap[name]))

			// Write the patch for each operation group
			for _, op := range ops {
				key := style.CoolGray.Render(op.Path)
				val := style.Cyan.Render(string(op.Value))
				formattedPatch.WriteString(fmt.Sprintf(format, p.indent, key, val))
			}
		}

		return formattedPatch.String()
	}

	return "No patch available"
}
