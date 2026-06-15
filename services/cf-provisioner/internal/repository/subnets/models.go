package subnets

import "time"

// CreateSubnetParams is the repository create input after service-level validation.
type CreateSubnetParams struct {
	NetworkID string
	Type      string
	CIDR      string
	Zone      string
}

// Subnet is the durable ScyllaDB row model for logical network subnets.
type Subnet struct {
	ID        string
	NetworkID string
	Type      string
	CIDR      string
	Zone      string
	CreatedAt time.Time
}
