package workloads

import (
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

const (
	Added    = "ADDED"
	Modified = "MODIFIED"
	Deleted  = "DELETED"
)

// PodList is a thread-safe struct to store the list of pods and notify subscribers of changes.
type PodList struct {
	mutex   sync.RWMutex
	pods    map[string]*v1.Pod
	Changes chan struct{}
}

// NewPodList initializes a PodList and sets up event handlers for pod changes.
func NewPodList(informerFactory informers.SharedInformerFactory) *PodList {
	p := &PodList{
		pods:    make(map[string]*v1.Pod),
		Changes: make(chan struct{}, 1),
	}

	podInformer := informerFactory.Core().V1().Pods()

	// Handlers to update the PodList
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			p.notifyChange(obj, Added)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			p.notifyChange(newObj, Modified)
		},
		DeleteFunc: func(obj interface{}) {
			p.notifyChange(obj, Deleted)
		},
	})

	return p
}

// GetPods returns a slice of the current pods.
func (p *PodList) GetPods() []*v1.Pod {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	pods := make([]*v1.Pod, 0, len(p.pods))
	for _, pod := range p.pods {
		pods = append(pods, pod)
	}
	return pods
}

// notifyChange updates the PodList based on the event type and notifies subscribers of changes.
func (p *PodList) notifyChange(obj interface{}, eventType string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	pod, ok := obj.(*v1.Pod)
	if !ok {
		return // or handle the error appropriately
	}

	switch eventType {
	case Added, Modified:
		p.pods[string(pod.UID)] = pod
	case Deleted:
		delete(p.pods, string(pod.UID))
	}

	// Notify subscribers of the change
	select {
	case p.Changes <- struct{}{}:
	default:
	}
}
