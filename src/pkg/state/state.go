// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package state

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/defenseunicorns/pkg/helpers/v2"
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
	Name        string      `json:"name"`
	PkgStatuses []PkgStatus `json:"packages"`
	Status      string      `json:"status"`
}

type Client struct {
	client kubernetes.Interface
}

const (
	Success   = "success"
	Failed    = "failed"
	Deploying = "deploying"
	stateNs   = "uds"
)

// NewClient creates a new state client
func NewClient(client kubernetes.Interface) (*Client, error) {
	stateClient := &Client{
		client: client,
	}
	return stateClient, nil
}

// InitBundleState initializes the bundle state in the K8s cluster if it doesn't exist
// this can safely be called multiple times
func (c *Client) InitBundleState(bundleName string) error {
	err := c.ensureNamespace()
	if err != nil {
		return err
	}
	_, err = c.getOrCreateBundleState(bundleName)
	if err != nil {
		return err
	}

	return err
}

// ensureNamespace creates the uds namespace if it doesn't exist
func (c *Client) ensureNamespace() error {
	_, err := c.client.CoreV1().Namespaces().Get(context.TODO(), stateNs, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: stateNs,
				},
			}
			_, err = c.client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create namespace: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get namespace: %w", err)
		}
	}
	return nil
}

// getOrCreateBundleState gets or creates the bundle state in the K8s cluster
func (c *Client) getOrCreateBundleState(bundleName string) (*BundleState, error) {
	var state *BundleState
	stateSecretName := fmt.Sprintf("uds-bundle-%s", bundleName)
	stateSecret, err := c.client.CoreV1().Secrets(stateNs).Get(context.TODO(), stateSecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// init state and secret
			state = &BundleState{
				Name:        bundleName,
				PkgStatuses: []PkgStatus{},
			}

			stateSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: stateSecretName,
				},
				Data: map[string][]byte{
					"data": {},
				},
			}
			_, err = c.client.CoreV1().Secrets(stateNs).Create(context.TODO(), stateSecret, metav1.CreateOptions{})
			if err != nil {
				return nil, err
			}
		}
	} else {
		state, err = c.unmarshalBundleState(stateSecret)
		if err != nil {
			return nil, err
		}
	}

	// update bundle state with Deploying status
	state.Status = Deploying

	// marshal into K8s secret and save
	jsonBundleState, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal bundle state: %w", err)
	}

	stateSecret.Data["data"] = jsonBundleState
	_, err = c.client.CoreV1().Secrets(stateNs).Update(context.TODO(), stateSecret, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update secret: %w", err)
	}

	return state, nil
}

// AddPackages adds packages to the bundle state
func (c *Client) AddPackages(bundleName string, packagesToDeploy []types.Package) ([]string, error) {
	// track warnings
	warnings := make([]string, 0)

	currentState, err := c.GetBundleState(bundleName)
	if err != nil {
		return nil, err
	}

	// Create a map of new packages for easy lookup
	newPkgs := make(map[string]bool)
	for _, pkg := range packagesToDeploy {
		newPkgs[pkg.Name] = true
	}

	// Check for removed packages
	for _, currentPkg := range currentState.PkgStatuses {
		if _, exists := newPkgs[currentPkg.Name]; !exists {
			warnings = append(warnings, fmt.Sprintf("package %s has been removed from the bundle", currentPkg.Name))
		}
	}

	// Create new package statuses from packagesToDeploy (set all packages to status: Deploying)
	newPkgStatuses := make([]PkgStatus, len(packagesToDeploy))
	for i, pkg := range packagesToDeploy {
		newPkgStatuses[i] = PkgStatus{Name: pkg.Name, Status: Deploying}
	}

	// dedup new packages with existing packages
	dedupedPkgStatuses := helpers.MergeSlices[PkgStatus](currentState.PkgStatuses, newPkgStatuses, func(i, j PkgStatus) bool {
		return i.Name == j.Name
	})

	newState := &BundleState{
		Name:        bundleName,
		PkgStatuses: dedupedPkgStatuses,
		Status:      Deploying,
	}

	jsonBundleState, err := json.Marshal(newState)
	if err != nil {
		return warnings, fmt.Errorf("failed to marshal bundle state: %w", err)
	}

	// marshal into K8s secret
	stateSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("uds-bundle-%s", bundleName),
		},
		Data: map[string][]byte{
			"data": jsonBundleState,
		},
	}

	_, err = c.client.CoreV1().Secrets(stateNs).Update(context.TODO(), stateSecret, metav1.UpdateOptions{})
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("failed to update secret: %s", err))
	}

	return warnings, nil
}

// UpdateBundleState updates the bundle state in the K8s cluster (not the packages in the state)
func (c *Client) UpdateBundleState(bundleName string, status string) error {
	stateSecret, err := c.client.CoreV1().Secrets(stateNs).Get(context.TODO(), fmt.Sprintf("uds-bundle-%s", bundleName), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get bundle state: %w", err)
	}
	bundleState, err := c.unmarshalBundleState(stateSecret)
	if err != nil {
		return err
	}

	bundleState.Status = status

	jsonBundleState, err := json.Marshal(bundleState)
	if err != nil {
		return fmt.Errorf("failed to marshal bundle state: %w", err)
	}

	stateSecret.Data["data"] = jsonBundleState
	_, err = c.client.CoreV1().Secrets(stateNs).Update(context.TODO(), stateSecret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	return nil
}

// GetBundleState gets the bundle state from the K8s cluster
func (c *Client) GetBundleState(bundleName string) (*BundleState, error) {
	stateSecret, err := c.client.CoreV1().Secrets(stateNs).Get(context.TODO(), fmt.Sprintf("uds-bundle-%s", bundleName), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get bundle state: %w", err)
	}
	return c.unmarshalBundleState(stateSecret)
}

func (c *Client) unmarshalBundleState(secret *corev1.Secret) (*BundleState, error) {
	var bundleState BundleState
	if data, ok := secret.Data["data"]; ok {
		err := json.Unmarshal(data, &bundleState)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal existing bundle state: %w", err)
		}
	}
	return &bundleState, nil
}

func (c *Client) UpdateBundlePkgState(bundleName string, pkgName string, status string) error {
	// get state
	stateSecret, err := c.client.CoreV1().Secrets(stateNs).Get(context.TODO(), fmt.Sprintf("uds-bundle-%s", bundleName), metav1.GetOptions{})
	if err != nil {
		return err
	}
	bundleState, err := c.unmarshalBundleState(stateSecret)
	if err != nil {
		return err
	}

	// update pkg status
	for i, pkg := range bundleState.PkgStatuses {
		if pkg.Name == pkgName {
			bundleState.PkgStatuses[i].Status = status
		}
	}

	// save state
	jsonBundleState, err := json.Marshal(bundleState)
	if err != nil {
		return err
	}
	stateSecret.Data["data"] = jsonBundleState
	_, err = c.client.CoreV1().Secrets(stateNs).Update(context.TODO(), stateSecret, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

// PkgExistsInState checks if a package exists in the bundle state
func (c *Client) PkgExistsInState(bundleName string, pkgName string) (bool, error) {
	state, err := c.GetBundleState(bundleName)
	if err != nil {
		return false, err
	}

	for _, pkg := range state.PkgStatuses {
		if pkg.Name == pkgName {
			return true, nil
		}
	}
	return false, nil
}
