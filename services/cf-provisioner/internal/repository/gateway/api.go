package gateway

import "context"

// GatewayClient manages HTTPRoute objects for Envoy Gateway (Gateway API).
type GatewayClient interface {
	// CreateHTTPRoute creates an HTTPRoute for a tenant's public endpoint.
	CreateHTTPRoute(ctx context.Context, params HTTPRouteParams) (HTTPRouteInfo, error)

	// CreateAppServiceHTTPRoute creates or updates the service-specific public
	// HTTPRoute for a CF App Service exposure. The route name is derived from
	// the app service ID so exposure retries and deletes target the same object.
	CreateAppServiceHTTPRoute(ctx context.Context, params AppServiceHTTPRouteParams) (HTTPRouteInfo, error)

	// GetHTTPRoute returns the current status of an HTTPRoute.
	GetHTTPRoute(ctx context.Context, namespace, name string) (HTTPRouteInfo, error)

	// DeleteHTTPRoute removes an HTTPRoute.
	DeleteHTTPRoute(ctx context.Context, namespace, name string) error

	// DeleteAppServiceHTTPRoute removes the service-specific route by app service ID.
	DeleteAppServiceHTTPRoute(ctx context.Context, namespace, appServiceID string) error
}

// New builds a [GatewayClient] from kubeconfig bytes for the management cluster.
func New(hostKubeconfig []byte) (GatewayClient, error) {
	return newGatewayClient(hostKubeconfig)
}
