package service

import "time"

// Domain requests and read models for [ProvisionerService].
//
// These types are the service boundary: they are not the same structs as in
// internal/repository/cidr, jobs, vcluster, etc. [ProvisionerService] maps repository rows into
// these shapes (or accepts them as RPC-style inputs) so HTTP/OpenAPI can stay stable if persistence
// changes.
//
// How things connect (one private network keyed by NetworkID):
//
//   - [ProvisionNetworkParams] starts lifecycle: allocate pod/svc CIDRs, create a provision_network
//     [Job], then async vCluster + Cilium + kubeconfig. [NetworkStatus] summarizes that work using
//     the CIDR row plus the latest provision/deprovision job (see GetNetworkStatus).
//
//   - [Job] is the polling handle for async work (provision network, deprovision, gateway up/down).
//     Every long operation creates a row in the jobs repository; [ListNetworkJobs] returns the history
//     for one NetworkID.
//
//   - [ProvisionGatewayParams] attaches public ingress to an already-active network: creates a
//     provision_gateway [Job], then HTTPRoute + Cilium ingress policy. [GatewayStatus] reads the
//     HTTPRoute; it is keyed by the same NetworkID as the network (one route name derived from ID).
//
//   - [Subnet] is optional overlay data today: stored in-process only until a subnet table exists.
//     It still hangs off NetworkID like everything else.
//
// TenantID appears on inputs and on [NetworkStatus] for kubeconfig correlation; it is not on the CIDR
// allocation row, so the service keeps a small in-memory networkID→tenant map after provision starts.

// ProvisionNetworkParams is the input for starting private-network provisioning.
// NetworkID is the stable id for the tenant network (often a UUID string). TenantID scopes kubeconfig
// storage in OpenBao. Region is passed through to the vCluster create path. Optional hints map to
// repository "requested" CIDR fields; leave empty to auto-allocate from the platform pool.
type ProvisionNetworkParams struct {
	NetworkID   string
	TenantID    string
	Region      string
	PodCIDRHint string // optional; maps to repository requested pod CIDR
	SvcCIDRHint string // optional; maps to repository requested service CIDR
}

// ProvisionGatewayParams is the input for exposing a network to the internet via Gateway API + Cilium.
// Requires the network to already be in an active state. PublicDNSName becomes the HTTPRoute hostname.
// TLSEnabled selects backend port (443 vs 80) for both the route backendRef and ingress policy until
// TLSRoute wiring exists in the gateway repository.
type ProvisionGatewayParams struct {
	NetworkID     string
	PublicDNSName string
	TLSEnabled    bool
}

// ProvisionSubnetParams is the input for the in-memory subnet registry (no persistence yet).
// Type is "private" or "public". CIDR and Zone are opaque to the service beyond validation.
type ProvisionSubnetParams struct {
	NetworkID string
	Type      string // "private" or "public"
	CIDR      string
	Zone      string
}

// NetworkStatusValue is the coarse lifecycle of a network from the control-plane point of view.
// It is derived (not stored as a single column): see GetNetworkStatus and inferNetworkStatusFromJobs.
type NetworkStatusValue string

const (
	NetworkStatusProvisioning   NetworkStatusValue = "provisioning"
	NetworkStatusActive         NetworkStatusValue = "active"
	NetworkStatusDeprovisioning NetworkStatusValue = "deprovisioning"
	NetworkStatusDeprovisioned  NetworkStatusValue = "deprovisioned"
	NetworkStatusFailed         NetworkStatusValue = "failed"
)

// NetworkStatus is the read model returned by GetNetworkStatus.
//
// Fields are filled from different sources: PodCIDR/SvcCIDR/CreatedAt from the CIDR allocation row
// when it still exists; Status and FailureReason from the latest provision_network or deprovision_network
// job; TenantID from service memory if known; VClusterName is always the deterministic name derived
// from NetworkID (same string the provisioner uses for vCluster create).
type NetworkStatus struct {
	NetworkID     string
	TenantID      string
	Status        NetworkStatusValue
	PodCIDR       string
	SvcCIDR       string
	VClusterName  string
	FailureReason string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// GatewayStatusValue describes public ingress readiness for a network.
// It is mainly driven by HTTPRoute status from the gateway repository; removing the route yields
// ErrGatewayNotFound from GetGatewayStatus rather than GatewayStatusRemoved until a richer model is needed.
type GatewayStatusValue string

const (
	GatewayStatusProvisioning GatewayStatusValue = "provisioning"
	GatewayStatusActive       GatewayStatusValue = "active"
	GatewayStatusRemoving     GatewayStatusValue = "removing"
	GatewayStatusRemoved      GatewayStatusValue = "removed"
	GatewayStatusFailed       GatewayStatusValue = "failed"
)

// GatewayStatus is the read model for one network's internet gateway surface.
// NetworkID links back to the same network as [NetworkStatus]. PublicEndpoint is currently the
// route hostname; HTTPRouteName is the Kubernetes object name (gw-* + short id). CreatedAt is reserved
// for when route creation time is plumbed from the API.
type GatewayStatus struct {
	NetworkID      string
	Status         GatewayStatusValue
	PublicEndpoint string
	HTTPRouteName  string
	CreatedAt      time.Time
}

// Job is a stable handle for async provisioning work, returned immediately from Provision*/Deprovision/
// RemoveGateway before infrastructure finishes.
//
// NetworkID ties the job to a network for listing and status correlation. Type and Status are plain
// strings here (mirroring job repository string values) for simple JSON serialization; the jobs repo
// uses typed constants internally. ErrorMessage is set when Status indicates failure.
type Job struct {
	ID           string
	NetworkID    string
	Type         string
	Status       string
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Subnet is a logical slice inside a network (L3 segmentation / AZ placement).
// Today it exists only in the service process; NetworkID groups subnets with the same network as
// [NetworkStatus] and [Job]. ID is a new UUID per ProvisionSubnet call.
type Subnet struct {
	ID        string
	NetworkID string
	Type      string
	CIDR      string
	Zone      string
	CreatedAt time.Time
}
