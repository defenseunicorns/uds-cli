package diagnostic

import (
	"bufio"
	"context"
	"fmt"
	"github.com/defenseunicorns/uds-cli/src/pkg/diagnostic/collectors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	redColor   = "\033[31m"
	resetColor = "\033[0m"
)

// RawFile holds the name and raw content of a collected file.
type RawFile struct {
	Name    string
	content []byte
}

// Collector defines a unit that gathers RawFile entries from the cluster.
type Collector interface {
	Collect(ctx context.Context, namespace string, filter Filter) ([]RawFile, error)
}

var _ Collector = &ScriptCollector{}

type ScriptCollector struct {
	ScriptName string // filesystem path or embedded name
}

// Collect runs the script, filters its output, and returns raw (unmasked) content.
func (s *ScriptCollector) Collect(ctx context.Context, namespace string, filter Filter) ([]RawFile, error) {
	if s.ScriptName == "" {
		return nil, fmt.Errorf("no script provided")
	}

	data, err := collectors.VendoredCollectors.ReadFile(s.ScriptName)
	if err != nil {
		return nil, fmt.Errorf("failed to read script: %w", err)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", string(data))
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute script: %w", err)
	}
	content := string(output)

	if !filter.Accept(content) {
		return nil, nil
	}

	return []RawFile{{
		Name:    s.ScriptName,
		content: []byte(content),
	}}, nil
}

var _ Collector = &LogsCollector{}

type LogsCollector struct{}

// Collect streams pod logs, applies filter, and returns raw (unmasked) log files.
func (s *LogsCollector) Collect(ctx context.Context, namespace string, filter Filter) ([]RawFile, error) {
	type logInfo struct {
		pod, container string
	}

	// initialize Kubernetes client
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	pods, err := client.CoreV1().Pods(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var toCollect []logInfo
	for _, pod := range pods.Items {
		for _, ctr := range pod.Spec.Containers {
			toCollect = append(toCollect, logInfo{pod.Name, ctr.Name})
		}
	}

	var results []RawFile
	// fetch and aggregate logs per container
	for _, li := range toCollect {
		stream, err := client.CoreV1().Pods(namespace).GetLogs(li.pod, &corev1.PodLogOptions{
			Follow:    false,
			TailLines: ptr.To(int64(100)),
			Container: li.container,
		}).Stream(ctx)
		if err != nil {
			fmt.Printf(redColor+"[LogsCollector] error from %s/%s, ignoring: %v"+resetColor, namespace, li.pod, err)
			continue
		}
		defer stream.Close()

		scanner := bufio.NewScanner(stream)
		var buf []byte
		for scanner.Scan() {
			line := scanner.Text() + "\n"
			if filter.Accept(line) {
				buf = append(buf, []byte(line)...)
			}
		}
		results = append(results, RawFile{
			Name:    fmt.Sprintf("logs-%s-%s-%s", namespace, li.pod, li.container),
			content: buf,
		})
	}

	return results, nil
}

// CollectionResult aggregates all RawFile outputs and any errors encountered.
type CollectionResult struct {
	rawObjects []RawFile
	errors     []error
	namespace  string
	context    string
}

// Collect invokes each Collector, then applies anonymization exactly once per file.
func Collect(ctx context.Context, namespace string, filter Filter, collectors []Collector, anonymizer Anonymizer) CollectionResult {
	result := CollectionResult{namespace: namespace}

	for _, coll := range collectors {
		files, err := coll.Collect(ctx, namespace, filter)
		errMsg := ""
		if err != nil {
			errMsg = fmt.Sprintf(redColor+" error: %v"+resetColor, err)
		}
		fmt.Printf("[%T] collected %d files%s\n", coll, len(files), errMsg)

		// Apply anonymization in one central place
		for _, rf := range files {
			clean := anonymizer.AnonymizeOutput(string(rf.content))
			rf.content = []byte(clean)
			result.rawObjects = append(result.rawObjects, rf)
		}

		if err != nil {
			result.errors = append(result.errors, err)
		}
	}

	return result
}
