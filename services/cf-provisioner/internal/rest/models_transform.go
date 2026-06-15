package rest

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	cidrrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/cidr"
	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/rest/generated"
	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/service"
)

// ToServiceProvisionNetworkParams maps the POST /v1/networks body to service input.
// A new network UUID is minted when callers do not provide one.
func ToServiceProvisionNetworkParams(body *generated.ProvisionNetworkJSONRequestBody, trusted TrustedProvisionContext) (service.ProvisionNetworkParams, error) {
	if body == nil {
		return service.ProvisionNetworkParams{}, fmt.Errorf("request body is required")
	}
	networkID := uuid.NewString()
	if body.NetworkId != nil {
		networkID = uuid.UUID(*body.NetworkId).String()
	}
	tenantID := strings.TrimSpace(trusted.TenantID)
	if body.TenantId != nil {
		bodyTenantID := uuid.UUID(*body.TenantId).String()
		if tenantID != "" && tenantID != bodyTenantID {
			return service.ProvisionNetworkParams{}, fmt.Errorf("tenantId conflicts with trusted tenant context")
		}
		if tenantID == "" {
			tenantID = bodyTenantID
		}
	}
	if tenantID == "" {
		return service.ProvisionNetworkParams{}, fmt.Errorf("tenantId is required")
	}
	region := strings.TrimSpace(body.Region)
	if region == "" {
		return service.ProvisionNetworkParams{}, fmt.Errorf("region is required")
	}
	var podHint, svcHint string
	if body.PodCIDRPrefix != nil {
		podHint = strings.TrimSpace(*body.PodCIDRPrefix)
	}
	if body.SvcCIDRPrefix != nil {
		svcHint = strings.TrimSpace(*body.SvcCIDRPrefix)
	}
	return service.ProvisionNetworkParams{
		NetworkID:   networkID,
		TenantID:    tenantID,
		Region:      region,
		PodCIDRHint: podHint,
		SvcCIDRHint: svcHint,
	}, nil
}

// ToServiceProvisionGatewayParams maps the gateway provision body.
func ToServiceProvisionGatewayParams(body *generated.ProvisionGatewayJSONRequestBody) (service.ProvisionGatewayParams, error) {
	if body == nil {
		return service.ProvisionGatewayParams{}, fmt.Errorf("request body is required")
	}
	host := strings.TrimSpace(body.PublicDNSName)
	if host == "" {
		return service.ProvisionGatewayParams{}, fmt.Errorf("publicDNSName is required")
	}
	tls := true
	if body.TlsEnabled != nil {
		tls = *body.TlsEnabled
	}
	return service.ProvisionGatewayParams{
		PublicDNSName: host,
		TLSEnabled:    tls,
	}, nil
}

// ToServiceProvisionSubnetParams maps create subnet body plus path network id.
func ToServiceProvisionSubnetParams(networkID string, body *generated.CreateSubnetJSONRequestBody) (service.ProvisionSubnetParams, error) {
	if body == nil {
		return service.ProvisionSubnetParams{}, fmt.Errorf("request body is required")
	}
	return service.ProvisionSubnetParams{
		NetworkID: strings.TrimSpace(networkID),
		Type:      string(body.Type),
		CIDR:      strings.TrimSpace(body.Cidr),
		Zone:      stringFromPtr(body.Zone),
	}, nil
}

func stringFromPtr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// ToJobFromService maps a service job to the OpenAPI Job model.
func ToJobFromService(j service.Job) (generated.Job, error) {
	id, err := uuid.Parse(strings.TrimSpace(j.ID))
	if err != nil {
		return generated.Job{}, fmt.Errorf("invalid job id: %w", err)
	}
	nid, err := uuid.Parse(strings.TrimSpace(j.NetworkID))
	if err != nil {
		return generated.Job{}, fmt.Errorf("invalid network id: %w", err)
	}
	var errMsg *string
	if strings.TrimSpace(j.ErrorMessage) != "" {
		m := j.ErrorMessage
		errMsg = &m
	}
	var updated *time.Time
	if !j.UpdatedAt.IsZero() {
		u := j.UpdatedAt.UTC()
		updated = &u
	}
	return generated.Job{
		Id:           openapi_types.UUID(id),
		NetworkId:    openapi_types.UUID(nid),
		Type:         generated.JobType(j.Type),
		Status:       generated.JobStatus(j.Status),
		ErrorMessage: errMsg,
		CreatedAt:    j.CreatedAt.UTC(),
		UpdatedAt:    updated,
	}, nil
}

