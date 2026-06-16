package workloads

// CloudForge workload label keys. These are applied to the Deployment, pod
// template, and Service so later policy/routing repositories can select tenant
// objects without parsing names.
const (
	LabelTenantID     = "cloudforge.io/tenant-id"
	LabelNetworkID    = "cloudforge.io/network-id"
	LabelSubnetID     = "cloudforge.io/subnet-id"
	LabelAppServiceID = "cloudforge.io/app-service-id"
	LabelVisibility   = "cloudforge.io/visibility"
)

type WorkloadVisibility string

const (
	WorkloadVisibilityPrivate WorkloadVisibility = "private"
	WorkloadVisibilityPublic  WorkloadVisibility = "public"
)

type WorkloadPortProtocol string

const (
	WorkloadPortProtocolHTTP WorkloadPortProtocol = "HTTP"
	WorkloadPortProtocolGRPC WorkloadPortProtocol = "GRPC"
	WorkloadPortProtocolTCP  WorkloadPortProtocol = "TCP"
)

type WorkloadStatus string

const (
	WorkloadStatusPending     WorkloadStatus = "pending"
	WorkloadStatusProgressing WorkloadStatus = "progressing"
	WorkloadStatusReady       WorkloadStatus = "ready"
	WorkloadStatusFailed      WorkloadStatus = "failed"
	WorkloadStatusDeleting    WorkloadStatus = "deleting"
)

// ApplyWorkloadParams is the service-layer desired state after durable
// app-service intent, subnet placement, and any image-build workflow have been
// resolved into a concrete container image.
type ApplyWorkloadParams struct {
	Namespace    string
	Name         string
	TenantID     string
	NetworkID    string
	SubnetID     string
	AppServiceID string
	Visibility   WorkloadVisibility
	Runtime      WorkloadRuntime
}

type WorkloadRuntime struct {
	// ServiceType mirrors the public app-service runtime shape ("rest",
	// "worker", "grpc", etc.). This package does not branch on it directly;
	// Kubernetes Service creation is controlled by Ports so worker services with
	// no ports remain valid.
	ServiceType string
	Image       string
	Command     []string
	Args        []string
	Resources   WorkloadResources
	Env         []WorkloadEnvVar
	Ports       []WorkloadPort
	Replicas    int32
}

type WorkloadResources struct {
	CPU    string
	Memory string
}

// WorkloadEnvVar is intentionally plaintext-only. Secret references from the
// REST/durable model must be resolved by a secret adapter before this repository
// receives apply parameters.
type WorkloadEnvVar struct {
	Name  string
	Value string
}

type WorkloadPort struct {
	Name          string
	ContainerPort int32
	Protocol      WorkloadPortProtocol
}

type WorkloadInfo struct {
	Namespace         string
	Name              string
	DeploymentName    string
	ServiceName       string
	Labels            map[string]string
	Status            WorkloadStatus
	Ready             bool
	DesiredReplicas   int32
	ReadyReplicas     int32
	AvailableReplicas int32
	Conditions        []WorkloadCondition
}

type WorkloadCondition struct {
	Type    string
	Status  string
	Reason  string
	Message string
}
