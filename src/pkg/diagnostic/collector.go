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

type RawFile struct {
	Name    string
	content []byte
}

type Collector interface {
	Collect(context.Context, string, Filter, Anonymizer) ([]RawFile, error)
}

var _ Collector = &ScriptCollector{}

type ScriptCollector struct {
	ScriptName string
}

func (s *ScriptCollector) Collect(ctx context.Context, namespace string, filter Filter, anonymizer Anonymizer) ([]RawFile, error) {
	if s.ScriptName == "" {
		return nil, fmt.Errorf("no script provided")
	}

	data, err := collectors.VendoredCollectors.ReadFile(s.ScriptName)
	if err != nil {
		return nil, fmt.Errorf("failed to read script: %w", err)
	}

	// Execute the script
	cmd := exec.CommandContext(ctx, "sh", "-c", string(data))
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute script: %w", err)
	}

	// Apply the filter
	outputStr := string(output)
	if !filter.Accept(outputStr) {
		return nil, nil
	}

	// Anonymize the content
	anonymizedContent := anonymizer.AnonymizeOutput(outputStr)

	// Create a RawFile
	rawFile := RawFile{
		Name:    s.ScriptName,
		content: []byte(anonymizedContent),
	}

	return []RawFile{rawFile}, nil
}

var _ Collector = &LogsCollector{}

type LogsCollector struct{}

func (s *LogsCollector) Collect(ctx context.Context, namespace string, filter Filter, anonymizer Anonymizer) ([]RawFile, error) {

	type LogsToCollect struct {
		Name          string
		ContainerName string
		Namespace     string
	}

	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var logsToCollect []LogsToCollect
	var result []RawFile
	for podIdx := range pods.Items {
		for containerIdx := range pods.Items[podIdx].Spec.Containers {
			logsToCollect = append(logsToCollect, LogsToCollect{
				Name:          pods.Items[podIdx].Name,
				ContainerName: pods.Items[podIdx].Spec.Containers[containerIdx].Name,
				Namespace:     pods.Items[podIdx].Namespace,
			})
		}
	}
	for i := range logsToCollect {
		podName := logsToCollect[i].Name
		PodLogsConnection := kubeClient.CoreV1().Pods(logsToCollect[i].Namespace).GetLogs(podName, &corev1.PodLogOptions{
			Follow:    false,
			TailLines: ptr.To(int64(100)),
			Container: logsToCollect[i].ContainerName,
		})
		LogStream, err := PodLogsConnection.Stream(ctx)
		if err != nil {
			fmt.Printf(redColor+"[%T] error from %s/%s, ignoring: %s\n"+resetColor, s, namespace, podName, err)
			continue
		}
		reader := bufio.NewScanner(LogStream)
		var line string
		var content []byte
		for reader.Scan() {
			line = fmt.Sprintf("%s\n", reader.Text())
			bytes := []byte(line)
			content = append(content, bytes...)
		}
		LogStream.Close()
		result = append(result, RawFile{
			Name:    fmt.Sprintf("logs-%s-%s-%s", logsToCollect[i].Namespace, logsToCollect[i].Name, logsToCollect[i].ContainerName),
			content: content,
		})
	}
	return result, nil
}

type CollectionResult struct {
	rawObjects []RawFile
	errors     []error
	namespace  string
	context    string
}

func Collect(ctx context.Context, namespace string, filter Filter, collectors []Collector, anonymizer Anonymizer) CollectionResult {
	result := CollectionResult{}
	result.namespace = namespace

	for _, collector := range collectors {
		collectedRawObjects, err := collector.Collect(ctx, namespace, filter, anonymizer)
		errorString := ""
		if err != nil {
			errorString = fmt.Sprintf(redColor+" error: %s"+resetColor, err)
		}
		fmt.Printf("[%T] collected %d files %s\n", collector, len(collectedRawObjects), errorString)
		result.rawObjects = append(result.rawObjects, collectedRawObjects...)
		if err != nil {
			result.errors = append(result.errors, err)
		}
	}
	return result
}
