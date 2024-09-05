// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

package state

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/message"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PkgStatus struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Status      string    `json:"status"`
	DateUpdated time.Time `json:"date_updated"`
}

type BundleState struct {
	Name        string      `json:"name"`
	Version     string      `json:"version"`
	PkgStatuses []PkgStatus `json:"packages"`
	Status      string      `json:"status"`
	DateUpdated time.Time   `json:"date_updated"`
}

type Client struct {
	client kubernetes.Interface
}

const (
	Success      = "success"       // deployed successfully
	Failed       = "failed"        // failed to deploy
	Deploying    = "deploying"     // deployment in progress
	NotDeployed  = "not_deployed"  // package is in the bundle but not deployed
	Removing     = "removing"      // removal in progress
	Removed      = "removed"       // package removed (does not apply to BundleState)
	FailedRemove = "failed_remove" // package failed to be removed (does not apply to BundleState)
	Unreferenced = "unreferenced"  // package has been removed from the bundle but still exists in the cluster
	stateNs      = "uds"
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
func (c *Client) InitBundleState(b *types.UDSBundle) error {
	err := c.ensureNamespace()
	if err != nil {
		return err
	}
	bundleState, isNewState, err := c.getOrCreateBundleState(b)
	if err != nil {
		return err
	} else if isNewState {
		message.Infof("Initialized bundle state for %s", b.Metadata.Name)
		return nil
	}

	// if existing state, update bundle state packages based on bundle YAML
	// create map of bundled packages for easy lookup
	bundledPkgs := make(map[string]types.Package, len(b.Packages))
	for _, pkg := range b.Packages {
		bundledPkgs[pkg.Name] = pkg
	}

	// create map of state packages for easy lookup
	statePkgs := make(map[string]PkgStatus, len(bundleState.PkgStatuses))
	for _, pkg := range bundleState.PkgStatuses {
		statePkgs[pkg.Name] = pkg
	}

	// check for updates and dropped/unreferenced packages
	for i, pkgStatus := range bundleState.PkgStatuses {
		_, exists := bundledPkgs[pkgStatus.Name] // check if bundled pkg is in state
		if exists {
			// existing package, update version and timestamp
			bundleState.PkgStatuses[i].Version = pkgStatus.Version
			bundleState.PkgStatuses[i].DateUpdated = time.Now()
		} else {
			// package no longer in bundle
			bundleState.PkgStatuses[i].Status = Unreferenced
			bundleState.PkgStatuses[i].DateUpdated = time.Now()
		}
	}

	// add new packages to state
	for _, pkg := range b.Packages {
		if _, exists := statePkgs[pkg.Name]; !exists {
			bundleState.PkgStatuses = append(bundleState.PkgStatuses, PkgStatus{
				Name:        pkg.Name,
				Version:     pkg.Ref,
				Status:      NotDeployed,
				DateUpdated: time.Now(),
			})
		}
	}

	// update state
	bundleState.Version = b.Metadata.Version
	bundleState.DateUpdated = time.Now()
	err = c.saveBundleState(bundleState)
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
func (c *Client) getOrCreateBundleState(b *types.UDSBundle) (*BundleState, bool, error) {
	var state *BundleState
	isNewState := false
	bundleName := b.Metadata.Name
	version := b.Metadata.Version
	stateSecretName := fmt.Sprintf("uds-bundle-%s", bundleName)
	stateSecret, err := c.client.CoreV1().Secrets(stateNs).Get(context.TODO(), stateSecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			var pkgStatuses []PkgStatus
			isNewState = true
			for _, pkg := range b.Packages {
				pkgStatuses = append(pkgStatuses, PkgStatus{
					Name:        pkg.Name,
					Version:     pkg.Ref,
					Status:      NotDeployed,
					DateUpdated: time.Now(),
				})
			}

			// init state and secret
			state = &BundleState{
				Name:        bundleName,
				Version:     version,
				PkgStatuses: pkgStatuses,
				DateUpdated: time.Now(),
			}

			// marshal into K8s secret and save
			jsonBundleState, err := json.Marshal(state)
			if err != nil {
				return nil, isNewState, fmt.Errorf("failed to marshal bundle state: %w", err)
			}
			stateSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: stateSecretName,
				},
				Data: map[string][]byte{
					"data": jsonBundleState,
				},
			}
			_, err = c.client.CoreV1().Secrets(stateNs).Create(context.TODO(), stateSecret, metav1.CreateOptions{})
			if err != nil {
				return nil, isNewState, err
			}
		} else {
			return nil, isNewState, err
		}
	} else {
		state, err = c.unmarshalBundleState(stateSecret)
		if err != nil {
			return nil, isNewState, err
		}
	}

	return state, isNewState, nil
}