// ToNetworkProvisioningStatusFromService maps GetNetworkStatus output.
func ToNetworkProvisioningStatusFromService(s service.NetworkStatus) (generated.NetworkProvisioningStatus, error) {
	nid, err := uuid.Parse(strings.TrimSpace(s.NetworkID))
	if err != nil {
		return generated.NetworkProvisioningStatus{}, fmt.Errorf("invalid network id: %w", err)
	}
	tid := uuid.Nil
	if strings.TrimSpace(s.TenantID) != "" {
		if parsed, err := uuid.Parse(s.TenantID); err == nil {
			tid = parsed
		}
	}
	var fail *string
	if strings.TrimSpace(s.FailureReason) != "" {
		f := s.FailureReason
		fail = &f
	}
	var vcn *string
	if strings.TrimSpace(s.VClusterName) != "" {
		v := s.VClusterName
		vcn = &v
	}
	var updated *time.Time
	if !s.UpdatedAt.IsZero() {
		u := s.UpdatedAt.UTC()
		updated = &u
	}
	created := s.CreatedAt.UTC()
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return generated.NetworkProvisioningStatus{
		NetworkId:     openapi_types.UUID(nid),
		TenantId:      openapi_types.UUID(tid),
		Status:        generated.NetworkProvisioningStatusStatus(s.Status),
		PodCIDR:       s.PodCIDR,
		SvcCIDR:       s.SvcCIDR,
		VclusterName:  vcn,
		CreatedAt:     created,
		UpdatedAt:     updated,
		FailureReason: fail,
	}, nil
}

// ToGatewayStatusFromService maps gateway read model.
func ToGatewayStatusFromService(s service.GatewayStatus) (generated.GatewayStatus, error) {
	nid, err := uuid.Parse(strings.TrimSpace(s.NetworkID))
	if err != nil {
		return generated.GatewayStatus{}, fmt.Errorf("invalid network id: %w", err)
	}
	var ep *string
	if strings.TrimSpace(s.PublicEndpoint) != "" {
		e := s.PublicEndpoint
		ep = &e
	}
	var route *string
	if strings.TrimSpace(s.HTTPRouteName) != "" {
		r := s.HTTPRouteName
		route = &r
	}
	var created *time.Time
	if !s.CreatedAt.IsZero() {
		c := s.CreatedAt.UTC()
		created = &c
	}
	return generated.GatewayStatus{
		NetworkId:      openapi_types.UUID(nid),
		Status:         generated.GatewayStatusStatus(s.Status),
		PublicEndpoint: ep,
		HttpRouteName:  route,
		CreatedAt:      created,
	}, nil
}

// ToSubnetFromService maps a service subnet to OpenAPI.
func ToSubnetFromService(s service.Subnet) (generated.Subnet, error) {
	id, err := uuid.Parse(strings.TrimSpace(s.ID))
	if err != nil {
		return generated.Subnet{}, fmt.Errorf("invalid subnet id: %w", err)
	}
	nid, err := uuid.Parse(strings.TrimSpace(s.NetworkID))
	if err != nil {
		return generated.Subnet{}, fmt.Errorf("invalid network id: %w", err)
	}
	var z *string
	if strings.TrimSpace(s.Zone) != "" {
		zz := s.Zone
		z = &zz
	}
	var created *time.Time
	if !s.CreatedAt.IsZero() {
		c := s.CreatedAt.UTC()
		created = &c
	}
	return generated.Subnet{
		Id:        openapi_types.UUID(id),
		NetworkId: openapi_types.UUID(nid),
		Type:      generated.SubnetType(s.Type),
		Cidr:      s.CIDR,
		Zone:      z,
		CreatedAt: created,
	}, nil
}

// ToCIDRAllocationFromRepo maps a repository allocation row to OpenAPI.
func ToCIDRAllocationFromRepo(a cidrrepo.CIDRAllocation) (generated.CIDRAllocation, error) {
	nid, err := uuid.Parse(strings.TrimSpace(a.NetworkID))
	if err != nil {
		return generated.CIDRAllocation{}, fmt.Errorf("invalid network id: %w", err)
	}
	return generated.CIDRAllocation{
		NetworkId:   openapi_types.UUID(nid),
		PodCIDR:     a.PodCIDR,
		SvcCIDR:     a.SvcCIDR,
		AllocatedAt: a.AllocatedAt.UTC(),
	}, nil
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
