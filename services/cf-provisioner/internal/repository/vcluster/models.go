package vcluster

type CreateVClusterParams struct {
	Name      string // e.g. "tenant-abc-network-xyz"
	Namespace string // host namespace where the vCluster pods run
	PodCIDR   string
	SvcCIDR   string
	Region    string
}

type VClusterStatus string

const (
	VClusterStatusCreating VClusterStatus = "creating"
	VClusterStatusRunning  VClusterStatus = "running"
	VClusterStatusFailed   VClusterStatus = "failed"
	VClusterStatusDeleting VClusterStatus = "deleting"
)

type VClusterInfo struct {
	Name      string
	Namespace string
	Status    VClusterStatus
	PodCIDR   string
	SvcCIDR   string
}
