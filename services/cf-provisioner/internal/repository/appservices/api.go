// Package appservices owns durable ScyllaDB records for CloudForge application services.
//
// This package persists workload intent, placement, status, exposure, and public Swagger/OpenAPI
// metadata. It deliberately does not validate tenant authorization and does not create Kubernetes,
// Gateway API, or Cilium resources; those responsibilities belong to the service and infrastructure
// repository layers.
package appservices

import (
	"context"

	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

type AppServicesRepository interface {
	Create(ctx context.Context, params CreateAppServiceParams) (AppService, error)
	Get(ctx context.Context, appServiceID string) (AppService, error)
	ListByNetwork(ctx context.Context, networkID string) ([]AppService, error)
	UpdateStatus(ctx context.Context, params UpdateStatusParams) (AppService, error)
	UpdateExposure(ctx context.Context, params UpdateExposureParams) (AppService, error)
	MarkDeleted(ctx context.Context, appServiceID string) (AppService, error)
	Delete(ctx context.Context, appServiceID string) error
}

func New(session *scylladbclient.Session) AppServicesRepository {
	return newWithStore(sessionStore{session: session})
}
