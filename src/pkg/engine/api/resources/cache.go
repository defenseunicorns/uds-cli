// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/defenseunicorns/uds-cli/src/pkg/engine/api/resources/workloads"
	"github.com/defenseunicorns/uds-cli/src/pkg/engine/k8s"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

type Handler cache.ResourceEventHandlerFuncs

type Cache struct {
	stopper chan struct{}
	factory informers.SharedInformerFactory
	Pods    *workloads.PodList
}

func NewCache(ctx context.Context) (*Cache, error) {
	k8s, _, err := k8s.NewClient()
	if err != nil {
		return nil, fmt.Errorf("unable to connect to the cluster: %v", err)
	}

	informerFactory := informers.NewSharedInformerFactory(k8s, time.Minute*10)

	c := &Cache{
		factory: informerFactory,
		stopper: make(chan struct{}),
	}

	c.Pods = workloads.NewPodList(c.factory)

	// start the informer
	go c.factory.Start(c.stopper)

	// Stop the informer when the context is done
	go func() {
		<-ctx.Done()
		close(c.stopper)
	}()

	return c, nil
}
