// Package workloads owns the Kubernetes workload objects for CF App Service.
//
// The package talks to a tenant vCluster through normal client-go APIs. It does
// not fetch kubeconfigs, validate tenant authorization, build container images,
// create Gateway API routes, or change Cilium policies; those decisions belong
// to the service orchestration layer and sibling infrastructure repositories.
package workloads

import "context"

// WorkloadClient applies and observes the Deployment/Service pair for one
// CloudForge app service inside a tenant vCluster.
type WorkloadClient interface {
	// Apply creates or updates the Deployment and, when ports are declared, the
	// matching ClusterIP Service. If no ports are declared, any stale Service with
	// the workload name is removed so worker-style services stay service-less.
	Apply(ctx context.Context, params ApplyWorkloadParams) (WorkloadInfo, error)

	// Get returns Deployment-derived readiness and Service presence for the
	// named workload. Missing Deployments return ErrWorkloadNotFound.
	Get(ctx context.Context, namespace, name string) (WorkloadInfo, error)

	// Delete removes the Service and Deployment. Missing objects are treated as
	// success so app-service cleanup jobs can be safely retried.
	Delete(ctx context.Context, namespace, name string) error
}

// New builds a WorkloadClient from tenant vCluster kubeconfig bytes.
//
// Callers normally load these bytes from the kubeconfig repository after the
// vCluster is ready. The resulting client only has the permissions encoded in
// that tenant kubeconfig; it should never be constructed from the host cluster
// kubeconfig for app workload creation.
func New(tenantKubeconfig []byte) (WorkloadClient, error) {
	return newWorkloadClient(tenantKubeconfig)
}
