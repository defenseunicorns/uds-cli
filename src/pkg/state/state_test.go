package state

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/defenseunicorns/uds-cli/src/types"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestUpdateBundleStateWithNoCleanup(t *testing.T) {
	fakeClient := fake.NewClientset()
	client, _ := NewClient(fakeClient)

	// Create an initial state
	initialState := &BundleState{
		Name:        "test-bundle",
		PkgStatuses: []PkgStatus{{Name: "pkg1", Status: Success}},
		Status:      Success,
	}
	initialStateJSON, _ := json.Marshal(initialState)
	_, err := fakeClient.CoreV1().Secrets(stateNs).Create(context.TODO(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "uds-bundle-test-bundle"},
		Data:       map[string][]byte{"data": initialStateJSON},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Update the state
	packagesToDeploy := []types.Package{{Name: "pkg2"}}
	warnings, err := client.AddPackages("test-bundle", packagesToDeploy)

	require.NoError(t, err)
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0], "pkg1 has been removed")

	// Verify the updated state
	updatedSecret, err := fakeClient.CoreV1().Secrets(stateNs).Get(context.TODO(), "uds-bundle-test-bundle", metav1.GetOptions{})
	require.NoError(t, err)

	var updatedState BundleState
	err = json.Unmarshal(updatedSecret.Data["data"], &updatedState)
	require.NoError(t, err)
	require.Equal(t, "test-bundle", updatedState.Name)
	require.Len(t, updatedState.PkgStatuses, 2)
}

