package appservices

import "time"

type AppServiceStatus string

const (
	AppServiceStatusCreating AppServiceStatus = "creating"
	AppServiceStatusRunning  AppServiceStatus = "running"
	AppServiceStatusUpdating AppServiceStatus = "updating"
	AppServiceStatusDeleting AppServiceStatus = "deleting"
	AppServiceStatusFailed   AppServiceStatus = "failed"

	// AppServiceStatusDeleted is repository-internal tombstone state. The public OpenAPI status enum
	// stops at "deleting"; callers normally delete the row after cleanup finishes.
	AppServiceStatusDeleted AppServiceStatus = "deleted"
)

type AppServiceExposureType string

const (
	AppServiceExposureTypeInternetGateway AppServiceExposureType = "InternetGateway"
)

type AppServiceExposureStatus string

const (
	AppServiceExposureStatusPending  AppServiceExposureStatus = "pending"
	AppServiceExposureStatusActive   AppServiceExposureStatus = "active"
	AppServiceExposureStatusRemoving AppServiceExposureStatus = "removing"
	AppServiceExposureStatusRemoved  AppServiceExposureStatus = "removed"
	AppServiceExposureStatusFailed   AppServiceExposureStatus = "failed"
)

type CreateAppServiceParams struct {
	ID        string
	TenantID  string
	NetworkID string
	SubnetID  string
	Name      string
	Runtime   AppServiceRuntime
	Exposure  *AppServiceExposure
	Status    AppServiceStatus
}

type UpdateStatusParams struct {
	AppServiceID string
	Status       AppServiceStatus
}

type UpdateExposureParams struct {
	AppServiceID string
	Exposure     *AppServiceExposure
	Status       AppServiceExposureStatus
}

// AppService is the repository read model. It is deliberately separate from REST generated models
// and future service-layer models; it mirrors what can be reconstructed from Scylla rows.
type AppService struct {
	ID             string
	TenantID       string
	NetworkID      string
	SubnetID       string
	Name           string
	Status         AppServiceStatus
	Runtime        AppServiceRuntime
	Exposure       *AppServiceExposure
	ExposureStatus AppServiceExposureStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AppServiceRuntime struct {
	ServiceType string
	Image       string
	Build       *AppServiceBuild
	Command     []string
	Args        []string
	Resources   AppServiceResources
	Env         []AppServiceEnvVar
	Ports       []AppServicePort
	Replicas    int
}

type AppServiceBuild struct {
	Context    string
	Dockerfile string
}

type AppServiceResources struct {
	CPU    string
	Memory string
}

type AppServiceEnvVar struct {
	Name      string
	Value     string
	SecretRef string
}

type AppServicePort struct {
	Name          string
	ContainerPort int
	Protocol      string
}

type AppServiceExposure struct {
	Type       AppServiceExposureType
	Host       string
	PortRef    string
	TLSEnabled bool
	Swagger    AppServiceSwagger
}

type AppServiceSwagger struct {
	SpecURL     string
	InlineSpec  map[string]any
	PublicPath  string
	OpenAPIPath string
}
