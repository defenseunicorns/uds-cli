// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package k8s contains k8s client logic
package k8s

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewClient creates a new Kubernetes cluster client
func NewClient() (*kubernetes.Clientset, *rest.Config, error) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{}).ClientConfig()

	if err != nil {
		return nil, nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}

	return clientset, config, nil
}
