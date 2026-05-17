package rest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	cfmiddleware "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/middleware"

	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/rest/generated"
	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/service"
)

// Handler implements generated.StrictServerInterface by delegating to [service.AccountsService].
type Handler struct {
	svc service.AccountsService
}

// NewHandler returns a REST handler backed by svc.
func NewHandler(svc service.AccountsService) *Handler {
	return &Handler{svc: svc}
}

func badRequest(ctx context.Context, message string) generated.BadRequestJSONResponse {
	return generated.BadRequestJSONResponse(withRequestID(ctx, generated.Error{
		Code:    string(cferrors.CodeInvalidInput),
		Message: message,
	}))
}

func (h *Handler) CreateAccount(ctx context.Context, request generated.CreateAccountRequestObject) (generated.CreateAccountResponseObject, error) {
	if request.Body == nil {
		return generated.CreateAccount400JSONResponse{BadRequestJSONResponse: badRequest(ctx, "request body is required")}, nil
	}
	params, err := ToServiceCreateAccountParams(request.Body)
	if err != nil {
		return generated.CreateAccount400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err.Error())}, nil
	}
	out, err := h.svc.CreateAccount(ctx, params)
	if err != nil {
		return mapCreateAccountError(ctx, err), nil
	}
	return generated.CreateAccount201JSONResponse(ToCreateAccountResultFromService(out)), nil
}

func mapCreateAccountError(ctx context.Context, err error) generated.CreateAccountResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusBadRequest:
		return generated.CreateAccount400JSONResponse{BadRequestJSONResponse: generated.BadRequestJSONResponse(body)}
	case http.StatusUnauthorized:
		return generated.CreateAccount401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.CreateAccount403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	case http.StatusConflict:
		return generated.CreateAccount409JSONResponse{ConflictJSONResponse: generated.ConflictJSONResponse(body)}
	default:
		return generated.CreateAccount500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) LoginWithPassword(ctx context.Context, request generated.LoginWithPasswordRequestObject) (generated.LoginWithPasswordResponseObject, error) {
	if request.Body == nil {
		return generated.LoginWithPassword400JSONResponse{BadRequestJSONResponse: badRequest(ctx, "request body is required")}, nil
	}
	params, err := ToServiceLoginWithPasswordParams(request.Body)
	if err != nil {
		return generated.LoginWithPassword400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err.Error())}, nil
	}
	acc, err := h.svc.LoginWithPassword(ctx, params)
	if err != nil {
		return mapLoginError(ctx, err), nil
	}
	return generated.LoginWithPassword200JSONResponse(ToAccountFromService(acc)), nil
}

func mapLoginError(ctx context.Context, err error) generated.LoginWithPasswordResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusBadRequest:
		return generated.LoginWithPassword400JSONResponse{BadRequestJSONResponse: generated.BadRequestJSONResponse(body)}
	case http.StatusUnauthorized:
		return generated.LoginWithPassword401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	default:
		return generated.LoginWithPassword500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) GetAccount(ctx context.Context, request generated.GetAccountRequestObject) (generated.GetAccountResponseObject, error) {
	acc, err := h.svc.GetAccount(ctx, request.AccountId.String())
	if err != nil {
		return mapGetAccountError(ctx, err), nil
	}
	return generated.GetAccount200JSONResponse(ToAccountFromService(acc)), nil
}

func mapGetAccountError(ctx context.Context, err error) generated.GetAccountResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.GetAccount401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.GetAccount403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	case http.StatusNotFound:
		return generated.GetAccount404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	default:
		return generated.GetAccount500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) ListAccounts(ctx context.Context, request generated.ListAccountsRequestObject) (generated.ListAccountsResponseObject, error) {
	limit := EffectiveLimit(request.Params.Limit)
	offset := EffectiveOffset(request.Params.Offset)
	items, total, err := h.svc.ListAccounts(ctx, limit, offset)
	if err != nil {
		return mapListAccountsError(ctx, err), nil
	}
	out := make([]generated.Account, 0, len(items))
	for _, a := range items {
		out = append(out, ToAccountFromService(a))
	}
	return generated.ListAccounts200JSONResponse(generated.AccountList{
		Items:  out,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}), nil
}

func mapListAccountsError(ctx context.Context, err error) generated.ListAccountsResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusBadRequest:
		return generated.ListAccounts400JSONResponse{BadRequestJSONResponse: generated.BadRequestJSONResponse(body)}
	case http.StatusUnauthorized:
		return generated.ListAccounts401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.ListAccounts403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	default:
		return generated.ListAccounts500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) DeleteAccount(ctx context.Context, request generated.DeleteAccountRequestObject) (generated.DeleteAccountResponseObject, error) {
	err := h.svc.SuspendAccount(ctx, request.AccountId.String())
	if err != nil {
		return mapDeleteAccountError(ctx, err), nil
	}
	return generated.DeleteAccount204Response{}, nil
}

