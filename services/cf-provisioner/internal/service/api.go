// Package service implements CF-Provisioner domain orchestration between HTTP (future)
// and infrastructure / state repositories.
package service

import (
	"context"

	cidrrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/cidr"
	ciliumrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/cilium"
	gatewayrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/gateway"
	jobsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/jobs"
	kubeconfigrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/kubeconfig"
	subnetsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/subnets"
	vclusterrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/vcluster"
)

// ProvisionerService coordinates CIDR allocation, vCluster lifecycle, Cilium, Gateway API,
// kubeconfig storage, and async jobs.
type ProvisionerService interface {
	ProvisionNetwork(ctx context.Context, params ProvisionNetworkParams) (Job, error)
	GetNetworkStatus(ctx context.Context, networkID string) (NetworkStatus, error)
	DeprovisionNetwork(ctx context.Context, networkID string) (Job, error)
	ProvisionGateway(ctx context.Context, params ProvisionGatewayParams) (Job, error)
	GetGatewayStatus(ctx context.Context, networkID string) (GatewayStatus, error)
	RemoveGateway(ctx context.Context, networkID string) (Job, error)
	GetJob(ctx context.Context, jobID string) (Job, error)
	ListNetworkJobs(ctx context.Context, networkID string, limit, offset int) ([]Job, error)
	ListCIDRAllocations(ctx context.Context, limit, offset int) ([]cidrrepo.CIDRAllocation, error)
	ProvisionSubnet(ctx context.Context, params ProvisionSubnetParams) (Subnet, error)
	ListSubnets(ctx context.Context, networkID string) ([]Subnet, error)
}

// Deps wires repository and infrastructure clients into the service.
type Deps struct {
	VCluster   vclusterrepo.VClusterClient
	Cilium     ciliumrepo.CiliumClient
	Gateway    gatewayrepo.GatewayClient
	Kubeconfig kubeconfigrepo.KubeconfigRepository
	CIDR       cidrrepo.CIDRRepository
	Jobs       jobsrepo.JobsRepository
	Subnets    subnetsrepo.SubnetsRepository
}

// New returns a [ProvisionerService] implementation.
func New(d Deps) ProvisionerService {
	return &CFProvisionerService{
		deps:                d,
		tenantByNetwork:     make(map[string]string),
		vclusterNSByNetwork: make(map[string]string),
	}
}