// UpdateBundleState updates the bundle state in the K8s cluster
func (c *Client) UpdateBundleState(b *types.UDSBundle, status string) error {
	stateSecret, err := c.client.CoreV1().Secrets(stateNs).Get(context.TODO(), fmt.Sprintf("uds-bundle-%s", b.Metadata.Name), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get bundle state: %w", err)
	}
	bundleState, err := c.unmarshalBundleState(stateSecret)
	if err != nil {
		return err
	}

	bundleState.Status = status
	bundleState.Version = b.Metadata.Version
	bundleState.DateUpdated = time.Now()

	// update state
	err = c.saveBundleState(bundleState)
	return err
}

// GetBundleState gets the bundle state from the K8s cluster
func (c *Client) GetBundleState(b *types.UDSBundle) (*BundleState, error) {
	bundleName := b.Metadata.Name
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

// saveBundleState saves the bundle state to the K8s cluster (but doesn't create a new state)
func (c *Client) saveBundleState(stateToSave *BundleState) error {
	jsonBundleState, err := json.Marshal(stateToSave)
	if err != nil {
		return err
	}
	stateSecret, err := c.client.CoreV1().Secrets(stateNs).Get(context.TODO(), fmt.Sprintf("uds-bundle-%s", stateToSave.Name), metav1.GetOptions{})
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

func (c *Client) UpdateBundlePkgState(b *types.UDSBundle, bundledPkg types.Package, status string) error {
	// get state
	bundleName := b.Metadata.Name
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
		if pkg.Name == bundledPkg.Name {
			bundleState.PkgStatuses[i].Status = status
			bundleState.PkgStatuses[i].Version = bundledPkg.Ref
			bundleState.PkgStatuses[i].DateUpdated = time.Now()
			break
		}
	}

	// save state
	bundleState.DateUpdated = time.Now()
	err = c.saveBundleState(bundleState)
	return err
}

// GetBundlePkgState checks if a package exists in the bundle state
func (c *Client) GetBundlePkgState(b *types.UDSBundle, pkgName string) (*PkgStatus, error) {
	state, err := c.GetBundleState(b)
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

// RemoveBundleState removes the bundle state from the K8s cluster
func (c *Client) RemoveBundleState(b *types.UDSBundle) error {
	// ensure all packages have been removed before deleting
	bundleName := b.Metadata.Name
	state, err := c.GetBundleState(b)
	if err != nil {
		return err
	}

	partialRemoval := false
	for _, pkg := range state.PkgStatuses {
		if pkg.Status != Removed && pkg.Status != NotDeployed {
			partialRemoval = true
			message.Debugf("not removing state for bundle: %s, package %s still exists in state", bundleName, pkg.Name)
		}
	}

	if partialRemoval {
		err = c.UpdateBundleState(b, Success) // not removing entire bundle, reset status
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

func (c *Client) GetUnreferencedPackages(b *types.UDSBundle) ([]types.Package, error) {
	state, err := c.GetBundleState(b)
	if err != nil {
		return nil, err
	}

	var unreferencedPkgs []types.Package
	for _, p := range state.PkgStatuses {
		if p.Status == Unreferenced {
			unreferencedPkgs = append(unreferencedPkgs, types.Package{Name: p.Name, Ref: p.Version})
		}
	}

	return unreferencedPkgs, nil
}

func (c *Client) RemovePackageFromState(b *types.UDSBundle, pkgToRemove string) error {
	state, err := c.GetBundleState(b)
	if err != nil {
		return err
	}

	// remove pkg from list of statuses
	var newPkgStatuses []PkgStatus
	for _, p := range state.PkgStatuses {
		if p.Name != pkgToRemove {
			newPkgStatuses = append(newPkgStatuses, p)
		}
	}

	// update state
	state.PkgStatuses = newPkgStatuses
	state.DateUpdated = time.Now()
	err = c.saveBundleState(state)
	return err
}

// GetDeployedPackageNames returns the names of the packages that have been deployed
func GetDeployedPackageNames() []string {
	var deployedPackageNames []string
	c, _ := cluster.NewCluster()
	if c != nil {
		deployedPackages, _ := c.GetDeployedZarfPackages(context.TODO())
		for _, pkg := range deployedPackages {
			deployedPackageNames = append(deployedPackageNames, pkg.Name)
		}
	}
	return deployedPackageNames
}
