package jobs

import "time"

// JobType is stored as provisioning_jobs.job_type.
//
// Keep these string values aligned with api/cf-provisioner/v1/openapi.yaml. Scylla stores them as
// plain text so the service can add lifecycle categories without schema churn; migrations create any
// additional indexes needed for workflows such as app-service job correlation.
type JobType string

const (
	JobTypeProvisionNetwork   JobType = "provision_network"
	JobTypeDeprovisionNetwork JobType = "deprovision_network"
	JobTypeProvisionGateway   JobType = "provision_gateway"
	JobTypeRemoveGateway      JobType = "remove_gateway"
	JobTypeProvisionSubnet    JobType = "provision_subnet"

	// App-service lifecycle is asynchronous for the same reason network and gateway operations are:
	// Kubernetes workload creation, route reconciliation, and policy updates can outlive the HTTP
	// request. These types are persisted in provisioning_jobs and correlated through the
	// app_service_jobs_by_app_service migration table added with the app-service schema.
	JobTypeCreateAppService         JobType = "create_app_service"
	JobTypeDeleteAppService         JobType = "delete_app_service"
	JobTypeExposeAppService         JobType = "expose_app_service"
	JobTypeRemoveAppServiceExposure JobType = "remove_app_service_exposure"
)

// JobStatus is stored as provisioning_jobs.status and mirrors the OpenAPI enum.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
)

// CreateJobParams is the repository input for inserting a pending job.
//
// NetworkID stays required because all current CF-Provisioner workflows are scoped under a private
// network. TenantID is optional for older call paths but should be supplied by app-service flows so
// records remain auditably tenant-scoped.
type CreateJobParams struct {
	NetworkID string
	TenantID  string
	Type      JobType
}

// Job is the durable async-work read model hydrated from provisioning_jobs.
type Job struct {
	ID           string
	TenantID     string
	NetworkID    string
	Type         JobType
	Status       JobStatus
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
