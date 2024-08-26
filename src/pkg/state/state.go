// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package state

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/zarf-dev/zarf/src/pkg/message"
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
	Success        = "success"
	Failed         = "failed"
	Deploying      = "deploying"
	AwaitingDeploy = "awaiting_deploy" // package is in the bundle but not yet deployed
	Removing       = "removing"
	Removed        = "removed"
	stateNs        = "uds"
)

// NewClient creates a new state client
func NewClient(client kubernetes.Interface) (*Client, error) {
	stateClient := &Client{
		client: client,
	}
	return stateClient, nil
}

// InitBundleState initializes the bundle state in the K8s cluster if it doesn't exist.
// This can safely be called multiple times
func (c *Client) InitBundleState(b types.UDSBundle) error {
	err := c.ensureNamespace()
	if err != nil {
		return err
	}
	_, err = c.getOrCreateBundleState(b)
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
func (c *Client) getOrCreateBundleState(b types.UDSBundle) (*BundleState, error) {
	var state *BundleState
	bundleName := b.Metadata.Name
	stateSecretName := fmt.Sprintf("uds-bundle-%s", bundleName)
	stateSecret, err := c.client.CoreV1().Secrets(stateNs).Get(context.TODO(), stateSecretName, metav1.GetOptions{})
	var pkgStatuses []PkgStatus
	for _, pkg := range b.Packages {
		pkgStatuses = append(pkgStatuses, PkgStatus{Name: pkg.Name, Status: AwaitingDeploy})
	}
	if err != nil {
		if errors.IsNotFound(err) {
			// init state and secret
			state = &BundleState{
				Name:        bundleName,
				PkgStatuses: pkgStatuses,
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

// GetBundlePkg checks if a package exists in the bundle state
func (c *Client) GetBundlePkg(bundleName string, pkgName string) (*PkgStatus, error) {
	state, err := c.GetBundleState(bundleName)
	if err != nil {
		return nil, err
	}

	for _, pkg := range state.PkgStatuses {
		if pkg.Name == pkgName {
			return &pkg, nil
		}
	}
	return nil, nil
}

func (c *Client) RemovePackageState(name string, pkgToRemove types.Package) error {
	state, err := c.GetBundleState(name)
	if err != nil {
		return err
	}

	// find pkg in state and remove
	newPkgStatuses := make([]PkgStatus, 0)
	for i, p := range state.PkgStatuses {
		if p.Name != pkgToRemove.Name {
			newPkgStatuses = append(newPkgStatuses, state.PkgStatuses[i])
		}
	}

	// save state
	state.PkgStatuses = newPkgStatuses
	jsonBundleState, err := json.Marshal(state)
	if err != nil {
		return err
	}
	stateSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("uds-bundle-%s", name),
		},
		Data: map[string][]byte{
			"data": jsonBundleState,
		},
	}
	_, err = c.client.CoreV1().Secrets(stateNs).Update(context.TODO(), stateSecret, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) RemoveBundleState(bundleName string) error {
	// ensure all packages have been removed before deleting
	state, err := c.GetBundleState(bundleName)
	if err != nil {
		return err
	}

	partialRemoval := false
	for _, pkg := range state.PkgStatuses {
		if pkg.Status != Removed {
			partialRemoval = true
			message.Debugf("not removing state for bundle: %s, package %s still exists in state", bundleName, pkg.Name)
		}
	}

	if partialRemoval {
		err = c.UpdateBundleState(bundleName, Success) // not removing entire bundle, reset status
		if err != nil {
			return err
		}
		return nil
	}

	// remove bundle state
	err = c.client.CoreV1().Secrets(stateNs).Delete(context.TODO(),
		fmt.Sprintf("uds-bundle-%s", bundleName), metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}
