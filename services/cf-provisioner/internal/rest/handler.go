package rest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	cfmiddleware "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/middleware"

	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/rest/generated"
	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/service"
)

// Handler implements generated.StrictServerInterface by delegating to [service.ProvisionerService].
type Handler struct {
	svc service.ProvisionerService
}

// NewHandler returns a REST handler backed by svc.
func NewHandler(svc service.ProvisionerService) *Handler {
	return &Handler{svc: svc}
}

func badRequest(ctx context.Context, message string) generated.BadRequestJSONResponse {
	return generated.BadRequestJSONResponse(withRequestID(ctx, generated.Error{
		Code:    string(cferrors.CodeInvalidInput),
		Message: message,
	}))
}

func (h *Handler) ListCIDRAllocations(ctx context.Context, request generated.ListCIDRAllocationsRequestObject) (generated.ListCIDRAllocationsResponseObject, error) {
	limit := EffectiveLimit(request.Params.Limit)
	offset := EffectiveOffset(request.Params.Offset)
	rows, err := h.svc.ListCIDRAllocations(ctx, limit, offset)
	if err != nil {
		return mapListCIDRAllocationsError(ctx, err), nil
	}
	items := make([]generated.CIDRAllocation, 0, len(rows))
	for i := range rows {
		a, err := ToCIDRAllocationFromRepo(rows[i])
		if err != nil {
			return generated.ListCIDRAllocations500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(
				withRequestID(ctx, generated.Error{Code: "INTERNAL", Message: err.Error()}),
			)}, nil
		}
		items = append(items, a)
	}
	return generated.ListCIDRAllocations200JSONResponse(generated.CIDRAllocationList{Items: items}), nil
}

func mapListCIDRAllocationsError(ctx context.Context, err error) generated.ListCIDRAllocationsResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.ListCIDRAllocations401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusUnprocessableEntity:
		return generated.ListCIDRAllocations422JSONResponse{UnprocessableEntityJSONResponse: generated.UnprocessableEntityJSONResponse(body)}
	default:
		return generated.ListCIDRAllocations500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) GetJob(ctx context.Context, request generated.GetJobRequestObject) (generated.GetJobResponseObject, error) {
	job, err := h.svc.GetJob(ctx, request.JobId.String())
	if err != nil {
		return mapGetJobError(ctx, err), nil
	}
	out, err := ToJobFromService(job)
	if err != nil {
		return generated.GetJob500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(
			withRequestID(ctx, generated.Error{Code: "INTERNAL", Message: err.Error()}),
		)}, nil
	}
	return generated.GetJob200JSONResponse(out), nil
}

func mapGetJobError(ctx context.Context, err error) generated.GetJobResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.GetJob401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusNotFound:
		return generated.GetJob404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	case http.StatusUnprocessableEntity:
		return generated.GetJob422JSONResponse{UnprocessableEntityJSONResponse: generated.UnprocessableEntityJSONResponse(body)}
	default:
		return generated.GetJob500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) ProvisionNetwork(ctx context.Context, request generated.ProvisionNetworkRequestObject) (generated.ProvisionNetworkResponseObject, error) {
	if request.Body == nil {
		return generated.ProvisionNetwork400JSONResponse{BadRequestJSONResponse: badRequest(ctx, "request body is required")}, nil
	}
	params, err := ToServiceProvisionNetworkParams(request.Body)
	if err != nil {
		return generated.ProvisionNetwork400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err.Error())}, nil
	}
	job, err := h.svc.ProvisionNetwork(ctx, params)
	if err != nil {
		return mapProvisionNetworkError(ctx, err), nil
	}
	out, err := ToJobFromService(job)
	if err != nil {
		return generated.ProvisionNetwork500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(
			withRequestID(ctx, generated.Error{Code: "INTERNAL", Message: err.Error()}),
		)}, nil
	}
	return generated.ProvisionNetwork202JSONResponse(out), nil
}

