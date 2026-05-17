package cilium

import "context"

// CiliumClient manages CiliumNetworkPolicy objects on the host cluster.
//
// CRDs are applied as unstructured objects because the Cilium Go API types are
// not vendored into this module; the shape matches cilium.io/v2 CiliumNetworkPolicy.
type CiliumClient interface {
	// ApplyDefaultDenyPolicy creates the default-deny CiliumNetworkPolicy
	// for a vCluster namespace. Must be called immediately after vCluster creation.
	ApplyDefaultDenyPolicy(ctx context.Context, vclusterNamespace, networkID string) error

	// ApplyIngressPolicy creates a policy allowing internet ingress to a specific
	// public subnet endpoint (used when provisioning an internet gateway).
	ApplyIngressPolicy(ctx context.Context, params IngressPolicyParams) error

	// RemovePolicy deletes a named CiliumNetworkPolicy.
	RemovePolicy(ctx context.Context, namespace, policyName string) error

	// GetPolicy returns the current state of a named policy.
	GetPolicy(ctx context.Context, namespace, policyName string) (PolicyInfo, error)
}

// New builds a [CiliumClient] from kubeconfig bytes for the management cluster.
func New(hostKubeconfig []byte) (CiliumClient, error) {
	return newCiliumClient(hostKubeconfig)
}
