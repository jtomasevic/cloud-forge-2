package cidr

import "time"

type AllocateParams struct {
	NetworkID    string
	RequestedPod string // empty = auto-allocate
	RequestedSvc string // empty = auto-allocate
}

type CIDRAllocation struct {
	NetworkID   string
	PodCIDR     string
	SvcCIDR     string
	AllocatedAt time.Time
}