func mapProvisionNetworkError(ctx context.Context, err error) generated.ProvisionNetworkResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusBadRequest:
		return generated.ProvisionNetwork400JSONResponse{BadRequestJSONResponse: generated.BadRequestJSONResponse(body)}
	case http.StatusUnauthorized:
		return generated.ProvisionNetwork401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusConflict:
		return generated.ProvisionNetwork409JSONResponse{ConflictJSONResponse: generated.ConflictJSONResponse(body)}
	case http.StatusUnprocessableEntity:
		return generated.ProvisionNetwork422JSONResponse{UnprocessableEntityJSONResponse: generated.UnprocessableEntityJSONResponse(body)}
	default:
		return generated.ProvisionNetwork500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) DeprovisionNetwork(ctx context.Context, request generated.DeprovisionNetworkRequestObject) (generated.DeprovisionNetworkResponseObject, error) {
	job, err := h.svc.DeprovisionNetwork(ctx, request.NetworkId.String())
	if err != nil {
		return mapDeprovisionNetworkError(ctx, err), nil
	}
	out, err := ToJobFromService(job)
	if err != nil {
		return generated.DeprovisionNetwork500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(
			withRequestID(ctx, generated.Error{Code: "INTERNAL", Message: err.Error()}),
		)}, nil
	}
	return generated.DeprovisionNetwork202JSONResponse(out), nil
}

func mapDeprovisionNetworkError(ctx context.Context, err error) generated.DeprovisionNetworkResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.DeprovisionNetwork401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusNotFound:
		return generated.DeprovisionNetwork404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	case http.StatusConflict:
		return generated.DeprovisionNetwork409JSONResponse{ConflictJSONResponse: generated.ConflictJSONResponse(body)}
	case http.StatusUnprocessableEntity:
		return generated.DeprovisionNetwork422JSONResponse{UnprocessableEntityJSONResponse: generated.UnprocessableEntityJSONResponse(body)}
	default:
		return generated.DeprovisionNetwork500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) GetNetworkProvisioningStatus(ctx context.Context, request generated.GetNetworkProvisioningStatusRequestObject) (generated.GetNetworkProvisioningStatusResponseObject, error) {
	st, err := h.svc.GetNetworkStatus(ctx, request.NetworkId.String())
	if err != nil {
		return mapGetNetworkProvisioningStatusError(ctx, err), nil
	}
	out, err := ToNetworkProvisioningStatusFromService(st)
	if err != nil {
		return generated.GetNetworkProvisioningStatus500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(
			withRequestID(ctx, generated.Error{Code: "INTERNAL", Message: err.Error()}),
		)}, nil
	}
	return generated.GetNetworkProvisioningStatus200JSONResponse(out), nil
}

func mapGetNetworkProvisioningStatusError(ctx context.Context, err error) generated.GetNetworkProvisioningStatusResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.GetNetworkProvisioningStatus401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusNotFound:
		return generated.GetNetworkProvisioningStatus404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	case http.StatusUnprocessableEntity:
		return generated.GetNetworkProvisioningStatus422JSONResponse{UnprocessableEntityJSONResponse: generated.UnprocessableEntityJSONResponse(body)}
	default:
		return generated.GetNetworkProvisioningStatus500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) RemoveGateway(ctx context.Context, request generated.RemoveGatewayRequestObject) (generated.RemoveGatewayResponseObject, error) {
	job, err := h.svc.RemoveGateway(ctx, request.NetworkId.String())
	if err != nil {
		return mapRemoveGatewayError(ctx, err), nil
	}
	out, err := ToJobFromService(job)
	if err != nil {
		return generated.RemoveGateway500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(
			withRequestID(ctx, generated.Error{Code: "INTERNAL", Message: err.Error()}),
		)}, nil
	}
	return generated.RemoveGateway202JSONResponse(out), nil
}

func mapRemoveGatewayError(ctx context.Context, err error) generated.RemoveGatewayResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.RemoveGateway401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusNotFound:
		return generated.RemoveGateway404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	case http.StatusConflict:
		return generated.RemoveGateway409JSONResponse{ConflictJSONResponse: generated.ConflictJSONResponse(body)}
	case http.StatusUnprocessableEntity:
		return generated.RemoveGateway422JSONResponse{UnprocessableEntityJSONResponse: generated.UnprocessableEntityJSONResponse(body)}
	default:
		return generated.RemoveGateway500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) GetGatewayStatus(ctx context.Context, request generated.GetGatewayStatusRequestObject) (generated.GetGatewayStatusResponseObject, error) {
	st, err := h.svc.GetGatewayStatus(ctx, request.NetworkId.String())
	if err != nil {
		return mapGetGatewayStatusError(ctx, err), nil
	}
	out, err := ToGatewayStatusFromService(st)
	if err != nil {
		return generated.GetGatewayStatus500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(
			withRequestID(ctx, generated.Error{Code: "INTERNAL", Message: err.Error()}),
		)}, nil
	}
	return generated.GetGatewayStatus200JSONResponse(out), nil
}

