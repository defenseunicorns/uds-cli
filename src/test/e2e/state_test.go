package test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/pkg/state"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUDSStateOnDeploy(t *testing.T) {
	e2e.CreateZarfPkg(t, "src/test/packages/no-cluster/output-var", false)
	e2e.CreateZarfPkg(t, "src/test/packages/no-cluster/receive-var", false)

	// deploy bundle
	bundleName := "state"
	bundlePath := "src/test/bundles/16-state/"
	bundleTarball := fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch)
	deployPath := fmt.Sprintf("%s/%s", bundlePath, bundleTarball)
	cleanStateSecret(t, bundleName)
	runCmd(t, fmt.Sprintf("create %s --confirm", bundlePath))
	runCmd(t, fmt.Sprintf("deploy %s --confirm", deployPath))

	t.Run("on deploy", func(t *testing.T) {
		bundleState := getStateSecret(t, bundleName)
		require.Equal(t, bundleName, bundleState.Name)
		require.Equal(t, state.Success, bundleState.Status)
		require.Len(t, bundleState.PkgStatuses, 2)
		require.Equal(t, state.Success, bundleState.PkgStatuses[0].Status)
	})

	t.Run("on deploy with --packages flag", func(t *testing.T) {
		runCmd(t, fmt.Sprintf("deploy --packages=receive-var %s --confirm", deployPath))
		bundleState := getStateSecret(t, bundleName)
		require.Equal(t, bundleName, bundleState.Name)
		require.Equal(t, state.Success, bundleState.Status)
		require.Len(t, bundleState.PkgStatuses, 2)
		require.Equal(t, state.Success, bundleState.PkgStatuses[0].Status)
	})

	t.Run("on deploy with --resume flag", func(t *testing.T) {
		cleanStateSecret(t, bundleName) // start with fresh state
		runCmd(t, fmt.Sprintf("deploy --packages=output-var %s --confirm", deployPath))
		bundleState := getStateSecret(t, bundleName)
		require.Equal(t, bundleName, bundleState.Name)
		require.Equal(t, state.Success, bundleState.Status)
		require.Len(t, bundleState.PkgStatuses, 1)
		require.Equal(t, state.Success, bundleState.PkgStatuses[0].Status)

		runCmd(t, fmt.Sprintf("deploy --resume %s --confirm", deployPath))
		bundleState = getStateSecret(t, bundleName)
		require.Equal(t, state.Success, bundleState.Status)
		require.Len(t, bundleState.PkgStatuses, 2)
	})
}

//func TestUDSStateOnRemove(t *testing.T) {
//	e2e.CreateZarfPkg(t, "src/test/packages/no-cluster/output-var", false)
//	e2e.CreateZarfPkg(t, "src/test/packages/no-cluster/receive-var", false)
//
//	bundleName := "state"
//	bundlePath := "src/test/bundles/16-state/"
//	bundleTarball := fmt.Sprintf("uds-bundle-%s-%s-0.0.1.tar.zst", bundleName, e2e.Arch)
//	deployPath := fmt.Sprintf("%s/%s", bundlePath, bundleTarball)
//	cleanStateSecret(t, bundleName)
//	runCmd(t, fmt.Sprintf("create %s --confirm", bundlePath))
//
//	t.Run("on remove", func(t *testing.T) {
//		runCmd(t, fmt.Sprintf("deploy %s --confirm", deployPath))
//		runCmd(t, fmt.Sprintf("remove %s --confirm", deployPath))
//		bundleState := getStateSecret(t, bundleName)
//		require.Nil(t, bundleState)
//	})
//
//	t.Run("on remove with --packages flag", func(t *testing.T) {
//		runCmd(t, fmt.Sprintf("deploy %s --confirm", deployPath))
//		bundleState := getStateSecret(t, bundleName)
//		require.Len(t, bundleState.PkgStatuses, 2)
//		runCmd(t, fmt.Sprintf("remove --packages=receive-var %s --confirm", deployPath))
//		bundleState = getStateSecret(t, bundleName)
//		require.Len(t, bundleState.PkgStatuses, 1)
//	})
//}

func cleanStateSecret(t *testing.T, bundleName string) {
	kc, err := cluster.NewCluster()
	require.NoError(t, err)
	err = kc.Clientset.CoreV1().Secrets("uds").
		Delete(context.TODO(), fmt.Sprintf("uds-bundle-%s", bundleName), metav1.DeleteOptions{})
	if !errors.IsNotFound(err) {
		require.NoError(t, err)
	}
}

func getStateSecret(t *testing.T, bundleName string) *state.BundleState {
	kc, err := cluster.NewCluster()
	require.NoError(t, err)

	// Get the secret
	secret, err := kc.Clientset.CoreV1().Secrets("uds").Get(context.TODO(), fmt.Sprintf("uds-bundle-%s", bundleName), metav1.GetOptions{})
	require.NoError(t, err)

	// marshal into struct for easy assertions
	var bundleState *state.BundleState
	err = json.Unmarshal(secret.Data["data"], &bundleState)
	require.NoError(t, err)

	return bundleState
}