func mapDeleteAccountError(ctx context.Context, err error) generated.DeleteAccountResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.DeleteAccount401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.DeleteAccount403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	case http.StatusNotFound:
		return generated.DeleteAccount404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	default:
		return generated.DeleteAccount500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) GetTenant(ctx context.Context, request generated.GetTenantRequestObject) (generated.GetTenantResponseObject, error) {
	t, err := h.svc.GetTenant(ctx, request.TenantId.String())
	if err != nil {
		return mapGetTenantError(ctx, err), nil
	}
	return generated.GetTenant200JSONResponse(ToTenantFromService(t)), nil
}

func mapGetTenantError(ctx context.Context, err error) generated.GetTenantResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.GetTenant401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.GetTenant403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	case http.StatusNotFound:
		return generated.GetTenant404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	default:
		return generated.GetTenant500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) ListTenants(ctx context.Context, request generated.ListTenantsRequestObject) (generated.ListTenantsResponseObject, error) {
	limit := EffectiveLimit(request.Params.Limit)
	offset := EffectiveOffset(request.Params.Offset)
	items, err := h.svc.ListTenants(ctx, request.AccountId.String(), limit, offset)
	if err != nil {
		return mapListTenantsError(ctx, err), nil
	}
	out := make([]generated.Tenant, 0, len(items))
	for _, t := range items {
		out = append(out, ToTenantFromService(t))
	}
	total := offset + len(out)
	return generated.ListTenants200JSONResponse(generated.TenantList{
		Items:  out,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}), nil
}

func mapListTenantsError(ctx context.Context, err error) generated.ListTenantsResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusBadRequest:
		return generated.ListTenants500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	case http.StatusUnauthorized:
		return generated.ListTenants401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.ListTenants403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	case http.StatusNotFound:
		return generated.ListTenants404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	default:
		return generated.ListTenants500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) CreateNetwork(ctx context.Context, request generated.CreateNetworkRequestObject) (generated.CreateNetworkResponseObject, error) {
	params, err := ToServiceCreateNetworkParams(request.TenantId.String(), request.Body)
	if err != nil {
		return generated.CreateNetwork400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err.Error())}, nil
	}
	n, err := h.svc.CreateNetwork(ctx, params)
	if err != nil {
		return mapCreateNetworkError(ctx, err), nil
	}
	return generated.CreateNetwork201JSONResponse(ToNetworkFromService(n)), nil
}

func mapCreateNetworkError(ctx context.Context, err error) generated.CreateNetworkResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusBadRequest:
		return generated.CreateNetwork400JSONResponse{BadRequestJSONResponse: generated.BadRequestJSONResponse(body)}
	case http.StatusUnauthorized:
		return generated.CreateNetwork401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.CreateNetwork403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	case http.StatusNotFound:
		return generated.CreateNetwork404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	case http.StatusConflict:
		return generated.CreateNetwork409JSONResponse{ConflictJSONResponse: generated.ConflictJSONResponse(body)}
	default:
		return generated.CreateNetwork500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) GetNetwork(ctx context.Context, request generated.GetNetworkRequestObject) (generated.GetNetworkResponseObject, error) {
	n, err := h.svc.GetNetwork(ctx, request.NetworkId.String())
	if err != nil {
		return mapGetNetworkError(ctx, err), nil
	}
	return generated.GetNetwork200JSONResponse(ToNetworkFromService(n)), nil
}

func mapGetNetworkError(ctx context.Context, err error) generated.GetNetworkResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.GetNetwork401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.GetNetwork403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	case http.StatusNotFound:
		return generated.GetNetwork404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	default:
		return generated.GetNetwork500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) ListNetworks(ctx context.Context, request generated.ListNetworksRequestObject) (generated.ListNetworksResponseObject, error) {
	limit := EffectiveLimit(request.Params.Limit)
	offset := EffectiveOffset(request.Params.Offset)
	all, err := h.svc.ListNetworks(ctx, request.TenantId.String())
	if err != nil {
		return mapListNetworksError(ctx, err), nil
	}
	page := SlicePage(all, offset, limit)
	out := make([]generated.Network, 0, len(page))
	for _, n := range page {
		out = append(out, ToNetworkFromService(n))
	}
	total := offset + len(out)
	return generated.ListNetworks200JSONResponse(generated.NetworkList{
		Items:  out,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}), nil
}

func mapListNetworksError(ctx context.Context, err error) generated.ListNetworksResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusBadRequest:
		return generated.ListNetworks500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	case http.StatusUnauthorized:
		return generated.ListNetworks401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.ListNetworks403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	case http.StatusNotFound:
		return generated.ListNetworks404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	default:
		return generated.ListNetworks500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) DeleteNetwork(ctx context.Context, request generated.DeleteNetworkRequestObject) (generated.DeleteNetworkResponseObject, error) {
	err := h.svc.DeprovisionNetwork(ctx, request.NetworkId.String())
	if err != nil {
		return mapDeleteNetworkError(ctx, err), nil
	}
	return generated.DeleteNetwork204Response{}, nil
}

