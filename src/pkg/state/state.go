// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package state

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/defenseunicorns/uds-cli/src/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PkgStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type BundleState struct {
	Name     string      `json:"name"`
	Packages []PkgStatus `json:"packages"`
	Status   string      `json:"status"`
}

type Client struct {
	client kubernetes.Interface
}

const udsNamespace = "uds"

func NewClient(client kubernetes.Interface) (*Client, error) {
	stateClient := &Client{
		client: client,
	}
	err := stateClient.ensureNamespace()
	if err != nil {
		return nil, err
	}
	return stateClient, nil
}

func (m *Client) ensureNamespace() error {
	_, err := m.client.CoreV1().Namespaces().Get(context.TODO(), udsNamespace, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: udsNamespace,
				},
			}
			_, err = m.client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create namespace: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get namespace: %w", err)
		}
	}
	return nil
}

func (m *Client) getOrCreateBundleState(bundleName string) (*corev1.Secret, error) {
	stateSecretName := fmt.Sprintf("uds-bundle-%s", bundleName)
	stateSecret, err := m.client.CoreV1().Secrets(udsNamespace).Get(context.TODO(), stateSecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Create an empty secret
			emptySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: stateSecretName,
				},
				// Leave StringData empty
			}
			stateSecret, err = m.client.CoreV1().Secrets(udsNamespace).Create(context.TODO(), emptySecret, metav1.CreateOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to create uds state secret: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to get uds state secret: %w", err)
		}
	}

	return stateSecret, nil
}

func (m *Client) UpdateBundleState(bundleName string, packagesToDeploy []types.Package) ([]string, error) {
	// track warnings
	warnings := make([]string, 0)

	stateSecret, err := m.getOrCreateBundleState(bundleName)
	if err != nil {
		return warnings, err
	}

	var currentState BundleState
	if len(stateSecret.Data["data"]) > 0 { // ensure state isn't empty before trying to unmarshal
		err = json.Unmarshal(stateSecret.Data["data"], &currentState)
		if err != nil {
			return warnings, fmt.Errorf("failed to unmarshal current state: %w", err)
		}
	}

	// Create a map of new packages for easy lookup
	newPkgs := make(map[string]bool)
	for _, pkg := range packagesToDeploy {
		newPkgs[pkg.Name] = true
	}

	// Check for removed packages
	for _, currentPkg := range currentState.Packages {
		if _, exists := newPkgs[currentPkg.Name]; !exists {
			warnings = append(warnings, fmt.Sprintf("package %s has been removed from the bundle", currentPkg.Name))
		}
	}

	// Create new package statuses from packagesToDeploy
	newPkgStatuses := make([]PkgStatus, len(packagesToDeploy))
	for i, pkg := range packagesToDeploy {
		newPkgStatuses[i] = PkgStatus{Name: pkg.Name, Status: "deploying"} // todo: const "deploying"
	}

	bundleState := &BundleState{
		Name:     bundleName,
		Packages: newPkgStatuses,
		Status:   "deploying",
	}

	jsonBundleState, err := json.Marshal(bundleState)
	if err != nil {
		return warnings, fmt.Errorf("failed to marshal bundle state: %w", err)
	}

	stateSecret.Data = map[string][]byte{
		"data": jsonBundleState,
	}

	_, err = m.client.CoreV1().Secrets(udsNamespace).Update(context.TODO(), stateSecret, metav1.UpdateOptions{})
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to update secret: %s", err))
	}

	return warnings, nil
}

func (m *Client) GetBundleState(bundleName string) (*BundleState, error) {
	stateSecret, err := m.getOrCreateBundleState(bundleName)
	if err != nil {
		return nil, err
	}

	return m.unmarshalBundleState(stateSecret)
}

func (m *Client) unmarshalBundleState(secret *corev1.Secret) (*BundleState, error) {
	var bundleState BundleState
	if data, ok := secret.Data["data"]; ok {
		err := json.Unmarshal(data, &bundleState)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal existing bundle state: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no data found in secret")
	}
	return &bundleState, nil
}
