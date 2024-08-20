package test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/pkg/state"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestUDSState(t *testing.T) {
	// deploy bundle
	bundleName := "state"
	bundlePath := "src/test/bundles/16-state/"
	cleanStateSecret(t, bundleName)
	runCmd(t, fmt.Sprintf("dev deploy %s", bundlePath))

	t.Run("Test UDS state", func(t *testing.T) {
		bundleState := getStateSecret(t, bundleName)
		require.Equal(t, bundleName, bundleState.Name)
		require.Equal(t, state.Success, bundleState.Status)
		require.Len(t, bundleState.PkgStatuses, 2)
		require.Equal(t, state.Success, bundleState.PkgStatuses[0].Status)
	})

	t.Run("Test UDS State with --packages flag", func(t *testing.T) {
		runCmd(t, fmt.Sprintf("dev deploy --packages=receive-var %s", bundlePath))
		bundleState := getStateSecret(t, bundleName)
		require.Equal(t, bundleName, bundleState.Name)
		require.Equal(t, state.Success, bundleState.Status)
		require.Len(t, bundleState.PkgStatuses, 2)
		require.Equal(t, state.Success, bundleState.PkgStatuses[0].Status)
	})

	t.Run("Test UDS State with --resume flag", func(t *testing.T) {

	})

}

func cleanStateSecret(t *testing.T, bundleName string) {
	kc, err := cluster.NewCluster()
	require.NoError(t, err)
	err = kc.Clientset.CoreV1().Secrets("uds").
		Delete(context.TODO(), fmt.Sprintf("uds-bundle-%s", bundleName), metav1.DeleteOptions{})
	require.NoError(t, err)
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