func mapDeleteNetworkError(ctx context.Context, err error) generated.DeleteNetworkResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.DeleteNetwork401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.DeleteNetwork403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	case http.StatusNotFound:
		return generated.DeleteNetwork404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	default:
		return generated.DeleteNetwork500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) CreateCredential(ctx context.Context, request generated.CreateCredentialRequestObject) (generated.CreateCredentialResponseObject, error) {
	c, err := h.svc.CreateCredential(ctx, service.CreateCredentialParams{AccountID: request.AccountId.String()})
	if err != nil {
		return mapCreateCredentialError(ctx, err), nil
	}
	return generated.CreateCredential201JSONResponse(ToCredentialCreatedFromService(c)), nil
}

func mapCreateCredentialError(ctx context.Context, err error) generated.CreateCredentialResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusBadRequest:
		return generated.CreateCredential500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	case http.StatusUnauthorized:
		return generated.CreateCredential401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.CreateCredential403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	case http.StatusNotFound:
		return generated.CreateCredential404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	default:
		return generated.CreateCredential500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) ListCredentials(ctx context.Context, request generated.ListCredentialsRequestObject) (generated.ListCredentialsResponseObject, error) {
	limit := EffectiveLimit(request.Params.Limit)
	offset := EffectiveOffset(request.Params.Offset)
	all, err := h.svc.ListCredentials(ctx, request.AccountId.String())
	if err != nil {
		return mapListCredentialsError(ctx, err), nil
	}
	page := SlicePage(all, offset, limit)
	out := make([]generated.CredentialMeta, 0, len(page))
	for _, m := range page {
		out = append(out, ToCredentialMetaFromService(m))
	}
	total := offset + len(out)
	return generated.ListCredentials200JSONResponse(generated.CredentialList{
		Items:  out,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}), nil
}

func mapListCredentialsError(ctx context.Context, err error) generated.ListCredentialsResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusBadRequest:
		return generated.ListCredentials500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	case http.StatusUnauthorized:
		return generated.ListCredentials401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.ListCredentials403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	case http.StatusNotFound:
		return generated.ListCredentials404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	default:
		return generated.ListCredentials500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) RevokeCredential(ctx context.Context, request generated.RevokeCredentialRequestObject) (generated.RevokeCredentialResponseObject, error) {
	err := h.svc.RevokeCredential(ctx, request.KeyId.String())
	if err != nil {
		return mapRevokeCredentialError(ctx, err), nil
	}
	return generated.RevokeCredential204Response{}, nil
}

func mapRevokeCredentialError(ctx context.Context, err error) generated.RevokeCredentialResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.RevokeCredential401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusForbidden:
		return generated.RevokeCredential403JSONResponse{ForbiddenJSONResponse: generated.ForbiddenJSONResponse(body)}
	case http.StatusNotFound:
		return generated.RevokeCredential404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	default:
		return generated.RevokeCredential500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) ResolveTenant(ctx context.Context, request generated.ResolveTenantRequestObject) (generated.ResolveTenantResponseObject, error) {
	params, err := ToServiceResolveTenantParams(request.Params)
	if err != nil {
		return generated.ResolveTenant400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err.Error())}, nil
	}
	tc, err := h.svc.ResolveTenantContext(ctx, params)
	if err != nil {
		return mapResolveTenantError(ctx, err), nil
	}
	return generated.ResolveTenant200JSONResponse(ToTenantContextFromService(tc)), nil
}

func mapResolveTenantError(ctx context.Context, err error) generated.ResolveTenantResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusBadRequest:
		return generated.ResolveTenant400JSONResponse{BadRequestJSONResponse: generated.BadRequestJSONResponse(body)}
	case http.StatusUnauthorized:
		return generated.ResolveTenant401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusNotFound:
		return generated.ResolveTenant404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	default:
		return generated.ResolveTenant500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

// Compile-time check that Handler implements the strict interface.
var _ generated.StrictServerInterface = (*Handler)(nil)

// JSONDecodeError writes a JSON problem body for invalid JSON (used by strict server options).
func JSONDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(withRequestID(ctx, generated.Error{
		Code:    string(cferrors.CodeInvalidInput),
		Message: "invalid JSON: " + err.Error(),
	}))
}

// JSONEncodeError handles failures writing JSON success bodies.
func JSONEncodeError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	slog.ErrorContext(ctx, "response encode error", "error", err, "request_id", cfmiddleware.RequestIDFromContext(ctx))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(withRequestID(ctx, generated.Error{
		Code:    string(cferrors.CodeInternal),
		Message: "failed to encode response",
	}))
}
