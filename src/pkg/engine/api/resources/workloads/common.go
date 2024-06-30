package workloads

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	Added    = "ADDED"
	Modified = "MODIFIED"
	Deleted  = "DELETED"
)

// ResourceList is a thread-safe struct to store the list of resources and notify subscribers of changes.
type ResourceList[T metav1.Object] struct {
	mutex     sync.RWMutex
	resources map[string]T
	Changes   chan struct{}
}

// NewResourceList initializes a ResourceList and sets up event handlers for resource changes.
func NewResourceList[T metav1.Object](informer cache.SharedIndexInformer) *ResourceList[T] {
	r := &ResourceList[T]{
		resources: make(map[string]T),
		Changes:   make(chan struct{}, 1),
	}

	// Handlers to update the ResourceList
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			r.notifyChange(obj, Added)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			r.notifyChange(newObj, Modified)
		},
		DeleteFunc: func(obj interface{}) {
			r.notifyChange(obj, Deleted)
		},
	})

	return r
}

// GetResources returns a slice of the current resources.
func (r *ResourceList[T]) GetResources() []T {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	resources := make([]T, 0, len(r.resources))
	for _, resource := range r.resources {
		resources = append(resources, resource)
	}
	return resources
}

// notifyChange updates the ResourceList based on the event type and notifies subscribers of changes.
func (r *ResourceList[T]) notifyChange(obj interface{}, eventType string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	resource, ok := obj.(T)
	if !ok {
		return // or handle the error appropriately
	}

	switch eventType {
	case Added, Modified:
		r.resources[string(resource.GetUID())] = resource
	case Deleted:
		delete(r.resources, string(resource.GetUID()))
	}

	// Notify subscribers of the change
	select {
	case r.Changes <- struct{}{}:
	default:
	}
}