func mapGetGatewayStatusError(ctx context.Context, err error) generated.GetGatewayStatusResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.GetGatewayStatus401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusNotFound:
		return generated.GetGatewayStatus404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	case http.StatusUnprocessableEntity:
		return generated.GetGatewayStatus422JSONResponse{UnprocessableEntityJSONResponse: generated.UnprocessableEntityJSONResponse(body)}
	default:
		return generated.GetGatewayStatus500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) ProvisionGateway(ctx context.Context, request generated.ProvisionGatewayRequestObject) (generated.ProvisionGatewayResponseObject, error) {
	if request.Body == nil {
		return generated.ProvisionGateway400JSONResponse{BadRequestJSONResponse: badRequest(ctx, "request body is required")}, nil
	}
	params, err := ToServiceProvisionGatewayParams(request.Body)
	if err != nil {
		return generated.ProvisionGateway400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err.Error())}, nil
	}
	params.NetworkID = request.NetworkId.String()
	job, err := h.svc.ProvisionGateway(ctx, params)
	if err != nil {
		return mapProvisionGatewayError(ctx, err), nil
	}
	out, err := ToJobFromService(job)
	if err != nil {
		return generated.ProvisionGateway500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(
			withRequestID(ctx, generated.Error{Code: "INTERNAL", Message: err.Error()}),
		)}, nil
	}
	return generated.ProvisionGateway202JSONResponse(out), nil
}

func mapProvisionGatewayError(ctx context.Context, err error) generated.ProvisionGatewayResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusBadRequest:
		return generated.ProvisionGateway400JSONResponse{BadRequestJSONResponse: generated.BadRequestJSONResponse(body)}
	case http.StatusUnauthorized:
		return generated.ProvisionGateway401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusNotFound:
		return generated.ProvisionGateway404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	case http.StatusConflict:
		return generated.ProvisionGateway409JSONResponse{ConflictJSONResponse: generated.ConflictJSONResponse(body)}
	case http.StatusUnprocessableEntity:
		return generated.ProvisionGateway422JSONResponse{UnprocessableEntityJSONResponse: generated.UnprocessableEntityJSONResponse(body)}
	default:
		return generated.ProvisionGateway500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) ListNetworkJobs(ctx context.Context, request generated.ListNetworkJobsRequestObject) (generated.ListNetworkJobsResponseObject, error) {
	networkID := request.NetworkId.String()
	if _, err := h.svc.GetNetworkStatus(ctx, networkID); err != nil {
		if errors.Is(err, service.ErrNetworkNotFound) {
			_, body := mapServiceError(ctx, err)
			return generated.ListNetworkJobs404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}, nil
		}
		return mapListNetworkJobsError(ctx, err), nil
	}
	limit := EffectiveLimit(request.Params.Limit)
	offset := EffectiveOffset(request.Params.Offset)
	jobs, err := h.svc.ListNetworkJobs(ctx, networkID, limit, offset)
	if err != nil {
		return mapListNetworkJobsError(ctx, err), nil
	}
	items := make([]generated.Job, 0, len(jobs))
	for i := range jobs {
		j, err := ToJobFromService(jobs[i])
		if err != nil {
			return generated.ListNetworkJobs500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(
				withRequestID(ctx, generated.Error{Code: "INTERNAL", Message: err.Error()}),
			)}, nil
		}
		items = append(items, j)
	}
	return generated.ListNetworkJobs200JSONResponse(generated.JobList{Items: items}), nil
}

