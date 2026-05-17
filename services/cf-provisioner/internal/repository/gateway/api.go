package gateway

import "context"

// GatewayClient manages HTTPRoute objects for Envoy Gateway (Gateway API).
type GatewayClient interface {
	// CreateHTTPRoute creates an HTTPRoute for a tenant's public endpoint.
	CreateHTTPRoute(ctx context.Context, params HTTPRouteParams) (HTTPRouteInfo, error)

	// GetHTTPRoute returns the current status of an HTTPRoute.
	GetHTTPRoute(ctx context.Context, namespace, name string) (HTTPRouteInfo, error)

	// DeleteHTTPRoute removes an HTTPRoute.
	DeleteHTTPRoute(ctx context.Context, namespace, name string) error
}

// New builds a [GatewayClient] from kubeconfig bytes for the management cluster.
func New(hostKubeconfig []byte) (GatewayClient, error) {
	return newGatewayClient(hostKubeconfig)
}
