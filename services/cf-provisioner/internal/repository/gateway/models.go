package gateway

type HTTPRouteParams struct {
	Name           string
	Namespace      string
	Hostname       string // e.g. "tenant-abc.gateway.cloudforge.io"
	BackendService string
	BackendPort    int
	TLSEnabled     bool
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
