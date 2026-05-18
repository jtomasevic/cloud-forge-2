package service

import (
	"context"
	"crypto/rsa"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"

	cfaccountsclient "github.com/jtomasevic/cloud-forge-2/libs/clients/cf-accounts/v1"
	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	apikeysrepo "github.com/jtomasevic/cloud-forge-2/services/cf-router/internal/repository/apikeys"
	"golang.org/x/crypto/blake2b"
)

// cfRouterService implements [RouterService]. JWKS keys are cached under jwksMu.
type cfRouterService struct {
	deps Deps

	jwksMu    sync.Mutex
	jwksByKid map[string]*rsa.PublicKey
}

// httpClient returns the configured HTTP client or the default.
func (s *cfRouterService) httpClient() *http.Client {
	if s.deps.HTTPClient != nil {
		return s.deps.HTTPClient
	}
	return http.DefaultClient
}

// internalSecretEditor returns an oapi-codegen request editor that injects X-CF-Internal-Secret
// on outbound CF-Accounts calls (required for /internal/v1/resolve).
func (s *cfRouterService) internalSecretEditor() cfaccountsclient.RequestEditorFn {
	secret := strings.TrimSpace(s.deps.InternalSecret)
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("X-CF-Internal-Secret", secret)
		return nil
	}
}

// hashAPIKey computes the BLAKE2b-256 digest of the raw API key and returns lowercase hex.
// This must match CF-Accounts / migrations: api_keys_by_hash.key_hash stores the same digest.
func hashAPIKey(raw string) string {
	h, _ := blake2b.New256(nil)
	_, _ = h.Write([]byte(raw))
	return hex.EncodeToString(h.Sum(nil))
}

// resolveTenantViaAccounts calls CF-Accounts GET /internal/v1/resolve?accountId=… with the mesh secret.
// On success it maps JSON200 into [TenantContext] (including optional region lookup).
func (s *cfRouterService) resolveTenantViaAccounts(ctx context.Context, accountID string, via AuthMethod, params ValidateParams) (TenantContext, error) {
	aid, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return TenantContext{}, ErrTenantResolution
	}
	resp, err := s.deps.AccountsClient.ResolveTenantWithResponse(ctx, &cfaccountsclient.ResolveTenantParams{
		AccountId: &aid,
	}, s.internalSecretEditor())
	if err != nil {
		if isDialFailure(err) {
			return TenantContext{}, cferrors.Wrapf(ErrAccountsUnreachable, "resolve tenant: %v", err)
		}
		return TenantContext{}, err
	}
	switch resp.StatusCode() {
	case http.StatusOK:
		if resp.JSON200 == nil {
			return TenantContext{}, ErrTenantResolution
		}
		return s.tenantContextFromResolve(ctx, resp.JSON200, via, params)
	case http.StatusUnauthorized, http.StatusNotFound:
		return TenantContext{}, ErrTenantResolution
	default:
		if resp.StatusCode() == 0 {
			return TenantContext{}, ErrTenantResolution
		}
		if resp.StatusCode() >= http.StatusInternalServerError {
			return TenantContext{}, cferrors.Wrapf(ErrAccountsUnreachable, "cf-accounts returned %d", resp.StatusCode())
		}
		return TenantContext{}, ErrTenantResolution
	}
}

// tenantContextFromResolve maps the CF-Accounts wire model into our [TenantContext].
// Region is best-effort: only populated for Bearer callers when GetNetwork succeeds.
func (s *cfRouterService) tenantContextFromResolve(ctx context.Context, tc *cfaccountsclient.TenantContext, via AuthMethod, params ValidateParams) (TenantContext, error) {
	out := TenantContext{
		TenantID:    tc.TenantId.String(),
		AccountID:   tc.AccountId.String(),
		Status:      tc.Status,
		ResolvedVia: via,
	}
	if tc.NetworkId != nil {
		out.NetworkID = tc.NetworkId.String()
	}
	if out.NetworkID != "" && strings.HasPrefix(strings.TrimSpace(params.AuthorizationHeader), "Bearer ") {
		nid := *tc.NetworkId
		gn, err := s.deps.AccountsClient.GetNetworkWithResponse(ctx, nid, func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", strings.TrimSpace(params.AuthorizationHeader))
			return nil
		})
		if err == nil && gn.StatusCode() == http.StatusOK && gn.JSON200 != nil {
			out.Region = strings.TrimSpace(gn.JSON200.Region)
		}
	}
	return out, nil
}

// isDialFailure classifies low-level transport failures (timeouts, connection refused, etc.).
func isDialFailure(err error) bool {
	if err == nil {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return false
}

// ValidateAndResolve implements [RouterService.ValidateAndResolve].
//
// Precedence: if Authorization looks like Bearer, JWT path wins (even if X-CF-API-Key is also set).
// Otherwise if X-CF-API-Key is non-empty, API key path. Else [ErrUnauthenticated].
func (s *cfRouterService) ValidateAndResolve(ctx context.Context, params ValidateParams) (TenantContext, error) {
	authz := strings.TrimSpace(params.AuthorizationHeader)
	apiKey := strings.TrimSpace(params.APIKeyHeader)

	if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		raw := strings.TrimSpace(authz[len("Bearer "):])
		if raw == "" {
			return TenantContext{}, ErrUnauthenticated
		}
		acct, err := verifyRS256JWT(ctx, s, raw)
		if err != nil {
			return TenantContext{}, cferrors.Wrapf(ErrJWTInvalid, "%v", err)
		}
		return s.resolveTenantViaAccounts(ctx, acct, AuthMethodJWT, params)
	}

	if apiKey != "" {
		rec, err := s.deps.APIKeys.GetByHash(ctx, hashAPIKey(apiKey))
		if err != nil {
			if errors.Is(err, apikeysrepo.ErrKeyRevoked) {
				return TenantContext{}, err
			}
			if errors.Is(err, apikeysrepo.ErrKeyNotFound) {
				return TenantContext{}, ErrTenantResolution
			}
			return TenantContext{}, err
		}
		return s.resolveTenantViaAccounts(ctx, rec.AccountID, AuthMethodAPIKey, params)
	}

	return TenantContext{}, ErrUnauthenticated
}

// Ready implements [RouterService.Ready].
//
// We intentionally call resolve with **invalid** parameters (both accountId and apiKeyHash empty).
// CF-Accounts should answer 400 when reachable + secret is valid — that proves HTTP + auth wiring.
// 401 implies wrong CF_INTERNAL_SECRET configuration. Network errors map to [ErrAccountsUnreachable].
func (s *cfRouterService) Ready(ctx context.Context) error {
	resp, err := s.deps.AccountsClient.ResolveTenantWithResponse(ctx, &cfaccountsclient.ResolveTenantParams{}, s.internalSecretEditor())
	if err != nil {
		if isDialFailure(err) {
			return cferrors.Wrapf(ErrAccountsUnreachable, "ready check: %v", err)
		}
		return err
	}
	switch resp.StatusCode() {
	case http.StatusBadRequest:
		return nil
	case http.StatusUnauthorized:
		return cferrors.New(cferrors.CodeUnavailable, "cf-accounts rejected X-CF-Internal-Secret")
	default:
		if resp.StatusCode() >= http.StatusInternalServerError {
			return cferrors.Wrapf(ErrAccountsUnreachable, "cf-accounts returned %d", resp.StatusCode())
		}
		return nil
	}
}
