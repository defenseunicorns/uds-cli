// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package engine contains the common logic for the UDS Engine.
package engine

import (
	"github.com/defenseunicorns/zarf/src/pkg/k8s"
	"github.com/defenseunicorns/zarf/src/pkg/message"
)

// Cluster represents a Kubernetes cluster for UDS Engine
type Cluster struct {
	*k8s.K8s
}

// NewCluster creates a new Cluster instance and validates connection to the cluster by fetching the Kubernetes version.
func NewCluster() (*Cluster, error) {
	c := &Cluster{}
	var err error

	c.K8s, err = k8s.New(message.Debugf)
	if err != nil {
		return nil, err
	}

	// Dogsled the version output. We just want to ensure no errors were returned to validate cluster connection.
	_, err = c.GetServerVersion()
	if err != nil {
		return nil, err
	}

	return c, nil
}