func TestUpdateBundleStateWithSamePackages(t *testing.T) {
	fakeClient := fake.NewClientset()
	client, _ := NewClient(fakeClient)

	// Create an initial state
	initialState := &BundleState{
		Name:        "test-bundle",
		PkgStatuses: []PkgStatus{{Name: "pkg1", Status: Success}, {Name: "pkg2", Status: Success}},
		Status:      Success,
	}
	initialStateJSON, _ := json.Marshal(initialState)
	_, err := fakeClient.CoreV1().Secrets(stateNs).Create(context.TODO(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "uds-bundle-test-bundle"},
		Data:       map[string][]byte{"data": initialStateJSON},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Update the state
	packagesToDeploy := []types.Package{{Name: "pkg1"}, {Name: "pkg2"}}
	warnings, err := client.AddPackages("test-bundle", packagesToDeploy)

	require.NoError(t, err)
	require.Len(t, warnings, 0)

	// Verify the updated state
	updatedSecret, err := fakeClient.CoreV1().Secrets(stateNs).Get(context.TODO(), "uds-bundle-test-bundle", metav1.GetOptions{})
	require.NoError(t, err)

	var updatedState BundleState
	err = json.Unmarshal(updatedSecret.Data["data"], &updatedState)
	require.NoError(t, err)
	require.Equal(t, "test-bundle", updatedState.Name)
	require.Len(t, updatedState.PkgStatuses, 2)
}

func TestGetExistingBundleState(t *testing.T) {
	fakeClient := fake.NewClientset()
	client, _ := NewClient(fakeClient)

	// Create a test state
	testState := &BundleState{
		Name:        "test-bundle",
		PkgStatuses: []PkgStatus{{Name: "pkg1", Status: Success}},
		Status:      Success,
	}
	testStateJSON, _ := json.Marshal(testState)
	_, err := fakeClient.CoreV1().Secrets(stateNs).Create(context.TODO(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "uds-bundle-test-bundle"},
		Data:       map[string][]byte{"data": testStateJSON},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Get the state
	bundleState, err := client.GetBundleState("test-bundle")

	require.NoError(t, err)
	require.NotNil(t, bundleState)
	require.Equal(t, "test-bundle", bundleState.Name)
	require.Len(t, bundleState.PkgStatuses, 1)
	require.Equal(t, "pkg1", bundleState.PkgStatuses[0].Name)
	require.Equal(t, Success, bundleState.PkgStatuses[0].Status)
	require.Equal(t, Success, bundleState.Status)
}

func TestGetBundleState(t *testing.T) {
	fakeClient := fake.NewClientset()
	client, _ := NewClient(fakeClient)

	// Create a test state
	testState := &BundleState{
		Name:        "test-bundle",
		PkgStatuses: []PkgStatus{{Name: "pkg1", Status: Deploying}},
		Status:      Deploying,
	}
	testStateJSON, _ := json.Marshal(testState)
	_, err := fakeClient.CoreV1().Secrets(stateNs).Create(context.TODO(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "uds-bundle-test-bundle"},
		Data:       map[string][]byte{"data": testStateJSON},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Get the state
	bundleState, err := client.GetBundleState("test-bundle")

	require.NoError(t, err)
	require.NotNil(t, bundleState)
	require.Equal(t, "test-bundle", bundleState.Name)
	require.Len(t, bundleState.PkgStatuses, 1)
	require.Equal(t, "pkg1", bundleState.PkgStatuses[0].Name)
	require.Equal(t, Deploying, bundleState.PkgStatuses[0].Status)
	require.Equal(t, Deploying, bundleState.Status)
}

func TestInitBundleState(t *testing.T) {
	fakeClient := fake.NewClientset()
	client, _ := NewClient(fakeClient)

	err := client.InitBundleState("test-bundle")
	require.NoError(t, err)

	bundleState, err := client.GetBundleState("test-bundle")
	require.NoError(t, err)
	require.NotNil(t, bundleState)
	require.Equal(t, "test-bundle", bundleState.Name)
	require.Len(t, bundleState.PkgStatuses, 0)
	require.Equal(t, "deploying", bundleState.Status)
}

func TestInitExistingBundleState(t *testing.T) {
	fakeClient := fake.NewClientset()
	client, _ := NewClient(fakeClient)

	// Create an existing state
	existingState := &BundleState{
		Name:        "test-bundle",
		PkgStatuses: []PkgStatus{{Name: "pkg1", Status: Success}},
		Status:      Success,
	}

	existingStateJSON, err := json.Marshal(existingState)
	require.NoError(t, err)
	_, err = fakeClient.CoreV1().Secrets(stateNs).Create(context.TODO(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "uds-bundle-test-bundle"},
		Data:       map[string][]byte{"data": existingStateJSON},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	err = client.InitBundleState("test-bundle")
	require.NoError(t, err)

	bundleState, err := client.GetBundleState("test-bundle")
	require.NoError(t, err)
	require.NotNil(t, bundleState)
	require.Equal(t, "test-bundle", bundleState.Name)
	require.Len(t, bundleState.PkgStatuses, 1)
	require.Equal(t, Deploying, bundleState.Status)
}

func TestUpdatePackageState(t *testing.T) {
	fakeClient := fake.NewClientset()
	client, _ := NewClient(fakeClient)

	// Create an initial state
	initialState := &BundleState{
		Name:        "test-bundle",
		PkgStatuses: []PkgStatus{{Name: "pkg1", Status: Deploying}},
		Status:      Deploying,
	}
	initialStateJSON, _ := json.Marshal(initialState)
	_, err := fakeClient.CoreV1().Secrets(stateNs).Create(context.TODO(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "uds-bundle-test-bundle"},
		Data:       map[string][]byte{"data": initialStateJSON},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Update the state
	err = client.UpdateBundlePkgState("test-bundle", "pkg1", Success)
	require.NoError(t, err)

	// Verify the updated state
	updatedSecret, err := fakeClient.CoreV1().Secrets(stateNs).Get(context.TODO(), "uds-bundle-test-bundle", metav1.GetOptions{})
	require.NoError(t, err)

	var updatedState BundleState
	err = json.Unmarshal(updatedSecret.Data["data"], &updatedState)
	require.NoError(t, err)
	require.Equal(t, "test-bundle", updatedState.Name)
	require.Len(t, updatedState.PkgStatuses, 1)
	require.Equal(t, "pkg1", updatedState.PkgStatuses[0].Name)
	require.Equal(t, Success, updatedState.PkgStatuses[0].Status)
}
