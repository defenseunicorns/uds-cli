package diagnostic

import (
	"bufio"
	"context"
	"fmt"
	"github.com/defenseunicorns/uds-cli/src/pkg/diagnostic/collectors"
	"github.com/goccy/go-yaml"
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
	Collect(ctx context.Context, namespace string, filter Filter, anonymizer Anonymizer) ([]RawFile, error)
}

var _ Collector = &ScriptCollector{}

type ScriptCollector struct {
	ScriptName string // filesystem path or embedded name
}

// Collect runs the script, filters its output, and returns raw (unmasked) content.
func (s *ScriptCollector) Collect(ctx context.Context, namespace string, filter Filter, anonymizer Anonymizer) ([]RawFile, error) {
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
func (s *LogsCollector) Collect(ctx context.Context, namespace string, filter Filter, anonymizer Anonymizer) ([]RawFile, error) {
	type logInfo struct {
		pod, container, namespace string
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
			toCollect = append(toCollect, logInfo{pod.Name, ctr.Name, pod.Namespace})
		}
	}

	var results []RawFile
	// fetch and aggregate logs per container
	for _, li := range toCollect {
		stream, err := client.CoreV1().Pods(li.namespace).GetLogs(li.pod, &corev1.PodLogOptions{
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
				buf = append(buf, []byte(anonymizer.AnonymizeOutput(line, false))...)
			}
		}
		results = append(results, RawFile{
			Name:    fmt.Sprintf("logs-%s-%s-%s", li.namespace, li.pod, li.container),
			content: buf,
		})
	}

	return results, nil
}

var _ Collector = &SecretCollector{}

type SecretCollector struct{}

func (s *SecretCollector) Collect(ctx context.Context, namespace string, filter Filter, anonymizer Anonymizer) ([]RawFile, error) {
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	secrets, err := client.CoreV1().Secrets(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range secrets.Items {
		item := secrets.Items[i]
		//if filter.Accept(&item) {
		for k, v := range item.Data {
			item.Data[k] = []byte(anonymizer.AnonymizeOutput(string(v), true))
		}
	}

	textOutput, err := yaml.Marshal(secrets)
	if err != nil {
		return nil, err
	}

	return []RawFile{
		{
			Name:    "secrets",
			content: textOutput,
		},
	}, nil
}

// CollectionResult aggregates all RawFile outputs and any Errors encountered.
type CollectionResult struct {
	RawObjects []RawFile
	Errors     []error
	Namespace  string
}

// Collect invokes each Collector, then applies anonymization exactly once per file.
func Collect(ctx context.Context, namespace string, filter Filter, collectors []Collector, anonymizer Anonymizer) CollectionResult {
	result := CollectionResult{Namespace: namespace}

	for _, coll := range collectors {
		files, err := coll.Collect(ctx, namespace, filter, anonymizer)
		// Apply anonymization in one central place
		for _, rf := range files {
			//clean := anonymizer.AnonymizeOutput(string(rf.content), false)
			//rf.content = []byte(clean)
			result.RawObjects = append(result.RawObjects, rf)
		}

		if err != nil {
			result.Errors = append(result.Errors, err)
		}
	}

	return result
}
