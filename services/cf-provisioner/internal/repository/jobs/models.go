package jobs

import "time"

type JobType string

const (
	JobTypeProvisionNetwork   JobType = "provision_network"
	JobTypeDeprovisionNetwork JobType = "deprovision_network"
	JobTypeProvisionGateway   JobType = "provision_gateway"
	JobTypeRemoveGateway      JobType = "remove_gateway"
	JobTypeProvisionSubnet    JobType = "provision_subnet"
)

type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
)

type CreateJobParams struct {
	NetworkID string
	TenantID  string
	Type      JobType
}

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
