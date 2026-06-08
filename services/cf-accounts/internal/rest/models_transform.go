package rest

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/rest/generated"
	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/service"
)

// ToServiceCreateAccountParams maps the OpenAPI signup body to service parameters.
func ToServiceCreateAccountParams(body *generated.CreateAccountRequest) (service.CreateAccountParams, error) {
	if body == nil {
		return service.CreateAccountParams{}, fmt.Errorf("request body is required")
	}
	pw := strings.TrimSpace(body.Password)
	if pw == "" {
		return service.CreateAccountParams{}, fmt.Errorf("password is required")
	}
	return service.CreateAccountParams{
		Email:    string(body.Email),
		Password: pw,
	}, nil
}

// ToServiceLoginWithPasswordParams maps the login body to service parameters.
func ToServiceLoginWithPasswordParams(body *generated.LoginRequest) (service.LoginWithPasswordParams, error) {
	if body == nil {
		return service.LoginWithPasswordParams{}, fmt.Errorf("request body is required")
	}
	pw := strings.TrimSpace(body.Password)
	if pw == "" {
		return service.LoginWithPasswordParams{}, fmt.Errorf("password is required")
	}
	return service.LoginWithPasswordParams{
		Email:    string(body.Email),
		Password: pw,
	}, nil
}

// ToCreateAccountResultFromService maps signup result to the OpenAPI response envelope.
func ToCreateAccountResultFromService(r service.CreateAccountResult) generated.CreateAccountResult {
	return generated.CreateAccountResult{
		Account:       ToAccountFromService(r.Account),
		DefaultTenant: ToTenantFromService(r.DefaultTenant),
	}
}

// ToLoginResponseFromService maps a service login result to the OpenAPI token response.
func ToLoginResponseFromService(r service.LoginResult) generated.LoginResponse {
	out := generated.LoginResponse{
		AccessToken: r.AccessToken,
		Account:     ToAccountFromService(r.Account),
		ExpiresIn:   int32(r.ExpiresIn),
		TokenType:   r.TokenType,
	}
	if r.RefreshToken != "" {
		out.RefreshToken = &r.RefreshToken
	}
	if r.RefreshExpiresIn > 0 {
		v := int32(r.RefreshExpiresIn)
		out.RefreshExpiresIn = &v
	}
	if r.IDToken != "" {
		out.IdToken = &r.IDToken
	}
	if r.Scope != "" {
		out.Scope = &r.Scope
	}
	return out
}

// ToAccountFromService maps a service account to the OpenAPI Account model.
func ToAccountFromService(a service.Account) generated.Account {
	uid := uuid.MustParse(a.ID)
	email := openapi_types.Email(a.Email)
	up := a.UpdatedAt
	return generated.Account{
		Id:        openapi_types.UUID(uid),
		Email:     email,
		Status:    generated.AccountStatus(a.Status),
		CreatedAt: a.CreatedAt.UTC(),
		UpdatedAt: &up,
	}
}

// ToTenantFromService maps a service tenant to the OpenAPI Tenant model.
func ToTenantFromService(t service.Tenant) generated.Tenant {
	tid := uuid.MustParse(t.ID)
	aid := uuid.MustParse(t.AccountID)
	up := t.UpdatedAt
	return generated.Tenant{
		Id:        openapi_types.UUID(tid),
		AccountId: openapi_types.UUID(aid),
		Slug:      t.Slug,
		Status:    generated.TenantStatus(t.Status),
		CreatedAt: t.CreatedAt.UTC(),
		UpdatedAt: &up,
	}
}

// ToNetworkFromService maps a service network to the OpenAPI Network model.
func ToNetworkFromService(n service.Network) generated.Network {
	nid := uuid.MustParse(n.ID)
	tid := uuid.MustParse(n.TenantID)
	up := n.UpdatedAt
	return generated.Network{
		Id:        openapi_types.UUID(nid),
		TenantId:  openapi_types.UUID(tid),
		Region:    n.Region,
		PodCIDR:   n.PodCIDR,
		SvcCIDR:   n.SvcCIDR,
		Status:    generated.NetworkStatus(n.Status),
		CreatedAt: n.CreatedAt.UTC(),
		UpdatedAt: &up,
	}
}