func mapListNetworkJobsError(ctx context.Context, err error) generated.ListNetworkJobsResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.ListNetworkJobs401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusNotFound:
		return generated.ListNetworkJobs404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	case http.StatusUnprocessableEntity:
		return generated.ListNetworkJobs422JSONResponse{UnprocessableEntityJSONResponse: generated.UnprocessableEntityJSONResponse(body)}
	default:
		return generated.ListNetworkJobs500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) ListSubnets(ctx context.Context, request generated.ListSubnetsRequestObject) (generated.ListSubnetsResponseObject, error) {
	networkID := request.NetworkId.String()
	if _, err := h.svc.GetNetworkStatus(ctx, networkID); err != nil {
		if errors.Is(err, service.ErrNetworkNotFound) {
			_, body := mapServiceError(ctx, err)
			return generated.ListSubnets404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}, nil
		}
		return mapListSubnetsError(ctx, err), nil
	}
	subs, err := h.svc.ListSubnets(ctx, networkID)
	if err != nil {
		return mapListSubnetsError(ctx, err), nil
	}
	items := make([]generated.Subnet, 0, len(subs))
	for i := range subs {
		sn, err := ToSubnetFromService(subs[i])
		if err != nil {
			return generated.ListSubnets500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(
				withRequestID(ctx, generated.Error{Code: "INTERNAL", Message: err.Error()}),
			)}, nil
		}
		items = append(items, sn)
	}
	return generated.ListSubnets200JSONResponse(generated.SubnetList{Items: items}), nil
}

func mapListSubnetsError(ctx context.Context, err error) generated.ListSubnetsResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusUnauthorized:
		return generated.ListSubnets401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusNotFound:
		return generated.ListSubnets404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	case http.StatusUnprocessableEntity:
		return generated.ListSubnets422JSONResponse{UnprocessableEntityJSONResponse: generated.UnprocessableEntityJSONResponse(body)}
	default:
		return generated.ListSubnets500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
	}
}

func (h *Handler) CreateSubnet(ctx context.Context, request generated.CreateSubnetRequestObject) (generated.CreateSubnetResponseObject, error) {
	if request.Body == nil {
		return generated.CreateSubnet400JSONResponse{BadRequestJSONResponse: badRequest(ctx, "request body is required")}, nil
	}
	networkID := request.NetworkId.String()
	if _, err := h.svc.GetNetworkStatus(ctx, networkID); err != nil {
		if errors.Is(err, service.ErrNetworkNotFound) {
			_, body := mapServiceError(ctx, err)
			return generated.CreateSubnet404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}, nil
		}
		return mapCreateSubnetError(ctx, err), nil
	}
	params, err := ToServiceProvisionSubnetParams(networkID, request.Body)
	if err != nil {
		return generated.CreateSubnet400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err.Error())}, nil
	}
	sub, err := h.svc.ProvisionSubnet(ctx, params)
	if err != nil {
		return mapCreateSubnetError(ctx, err), nil
	}
	out, err := ToSubnetFromService(sub)
	if err != nil {
		return generated.CreateSubnet500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(
			withRequestID(ctx, generated.Error{Code: "INTERNAL", Message: err.Error()}),
		)}, nil
	}
	return generated.CreateSubnet201JSONResponse(out), nil
}

func mapCreateSubnetError(ctx context.Context, err error) generated.CreateSubnetResponseObject {
	st, body := mapServiceError(ctx, err)
	switch st {
	case http.StatusBadRequest:
		return generated.CreateSubnet400JSONResponse{BadRequestJSONResponse: generated.BadRequestJSONResponse(body)}
	case http.StatusUnauthorized:
		return generated.CreateSubnet401JSONResponse{UnauthorizedJSONResponse: generated.UnauthorizedJSONResponse(body)}
	case http.StatusNotFound:
		return generated.CreateSubnet404JSONResponse{NotFoundJSONResponse: generated.NotFoundJSONResponse(body)}
	case http.StatusConflict:
		return generated.CreateSubnet409JSONResponse{ConflictJSONResponse: generated.ConflictJSONResponse(body)}
	case http.StatusUnprocessableEntity:
		return generated.CreateSubnet422JSONResponse{UnprocessableEntityJSONResponse: generated.UnprocessableEntityJSONResponse(body)}
	default:
		return generated.CreateSubnet500JSONResponse{InternalServerErrorJSONResponse: generated.InternalServerErrorJSONResponse(body)}
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
