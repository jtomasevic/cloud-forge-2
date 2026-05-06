// Package network defines the canonical shared network and subnet models used
// across all CloudForge platform services. These are domain types — not
// persistence models or REST models.
package network

import "time"

// NetworkStatus represents the lifecycle state of a private network.
type NetworkStatus string

const (
	// NetworkStatusProvisioning indicates the network infrastructure is being created.
	NetworkStatusProvisioning NetworkStatus = "provisioning"
	// NetworkStatusActive indicates the network is fully operational.
	NetworkStatusActive NetworkStatus = "active"
	// NetworkStatusSuspended indicates the network has been temporarily suspended.
	NetworkStatusSuspended NetworkStatus = "suspended"
	// NetworkStatusDeleted indicates the network has been deprovisioned and removed.
	NetworkStatusDeleted NetworkStatus = "deleted"
)

// SubnetType distinguishes private from public subnets within a network.
type SubnetType string

const (
	// SubnetTypePrivate is a subnet whose resources are not reachable from the internet.
	SubnetTypePrivate SubnetType = "private"
	// SubnetTypePublic is a subnet whose resources may be exposed via an internet gateway.
	SubnetTypePublic SubnetType = "public"
)

// Network is the canonical shared private network model.
type Network struct {
	ID        string
	TenantID  string
	Region    string
	PodCIDR   string
	SvcCIDR   string
	Status    NetworkStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Subnet represents a logical subnet within a private network.
type Subnet struct {
	ID        string
	NetworkID string
	Type      SubnetType
	CIDR      string
	Zone      string
	CreatedAt time.Time
}
