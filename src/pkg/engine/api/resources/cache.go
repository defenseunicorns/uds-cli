// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/defenseunicorns/uds-cli/src/pkg/engine/api/resources/workloads"
	"github.com/defenseunicorns/uds-cli/src/pkg/engine/k8s"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

type Handler cache.ResourceEventHandlerFuncs

type Cache struct {
	stopper      chan struct{}
	factory      informers.SharedInformerFactory
	Namespaces   *workloads.ResourceList[*v1.Namespace]
	Pods         *workloads.ResourceList[*v1.Pod]
	Deployments  *workloads.ResourceList[*appsv1.Deployment]
	Daemonsets   *workloads.ResourceList[*appsv1.DaemonSet]
	Statefulsets *workloads.ResourceList[*appsv1.StatefulSet]
}

func NewCache(ctx context.Context) (*Cache, error) {
	k8s, _, err := k8s.NewClient()
	if err != nil {
		return nil, fmt.Errorf("unable to connect to the cluster: %v", err)
	}

	factory := informers.NewSharedInformerFactory(k8s, time.Minute*10)

	c := &Cache{
		factory: factory,
		stopper: make(chan struct{}),
	}

	c.Namespaces = workloads.NewResourceList[*v1.Namespace](factory.Core().V1().Namespaces().Informer())
	c.Pods = workloads.NewResourceList[*v1.Pod](factory.Core().V1().Pods().Informer())
	c.Deployments = workloads.NewResourceList[*appsv1.Deployment](factory.Apps().V1().Deployments().Informer())
	c.Daemonsets = workloads.NewResourceList[*appsv1.DaemonSet](factory.Apps().V1().DaemonSets().Informer())
	c.Statefulsets = workloads.NewResourceList[*appsv1.StatefulSet](factory.Apps().V1().StatefulSets().Informer())

	// start the informer
	go c.factory.Start(c.stopper)

	// Stop the informer when the context is done
	go func() {
		<-ctx.Done()
		close(c.stopper)
	}()

	return c, nil
}