// ToCredentialMetaFromService maps credential metadata to the OpenAPI model.
func ToCredentialMetaFromService(m service.CredentialMeta) generated.CredentialMeta {
	id := uuid.MustParse(m.ID)
	aid := uuid.MustParse(m.AccountID)
	out := generated.CredentialMeta{
		Id:        openapi_types.UUID(id),
		AccountId: openapi_types.UUID(aid),
		Prefix:    m.KeyPrefix,
		CreatedAt: m.CreatedAt.UTC(),
	}
	if m.RevokedAt != nil {
		rt := m.RevokedAt.UTC()
		out.RevokedAt = &rt
	}
	return out
}

// ToCredentialCreatedFromService maps the one-time credential creation payload.
func ToCredentialCreatedFromService(c service.CredentialCreated) generated.CredentialCreated {
	meta := ToCredentialMetaFromService(c.CredentialMeta)
	return generated.CredentialCreated{
		Id:        meta.Id,
		AccountId: meta.AccountId,
		Prefix:    meta.Prefix,
		CreatedAt: meta.CreatedAt,
		RevokedAt: meta.RevokedAt,
		Secret:    c.Secret,
	}
}

// ToTenantContextFromService maps internal router context to the OpenAPI TenantContext.
func ToTenantContextFromService(tc service.TenantContext) generated.TenantContext {
	tid := uuid.MustParse(tc.TenantID)
	aid := uuid.MustParse(tc.AccountID)
	out := generated.TenantContext{
		TenantId:  openapi_types.UUID(tid),
		AccountId: openapi_types.UUID(aid),
		Status:    tc.Status,
	}
	if tc.NetworkID != "" {
		nid := uuid.MustParse(tc.NetworkID)
		u := openapi_types.UUID(nid)
		out.NetworkId = &u
	}
	return out
}

// ToServiceCreateNetworkParams maps create-network JSON to service params (v1 stores region only;
// pod and service CIDRs from the request are accepted at the API but not yet persisted by the service).
func ToServiceCreateNetworkParams(tenantID string, body *generated.CreateNetworkRequest) (service.CreateNetworkParams, error) {
	if body == nil {
		return service.CreateNetworkParams{}, fmt.Errorf("request body is required")
	}
	return service.CreateNetworkParams{
		TenantID: tenantID,
		Region:   body.Region,
	}, nil
}

// ToServiceResolveTenantParams builds resolution input from query parameters.
func ToServiceResolveTenantParams(p generated.ResolveTenantParams) (service.ResolveTenantParams, error) {
	hasAccount := p.AccountId != nil && *p.AccountId != uuid.Nil
	hasHash := p.ApiKeyHash != nil && strings.TrimSpace(*p.ApiKeyHash) != ""

	switch {
	case hasAccount && hasHash:
		return service.ResolveTenantParams{}, fmt.Errorf("provide only one of accountId or apiKeyHash")
	case !hasAccount && !hasHash:
		return service.ResolveTenantParams{}, fmt.Errorf("exactly one of accountId or apiKeyHash is required")
	case hasHash:
		return service.ResolveTenantParams{APIKeyHash: strings.TrimSpace(*p.ApiKeyHash)}, nil
	default:
		id := uuid.UUID(*p.AccountId)
		return service.ResolveTenantParams{AccountID: id.String()}, nil
	}
}

// EffectiveLimit returns a positive page size capped at 100 (OpenAPI default 20).
func EffectiveLimit(p *generated.Limit) int {
	const def = 20
	if p == nil {
		return def
	}
	v := int(*p)
	if v < 1 {
		return def
	}
	if v > 100 {
		return 100
	}
	return v
}

// EffectiveOffset returns a non-negative offset (OpenAPI default 0).
func EffectiveOffset(p *generated.Offset) int {
	if p == nil {
		return 0
	}
	v := int(*p)
	if v < 0 {
		return 0
	}
	return v
}

// SlicePage returns items[offset : offset+limit] with safe bounds for list endpoints
// where the service returns the full set (e.g. networks, credentials v1).
func SlicePage[T any](items []T, offset, limit int) []T {
	if offset >= len(items) {
		return nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	out := make([]T, end-offset)
	copy(out, items[offset:end])
	return out
}

// PtrTimeUTC returns a pointer to t in UTC (for optional OpenAPI timestamps).
func PtrTimeUTC(t time.Time) *time.Time {
	utc := t.UTC()
	return &utc
}
