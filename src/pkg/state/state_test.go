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

func TestNewClientNoNamespace(t *testing.T) {
	fakeClient := fake.NewClientset()
	client, err := NewClient(fakeClient)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Check if the namespace was created
	ns, err := fakeClient.CoreV1().Namespaces().Get(context.TODO(), stateNs, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, stateNs, ns.Name)
}

func TestNewClientExistingNamespace(t *testing.T) {
	fakeClient := fake.NewClientset()

	// Create the UDS namespace
	_, err := fakeClient.CoreV1().Namespaces().Create(context.TODO(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: stateNs}}, metav1.CreateOptions{})
	require.NoError(t, err)

	client, err := NewClient(fakeClient)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Check if the namespace was created
	ns, err := fakeClient.CoreV1().Namespaces().Get(context.TODO(), stateNs, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, stateNs, ns.Name)
}

func TestUpdateBundleStateWithPackageRemoval(t *testing.T) {
	fakeClient := fake.NewClientset()
	client, _ := NewClient(fakeClient)

	// Create an initial state
	initialState := &BundleState{
		Name:     "test-bundle",
		Packages: []PkgStatus{{Name: "pkg1", Status: "deployed"}},
		Status:   "deployed", // todo: const
	}
	initialStateJSON, _ := json.Marshal(initialState)
	_, err := fakeClient.CoreV1().Secrets(stateNs).Create(context.TODO(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "uds-bundle-test-bundle"},
		Data:       map[string][]byte{"data": initialStateJSON},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	// Update the state
	packagesToDeploy := []types.Package{{Name: "pkg2"}}
	warnings, err := client.UpdateBundleState("test-bundle", packagesToDeploy)

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
	require.Len(t, updatedState.Packages, 1)
	require.Equal(t, "pkg2", updatedState.Packages[0].Name)
	require.Equal(t, "deploying", updatedState.Packages[0].Status)
}

func TestGetBundleState(t *testing.T) {
	fakeClient := fake.NewClientset()
	client, _ := NewClient(fakeClient)

	// Create a test state
	testState := &BundleState{
		Name:     "test-bundle",
		Packages: []PkgStatus{{Name: "pkg1", Status: "deployed"}},
		Status:   "deployed",
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
	require.Len(t, bundleState.Packages, 1)
	require.Equal(t, "pkg1", bundleState.Packages[0].Name)
	require.Equal(t, "deployed", bundleState.Packages[0].Status)
	require.Equal(t, "deployed", bundleState.Status)
}

func TestGetOrCreateBundleState(t *testing.T) {
	fakeClient := fake.NewClientset()
	client, _ := NewClient(fakeClient)

	// Test creating a new state
	secret, err := client.getOrCreateBundleState("new-bundle")
	require.NoError(t, err)
	require.NotNil(t, secret)
	require.Equal(t, "uds-bundle-new-bundle", secret.Name)

	// Test getting an existing state
	existingSecret, err := client.getOrCreateBundleState("new-bundle")
	require.NoError(t, err)
	require.NotNil(t, existingSecret)
	require.Equal(t, secret.Name, existingSecret.Name)
}
