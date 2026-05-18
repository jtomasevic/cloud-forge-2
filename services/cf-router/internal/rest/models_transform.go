// DTO mapping between service models and generated OpenAPI types (see package [rest] doc in doc.go).
package rest

import (
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/rest/generated"
	"github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/service"
)

// ToTenantContextResponse maps the service-layer [service.TenantContext] into the wire shape expected
// by the CF-Router OpenAPI schema ([generated.TenantContextResponse]).
//
// UUID fields are validated here: invalid IDs from upstream are treated as mapping errors and become
// 500s at the REST layer.
func ToTenantContextResponse(t service.TenantContext) (generated.TenantContextResponse, error) {
	tid, err := uuid.Parse(t.TenantID)
	if err != nil {
		return generated.TenantContextResponse{}, err
	}
	aid, err := uuid.Parse(t.AccountID)
	if err != nil {
		return generated.TenantContextResponse{}, err
	}
	out := generated.TenantContextResponse{
		TenantId:  openapi_types.UUID(tid),
		AccountId: openapi_types.UUID(aid),
		Region:    t.Region,
		Status:    t.Status,
	}
	if t.NetworkID != "" {
		nid, err := uuid.Parse(t.NetworkID)
		if err != nil {
			return generated.TenantContextResponse{}, err
		}
		u := openapi_types.UUID(nid)
		out.NetworkId = &u
	}
	switch t.ResolvedVia {
	case service.AuthMethodJWT:
		v := generated.Jwt
		out.ResolvedVia = &v
	case service.AuthMethodAPIKey:
		v := generated.ApiKey
		out.ResolvedVia = &v
	}
	return out, nil
}
