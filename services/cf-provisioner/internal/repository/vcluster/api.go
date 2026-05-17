package vcluster

import "context"

// VClusterClient manages vCluster instances on the host Kubernetes cluster.
type VClusterClient interface {
	// Create provisions a new vCluster in the host cluster.
	Create(ctx context.Context, params CreateVClusterParams) (VClusterInfo, error)

	// Get returns the current status of a vCluster.
	Get(ctx context.Context, name string) (VClusterInfo, error)

	// Delete destroys a vCluster and all its resources.
	Delete(ctx context.Context, name string) error

	// GetKubeconfig retrieves the admin kubeconfig for the vCluster.
	GetKubeconfig(ctx context.Context, name string) ([]byte, error)
}

// New builds a [VClusterClient] from kubeconfig bytes for the management cluster.
func New(hostKubeconfig []byte) (VClusterClient, error) {
	return newVClusterClient(hostKubeconfig, osExecRunner{})
}
