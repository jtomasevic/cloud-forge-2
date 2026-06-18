package gateway

type HTTPRouteParams struct {
	Name           string
	Namespace      string
	Hostname       string // e.g. "tenant-abc.gateway.cloudforge.io"
	BackendService string
	// BackendNamespace is optional; when empty Gateway API resolves the backend
	// Service in the HTTPRoute namespace. Cross-namespace backends require the
	// target namespace to grant the reference separately.
	BackendNamespace string
	BackendPort      int
	TLSEnabled       bool
	Labels           map[string]string
	Rules            []HTTPRouteRuleParams
}

type HTTPRoutePathMatchType string

const (
	HTTPRoutePathMatchExact  HTTPRoutePathMatchType = "Exact"
	HTTPRoutePathMatchPrefix HTTPRoutePathMatchType = "PathPrefix"
)

type HTTPRouteRuleParams struct {
	Path             string
	PathType         HTTPRoutePathMatchType
	BackendService   string
	BackendNamespace string
	BackendPort      int
}

// AppServiceHTTPRouteParams creates the public HTTPRoute for one CF App Service exposure.
//
// Service traffic, Swagger UI, and OpenAPI JSON are modeled as separate HTTPRoute rules so Task 34
// can move documentation paths to a CloudForge docs adapter without changing the public route name.
type AppServiceHTTPRouteParams struct {
	Namespace        string
	AppServiceID     string
	Hostname         string
	BackendService   string
	BackendNamespace string
	BackendPort      int
	TLSEnabled       bool
	ServicePath      string
	SwaggerPath      string
	OpenAPIPath      string
	DocsBackend      *HTTPRouteBackend
}

type HTTPRouteBackend struct {
	Service   string
	Namespace string
	Port      int
}

type HTTPRouteStatus string

const (
	HTTPRouteStatusPending HTTPRouteStatus = "pending"
	HTTPRouteStatusReady   HTTPRouteStatus = "ready"
	HTTPRouteStatusFailed  HTTPRouteStatus = "failed"
)

type HTTPRouteInfo struct {
	Name      string
	Namespace string
	Hostname  string
	Status    HTTPRouteStatus
}
