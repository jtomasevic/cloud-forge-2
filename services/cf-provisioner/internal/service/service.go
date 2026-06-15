package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	cfnetwork "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/network"
	cidrrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/cidr"
	ciliumrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/cilium"
	gatewayrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/gateway"
	jobsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/jobs"
	subnetsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/subnets"
	vclusterrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/vcluster"
)

// CFProvisionerService implements [ProvisionerService].
//
// It owns a small amount of in-process state (tenant and vCluster namespace hints) that is not
// persisted in Scylla today; async work always uses a fresh [context.Background] so HTTP request
// cancellation does not strand half-finished infrastructure.
type CFProvisionerService struct {
	deps Deps

	mu                  sync.RWMutex
	tenantByNetwork     map[string]string // used for OpenBao paths on kubeconfig revoke (see rememberTenant).
	vclusterNSByNetwork map[string]string // host namespace observed after vCluster is up; falls back to vclusterName if unknown.
}

// --- Naming helpers ---
//
// Kubernetes object names and Cilium policy names must be deterministic from networkID so
// provision, gateway, and deprovision agree on the same strings.
//
// CRITICAL: sanitizeNetworkIDForPolicy, defaultDenyPolicyName, and ingressPolicyName must stay
// in lockstep with internal/repository/cilium/client.go (sanitizeNetworkID, defaultDenyName,
// ingressPolicyName). If they diverge, deprovision will delete the wrong policy or nothing.

// sanitizeNetworkIDForPolicy normalizes networkID for use inside Kubernetes resource names.
// Effects: lowercase + trim so " UUID " matches the canonical id; spaces become hyphens so
// multi-word test ids remain valid DNS label fragments when embedded in policy names.
func sanitizeNetworkIDForPolicy(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.ReplaceAll(id, " ", "-")
	return id
}

// defaultDenyPolicyName returns the CiliumNetworkPolicy object name for the default-deny egress
// policy scoped to this network’s vCluster namespace. Must match the cilium repository.
func defaultDenyPolicyName(networkID string) string {
	return "default-deny-egress-" + sanitizeNetworkIDForPolicy(networkID)
}

// ingressPolicyName returns the CiliumNetworkPolicy name used when an internet gateway is
// attached (ingress path). Removed on gateway teardown. Must match the cilium repository.
func ingressPolicyName(networkID string) string {
	return "internet-ingress-" + sanitizeNetworkIDForPolicy(networkID)
}

// vclusterShortID derives a short, stable token from networkID for naming host-cluster objects.
//
// Why hex-first: typical network IDs are UUIDs; taking the first eight hexadecimal characters
// yields a compact token that is stable for the same UUID string (hyphens in the UUID are skipped
// while scanning so we still collect eight hex digits).
//
// Fallback path: if the id has no hex digits (unlikely), we strip hyphens and take a rune prefix,
// or finally "net" so we never return an empty name (empty names would break API validation).
func vclusterShortID(networkID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(networkID) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			b.WriteRune(r)
			if b.Len() == 8 {
				return b.String()
			}
		}
	}
	if b.Len() > 0 {
		return b.String()
	}
	s := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(networkID)), "-", "")
	if len(s) >= 8 {
		return s[:8]
	}
	if s != "" {
		return s
	}
	return "net"
}

// vclusterName is the vCluster CLI "name" and the host namespace we create under (same string).
func vclusterName(networkID string) string {
	return "cf-" + vclusterShortID(networkID)
}

// httpRouteName is the Gateway API HTTPRoute object name for this network’s public route.
func httpRouteName(networkID string) string {
	return "gw-" + vclusterShortID(networkID)
}

// httpRouteNamespace selects where HTTPRoutes are reconciled (Envoy Gateway watches this namespace).
// Override with env CF_HTTPROUTE_NAMESPACE in clusters that don’t use the default.
func httpRouteNamespace() string {
	if ns := strings.TrimSpace(os.Getenv("CF_HTTPROUTE_NAMESPACE")); ns != "" {
		return ns
	}
	return "cf-provisioner"
}

// gatewayBackendService is the Service name traffic is forwarded to after the HTTPRoute matches.
// Override with CF_GATEWAY_BACKEND_SERVICE when the dataplane placeholder is wired for real.
func gatewayBackendService() string {
	if s := strings.TrimSpace(os.Getenv("CF_GATEWAY_BACKEND_SERVICE")); s != "" {
		return s
	}
	return "cf-backend-placeholder"
}

// gatewayBackendPort picks a port for the HTTPRoute backendRef and Cilium ingress policy; TLS at
// the edge often terminates on 443 even though HTTPRoute rules are HTTP-level; this matches the
// repository’s TLSEnabled placeholder until TLSRoute is implemented.
func gatewayBackendPort(tls bool) int {
	if tls {
		return 443
	}
	return 80
}

// toServiceJob maps persistence types to the service DTO (stringly job type/status for REST).
func toServiceJob(j jobsrepo.Job) Job {
	return Job{
		ID:           j.ID,
		TenantID:     j.TenantID,
		NetworkID:    j.NetworkID,
		Type:         string(j.Type),
		Status:       string(j.Status),
		ErrorMessage: j.ErrorMessage,
		CreatedAt:    j.CreatedAt,
		UpdatedAt:    j.UpdatedAt,
	}
}

// rememberTenant records which tenant owns a network’s kubeconfig in OpenBao. Written after
// successful CIDR allocate so deprovision can Revoke even though tenant ID is not on the CIDR row.
// Effect: in-memory only until a durable mapping exists elsewhere.
func (s *CFProvisionerService) rememberTenant(networkID, tenantID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenantByNetwork[networkID] = strings.TrimSpace(tenantID)
}

// tenantID returns the last remembered tenant for kubeconfig operations (may be empty).
func (s *CFProvisionerService) tenantID(networkID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tenantByNetwork[networkID]
}

// rememberVClusterNS stores the host namespace the vCluster StatefulSet lives in (from Get after ready).
// Used for Cilium policies and gateway ingress policy namespace; falls back to vclusterName if unset.
func (s *CFProvisionerService) rememberVClusterNS(networkID, ns string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vclusterNSByNetwork[networkID] = strings.TrimSpace(ns)
}

// vclusterNamespace returns the namespace used for Cilium policies targeting this network’s workloads.
func (s *CFProvisionerService) vclusterNamespace(networkID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if ns := s.vclusterNSByNetwork[networkID]; ns != "" {
		return ns
	}
	return vclusterName(networkID)
}

// clearNetworkMaps drops cached tenant/namespace hints after successful deprovision.
func (s *CFProvisionerService) clearNetworkMaps(networkID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tenantByNetwork, networkID)
	delete(s.vclusterNSByNetwork, networkID)
}

// ProvisionNetwork reserves CIDRs synchronously, inserts a pending job, then kicks async work.
// Why allocate before the job row: callers must fail fast on pool exhaustion without a stuck job.
// Why context.Background in the goroutine: the HTTP ctx may be cancelled while infra still runs.
func (s *CFProvisionerService) ProvisionNetwork(ctx context.Context, params ProvisionNetworkParams) (Job, error) {
	networkID := strings.TrimSpace(params.NetworkID)
	tenantID := strings.TrimSpace(params.TenantID)
	if networkID == "" || tenantID == "" {
		return Job{}, cferrors.Wrap(cferrors.CodeInvalidInput, "networkID and tenantID are required", cferrors.ErrInvalidInput)
	}

	alloc, err := s.deps.CIDR.Allocate(ctx, cidrrepo.AllocateParams{
		NetworkID:    networkID,
		RequestedPod: strings.TrimSpace(params.PodCIDRHint),
		RequestedSvc: strings.TrimSpace(params.SvcCIDRHint),
	})
	if err != nil {
		return Job{}, err
	}

	s.rememberTenant(networkID, tenantID)

	job, err := s.deps.Jobs.Create(ctx, jobsrepo.CreateJobParams{
		NetworkID: networkID,
		TenantID:  tenantID,
		Type:      jobsrepo.JobTypeProvisionNetwork,
	})
	if err != nil {
		return Job{}, err
	}

	jobID := job.ID
	vcName := vclusterName(networkID)
	go s.runProvisionNetwork(context.Background(), jobID, networkID, tenantID, vcName, alloc, strings.TrimSpace(params.Region))

	return toServiceJob(job), nil
}

// runProvisionNetwork performs the long-running provision steps and updates the job row on each outcome.
// Order matters: default-deny only after the vCluster API server is reachable; kubeconfig only after
// the control plane reports ready, otherwise Store would save unusable bytes.
func (s *CFProvisionerService) runProvisionNetwork(bg context.Context, jobID, networkID, tenantID, vcName string, alloc cidrrepo.CIDRAllocation, region string) {
	defer s.recoverJob(bg, jobID, "provision_network")

	if err := s.deps.Jobs.UpdateStatus(bg, jobID, jobsrepo.JobStatusRunning, ""); err != nil {
		slog.Error("provision_network: update running", "job_id", jobID, "err", err)
		return
	}

	// Create uses the same name and namespace so host RBAC and Cilium namespace selectors line up.
	_, err := s.deps.VCluster.Create(bg, vclusterrepo.CreateVClusterParams{
		Name:      vcName,
		Namespace: vcName,
		PodCIDR:   alloc.PodCIDR,
		SvcCIDR:   alloc.SvcCIDR,
		Region:    region,
	})
	if err != nil {
		s.failJob(bg, jobID, err)
		return
	}

	// CLI create returns quickly; workloads need time before kubeconfig/policy calls succeed.
	if err := waitVClusterRunning(bg, s.deps.VCluster, vcName, 5*time.Minute); err != nil {
		s.failJob(bg, jobID, err)
		return
	}

	info, err := s.deps.VCluster.Get(bg, vcName)
	if err != nil {
		s.failJob(bg, jobID, err)
		return
	}
	ns := strings.TrimSpace(info.Namespace)
	if ns == "" {
		ns = vcName
	}
	s.rememberVClusterNS(networkID, ns)

	// networkID is passed through so the Cilium policy name matches sanitizeNetworkIDForPolicy above.
	if err := s.deps.Cilium.ApplyDefaultDenyPolicy(bg, ns, networkID); err != nil {
		s.failJob(bg, jobID, err)
		return
	}

	kc, err := waitKubeconfig(bg, s.deps.VCluster, vcName, 2*time.Minute)
	if err != nil {
		s.failJob(bg, jobID, err)
		return
	}

	if err := s.deps.Kubeconfig.Store(bg, tenantID, kc); err != nil {
		s.failJob(bg, jobID, err)
		return
	}

	if err := s.deps.Jobs.UpdateStatus(bg, jobID, jobsrepo.JobStatusSucceeded, ""); err != nil {
		slog.Error("provision_network: update succeeded", "job_id", jobID, "err", err)
	}
}

// waitVClusterRunning polls until the vCluster reports Running or a terminal failure/timeout.
func waitVClusterRunning(ctx context.Context, vc vclusterrepo.VClusterClient, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	delay := 100 * time.Millisecond
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := vc.Get(ctx, name)
		if err != nil {
			return err
		}
		switch info.Status {
		case vclusterrepo.VClusterStatusRunning:
			return nil
		case vclusterrepo.VClusterStatusFailed:
			return fmt.Errorf("vcluster %q entered failed state", name)
		}
		time.Sleep(delay)
		if delay < 5*time.Second {
			delay *= 2
		}
	}
	return fmt.Errorf("timeout waiting for vcluster %q to run", name)
}

// waitKubeconfig retries GetKubeconfig while the CLI returns ErrKubeconfigNotReady (warmup window).
func waitKubeconfig(ctx context.Context, vc vclusterrepo.VClusterClient, name string, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	delay := 200 * time.Millisecond
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		kc, err := vc.GetKubeconfig(ctx, name)
		if err == nil {
			return kc, nil
		}
		if !errors.Is(err, vclusterrepo.ErrKubeconfigNotReady) {
			return nil, err
		}
		time.Sleep(delay)
		if delay < 3*time.Second {
			delay *= 2
		}
	}
	return nil, fmt.Errorf("timeout waiting for kubeconfig for vcluster %q", name)
}

// recoverJob turns panics in async goroutines into a failed job so operators see a paper trail.
func (s *CFProvisionerService) recoverJob(bg context.Context, jobID, op string) {
	if r := recover(); r != nil {
		slog.Error("async job panic", "op", op, "job_id", jobID, "recover", r)
		_ = s.deps.Jobs.UpdateStatus(bg, jobID, jobsrepo.JobStatusFailed, fmt.Sprint(r))
	}
}

// failJob persists terminal error text on the job (truncated to keep Scylla row size bounded).
func (s *CFProvisionerService) failJob(bg context.Context, jobID string, err error) {
	msg := err.Error()
	if len(msg) > 4000 {
		msg = msg[:4000]
	}
	if u := s.deps.Jobs.UpdateStatus(bg, jobID, jobsrepo.JobStatusFailed, msg); u != nil {
		slog.Error("job fail: could not persist status", "job_id", jobID, "err", u)
	}
}

// GetNetworkStatus synthesizes a view from jobs + CIDR because there is no dedicated networks table.
//
// Why list jobs before CIDR.Get: we need job-derived state even when the allocation row is already
// released (fully deprovisioned) so callers can still see "deprovisioned" from the last job.
// TenantID comes from in-memory rememberTenant; it may be empty for networks created elsewhere.
func (s *CFProvisionerService) GetNetworkStatus(ctx context.Context, networkID string) (NetworkStatus, error) {
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return NetworkStatus{}, cferrors.Wrap(cferrors.CodeInvalidInput, "networkID is required", cferrors.ErrInvalidInput)
	}

	jobsList, err := s.deps.Jobs.ListByNetwork(ctx, networkID)
	if err != nil {
		return NetworkStatus{}, err
	}

	alloc, err := s.deps.CIDR.Get(ctx, networkID)
	cidrMissing := errors.Is(err, cidrrepo.ErrCIDRNotFound)
	if err != nil && !cidrMissing {
		return NetworkStatus{}, err
	}

	status, failure, updated := inferNetworkStatusFromJobs(jobsList, cidrMissing)
	// Unknown network: no CIDR row and no lifecycle jobs to infer from.
	if cidrMissing && status == "" {
		return NetworkStatus{}, ErrNetworkNotFound
	}

	tenant := s.tenantID(networkID)
	if tenant == "" {
		tenant = latestTenantIDFromJobs(jobsList)
	}
	vc := vclusterName(networkID)
	ns := NetworkStatus{
		NetworkID:     networkID,
		TenantID:      tenant,
		Status:        status,
		VClusterName:  vc,
		FailureReason: failure,
		UpdatedAt:     updated,
	}
	if !cidrMissing {
		ns.PodCIDR = alloc.PodCIDR
		ns.SvcCIDR = alloc.SvcCIDR
		ns.CreatedAt = alloc.AllocatedAt
	}
	return ns, nil
}

// inferNetworkStatusFromJobs picks the most recent provision_network or deprovision_network job by
// CreatedAt and maps its status to a coarse NetworkStatusValue.
//
// Why CreatedAt not UpdatedAt: retries bump UpdatedAt; CreatedAt stays the true ordering of user-driven
// lifecycle attempts when jobs are listed out of strict wall-clock order.
//
// cidrMissing tweaks provision success: if the row is gone but the last provision job succeeded,
// we treat that as deprovisioned (CIDR released after teardown) rather than "active with no CIDR".
func inferNetworkStatusFromJobs(jobsList []jobsrepo.Job, cidrMissing bool) (NetworkStatusValue, string, time.Time) {
	pickIdx := -1
	for i := range jobsList {
		j := jobsList[i]
		switch j.Type {
		case jobsrepo.JobTypeProvisionNetwork, jobsrepo.JobTypeDeprovisionNetwork:
			if pickIdx < 0 || j.CreatedAt.After(jobsList[pickIdx].CreatedAt) {
				pickIdx = i
			}
		}
	}
	if pickIdx < 0 {
		if cidrMissing {
			return "", "", time.Time{}
		}
		// CIDR exists but no tracked lifecycle jobs yet (e.g. allocate succeeded, job row not visible).
		return NetworkStatusActive, "", time.Time{}
	}
	pick := jobsList[pickIdx]

	switch pick.Type {
	case jobsrepo.JobTypeDeprovisionNetwork:
		switch pick.Status {
		case jobsrepo.JobStatusSucceeded:
			return NetworkStatusDeprovisioned, "", pick.UpdatedAt
		case jobsrepo.JobStatusFailed:
			return NetworkStatusFailed, pick.ErrorMessage, pick.UpdatedAt
		case jobsrepo.JobStatusPending, jobsrepo.JobStatusRunning:
			return NetworkStatusDeprovisioning, "", pick.UpdatedAt
		}
	case jobsrepo.JobTypeProvisionNetwork:
		switch pick.Status {
		case jobsrepo.JobStatusFailed:
			return NetworkStatusFailed, pick.ErrorMessage, pick.UpdatedAt
		case jobsrepo.JobStatusPending, jobsrepo.JobStatusRunning:
			return NetworkStatusProvisioning, "", pick.UpdatedAt
		case jobsrepo.JobStatusSucceeded:
			if cidrMissing {
				return NetworkStatusDeprovisioned, "", pick.UpdatedAt
			}
			return NetworkStatusActive, "", pick.UpdatedAt
		}
	}

	// Unexpected job status enum combination: fall back conservatively.
	if cidrMissing {
		return "", "", time.Time{}
	}
	return NetworkStatusActive, "", pick.UpdatedAt
}

func latestTenantIDFromJobs(jobsList []jobsrepo.Job) string {
	pickIdx := -1
	for i := range jobsList {
		if strings.TrimSpace(jobsList[i].TenantID) == "" {
			continue
		}
		if pickIdx < 0 || jobsList[i].CreatedAt.After(jobsList[pickIdx].CreatedAt) {
			pickIdx = i
		}
	}
	if pickIdx < 0 {
		return ""
	}
	return strings.TrimSpace(jobsList[pickIdx].TenantID)
}

// DeprovisionNetwork validates the allocation still exists, enqueues async teardown, and returns the job.
// Sync path only checks CIDR.Get so we never enqueue teardown for unknown networks; tenantID is read
// once and passed into the goroutine snapshot (maps can change later, but deprovision is tied to this call).
func (s *CFProvisionerService) DeprovisionNetwork(ctx context.Context, networkID string) (Job, error) {
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return Job{}, cferrors.Wrap(cferrors.CodeInvalidInput, "networkID is required", cferrors.ErrInvalidInput)
	}

	if _, err := s.deps.CIDR.Get(ctx, networkID); err != nil {
		if errors.Is(err, cidrrepo.ErrCIDRNotFound) {
			return Job{}, ErrNetworkNotFound
		}
		return Job{}, err
	}

	tenant := s.tenantID(networkID)
	if tenant == "" {
		jobsList, err := s.deps.Jobs.ListByNetwork(ctx, networkID)
		if err != nil {
			return Job{}, err
		}
		tenant = latestTenantIDFromJobs(jobsList)
	}

	job, err := s.deps.Jobs.Create(ctx, jobsrepo.CreateJobParams{
		NetworkID: networkID,
		TenantID:  tenant,
		Type:      jobsrepo.JobTypeDeprovisionNetwork,
	})
	if err != nil {
		return Job{}, err
	}

	vcName := vclusterName(networkID)
	go s.runDeprovisionNetwork(context.Background(), job.ID, networkID, tenant, vcName)

	return toServiceJob(job), nil
}

// runDeprovisionNetwork tears down in dependency-safe order: revoke secrets before deleting the
// cluster, remove Cilium policies before/after as needed (NotFound is ignored at repo layer),
// then release CIDR. Effect: clearNetworkMaps wipes cached tenant/namespace hints.
func (s *CFProvisionerService) runDeprovisionNetwork(bg context.Context, jobID, networkID, tenantID, vcName string) {
	defer s.recoverJob(bg, jobID, "deprovision_network")

	if err := s.deps.Jobs.UpdateStatus(bg, jobID, jobsrepo.JobStatusRunning, ""); err != nil {
		slog.Error("deprovision_network: update running", "job_id", jobID, "err", err)
		return
	}

	ns := s.vclusterNamespace(networkID)

	if tenantID != "" {
		if err := s.deps.Kubeconfig.Revoke(bg, tenantID); err != nil {
			s.failJob(bg, jobID, err)
			return
		}
	} else {
		slog.Warn("deprovision_network: missing tenantID; skipping kubeconfig revoke", "network_id", networkID)
	}

	// Best-effort: repo returns nil on NotFound; failures here are intentionally ignored so delete proceeds.
	_ = s.deps.Cilium.RemovePolicy(bg, ns, defaultDenyPolicyName(networkID))
	_ = s.deps.Cilium.RemovePolicy(bg, ns, ingressPolicyName(networkID))

	if err := s.deps.VCluster.Delete(bg, vcName); err != nil && !errors.Is(err, vclusterrepo.ErrVClusterNotFound) {
		s.failJob(bg, jobID, err)
		return
	}

	if err := s.deps.CIDR.Release(bg, networkID); err != nil {
		s.failJob(bg, jobID, err)
		return
	}

	s.clearNetworkMaps(networkID)

	if err := s.deps.Jobs.UpdateStatus(bg, jobID, jobsrepo.JobStatusSucceeded, ""); err != nil {
		slog.Error("deprovision_network: update succeeded", "job_id", jobID, "err", err)
	}
}

// ProvisionGateway requires an active network and no existing HTTPRoute for this network (gw-* name).
func (s *CFProvisionerService) ProvisionGateway(ctx context.Context, params ProvisionGatewayParams) (Job, error) {
	networkID := strings.TrimSpace(params.NetworkID)
	if networkID == "" || strings.TrimSpace(params.PublicDNSName) == "" {
		return Job{}, cferrors.Wrap(cferrors.CodeInvalidInput, "networkID and publicDNSName are required", cferrors.ErrInvalidInput)
	}

	st, err := s.GetNetworkStatus(ctx, networkID)
	if err != nil {
		return Job{}, err
	}
	if st.Status != NetworkStatusActive {
		return Job{}, ErrNetworkNotActive
	}

	routeNS := httpRouteNamespace()
	routeName := httpRouteName(networkID)
	// Duplicate gateway: treat an existing route object as conflict (caller should poll job or delete first).
	if _, err := s.deps.Gateway.GetHTTPRoute(ctx, routeNS, routeName); err == nil {
		return Job{}, ErrGatewayExists
	} else if err != nil && !errors.Is(err, gatewayrepo.ErrHTTPRouteNotFound) {
		return Job{}, err
	}

	job, err := s.deps.Jobs.Create(ctx, jobsrepo.CreateJobParams{
		NetworkID: networkID,
		Type:      jobsrepo.JobTypeProvisionGateway,
	})
	if err != nil {
		return Job{}, err
	}

	go s.runProvisionGateway(context.Background(), job.ID, networkID, strings.TrimSpace(params.PublicDNSName), params.TLSEnabled)

	return toServiceJob(job), nil
}

// runProvisionGateway wires Gateway API routing first, then opens Cilium ingress for the declared port.
// Why HTTPRoute before Cilium: the route object should exist before we widen policy to send traffic to it.
func (s *CFProvisionerService) runProvisionGateway(bg context.Context, jobID, networkID, hostname string, tls bool) {
	defer s.recoverJob(bg, jobID, "provision_gateway")

	if err := s.deps.Jobs.UpdateStatus(bg, jobID, jobsrepo.JobStatusRunning, ""); err != nil {
		slog.Error("provision_gateway: update running", "job_id", jobID, "err", err)
		return
	}

	routeNS := httpRouteNamespace()
	routeName := httpRouteName(networkID)
	ns := s.vclusterNamespace(networkID)

	if _, err := s.deps.Gateway.CreateHTTPRoute(bg, gatewayrepo.HTTPRouteParams{
		Name:           routeName,
		Namespace:      routeNS,
		Hostname:       hostname,
		BackendService: gatewayBackendService(),
		BackendPort:    gatewayBackendPort(tls),
		TLSEnabled:     tls,
	}); err != nil {
		s.failJob(bg, jobID, err)
		return
	}

	if err := s.deps.Cilium.ApplyIngressPolicy(bg, ciliumrepo.IngressPolicyParams{
		VClusterNamespace:  ns,
		NetworkID:          networkID,
		PublicEndpointPort: gatewayBackendPort(tls),
	}); err != nil {
		s.failJob(bg, jobID, err)
		return
	}

	if err := s.deps.Jobs.UpdateStatus(bg, jobID, jobsrepo.JobStatusSucceeded, ""); err != nil {
		slog.Error("provision_gateway: update succeeded", "job_id", jobID, "err", err)
	}
}

// GetGatewayStatus reflects HTTPRoute conditions into a small service-level enum.
func (s *CFProvisionerService) GetGatewayStatus(ctx context.Context, networkID string) (GatewayStatus, error) {
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return GatewayStatus{}, cferrors.Wrap(cferrors.CodeInvalidInput, "networkID is required", cferrors.ErrInvalidInput)
	}

	routeNS := httpRouteNamespace()
	routeName := httpRouteName(networkID)
	info, err := s.deps.Gateway.GetHTTPRoute(ctx, routeNS, routeName)
	if err != nil {
		if errors.Is(err, gatewayrepo.ErrHTTPRouteNotFound) {
			return GatewayStatus{}, ErrGatewayNotFound
		}
		return GatewayStatus{}, err
	}

	gst := GatewayStatusProvisioning // repository defaults to pending until parent conditions are observed.
	switch info.Status {
	case gatewayrepo.HTTPRouteStatusReady:
		gst = GatewayStatusActive
	case gatewayrepo.HTTPRouteStatusFailed:
		gst = GatewayStatusFailed
	}

	return GatewayStatus{
		NetworkID:      networkID,
		Status:         gst,
		PublicEndpoint: info.Hostname,
		HTTPRouteName:  info.Name,
		CreatedAt:      time.Time{},
	}, nil
}

// RemoveGateway enqueues async removal after confirming the route still exists (GetGatewayStatus).
func (s *CFProvisionerService) RemoveGateway(ctx context.Context, networkID string) (Job, error) {
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return Job{}, cferrors.Wrap(cferrors.CodeInvalidInput, "networkID is required", cferrors.ErrInvalidInput)
	}

	if _, err := s.GetGatewayStatus(ctx, networkID); err != nil {
		return Job{}, err
	}

	job, err := s.deps.Jobs.Create(ctx, jobsrepo.CreateJobParams{
		NetworkID: networkID,
		Type:      jobsrepo.JobTypeRemoveGateway,
	})
	if err != nil {
		return Job{}, err
	}

	go s.runRemoveGateway(context.Background(), job.ID, networkID)

	return toServiceJob(job), nil
}

// runRemoveGateway deletes the HTTPRoute then drops the ingress Cilium policy (default-deny stays).
func (s *CFProvisionerService) runRemoveGateway(bg context.Context, jobID, networkID string) {
	defer s.recoverJob(bg, jobID, "remove_gateway")

	if err := s.deps.Jobs.UpdateStatus(bg, jobID, jobsrepo.JobStatusRunning, ""); err != nil {
		slog.Error("remove_gateway: update running", "job_id", jobID, "err", err)
		return
	}

	routeNS := httpRouteNamespace()
	routeName := httpRouteName(networkID)
	ns := s.vclusterNamespace(networkID)

	if err := s.deps.Gateway.DeleteHTTPRoute(bg, routeNS, routeName); err != nil {
		s.failJob(bg, jobID, err)
		return
	}

	// Same best-effort semantics as deprovision: repo swallows NotFound.
	_ = s.deps.Cilium.RemovePolicy(bg, ns, ingressPolicyName(networkID))

	if err := s.deps.Jobs.UpdateStatus(bg, jobID, jobsrepo.JobStatusSucceeded, ""); err != nil {
		slog.Error("remove_gateway: update succeeded", "job_id", jobID, "err", err)
	}
}

// GetJob loads a single job row; maps repository not-found to service ErrJobNotFound.
func (s *CFProvisionerService) GetJob(ctx context.Context, jobID string) (Job, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return Job{}, cferrors.Wrap(cferrors.CodeInvalidInput, "jobID is required", cferrors.ErrInvalidInput)
	}
	j, err := s.deps.Jobs.Get(ctx, jobID)
	if err != nil {
		if errors.Is(err, jobsrepo.ErrJobNotFound) {
			return Job{}, ErrJobNotFound
		}
		return Job{}, err
	}
	return toServiceJob(j), nil
}

// ListNetworkJobs returns newest-first rows from the repository as service Jobs, sliced to [offset, offset+limit).
func (s *CFProvisionerService) ListNetworkJobs(ctx context.Context, networkID string, limit, offset int) ([]Job, error) {
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return nil, cferrors.Wrap(cferrors.CodeInvalidInput, "networkID is required", cferrors.ErrInvalidInput)
	}
	list, err := s.deps.Jobs.ListByNetwork(ctx, networkID)
	if err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(list))
	for _, j := range list {
		out = append(out, toServiceJob(j))
	}
	return slicePage(out, offset, limit), nil
}

// ListCIDRAllocations returns a page of all CIDR rows (operator view). Order follows repository ListAll.
func (s *CFProvisionerService) ListCIDRAllocations(ctx context.Context, limit, offset int) ([]cidrrepo.CIDRAllocation, error) {
	all, err := s.deps.CIDR.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return slicePage(all, offset, limit), nil
}

func slicePage[T any](items []T, offset, limit int) []T {
	if offset < 0 {
		offset = 0
	}
	if limit < 1 {
		limit = 1
	}
	if offset >= len(items) {
		return nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	out := make([]T, end-offset)
	copy(out, items[offset:end])
	return out
}

func (s *CFProvisionerService) ProvisionSubnet(ctx context.Context, params ProvisionSubnetParams) (Subnet, error) {
	networkID := strings.TrimSpace(params.NetworkID)
	if networkID == "" {
		return Subnet{}, cferrors.Wrap(cferrors.CodeInvalidInput, "networkID is required", cferrors.ErrInvalidInput)
	}
	typ := strings.ToLower(strings.TrimSpace(params.Type))
	if typ != string(cfnetwork.SubnetTypePrivate) && typ != string(cfnetwork.SubnetTypePublic) {
		return Subnet{}, cferrors.Wrap(cferrors.CodeInvalidInput, "type must be private or public", cferrors.ErrInvalidInput)
	}
	cidr, err := normalizeSubnetCIDR(params.CIDR)
	if err != nil {
		return Subnet{}, err
	}
	if s.deps.Subnets == nil {
		return Subnet{}, cferrors.Wrap(cferrors.CodeInternal, "subnets repository is required", cferrors.ErrInternal)
	}
	sub, err := s.deps.Subnets.Create(ctx, subnetsrepo.CreateSubnetParams{
		NetworkID: networkID,
		Type:      typ,
		CIDR:      cidr,
		Zone:      strings.TrimSpace(params.Zone),
	})
	if err != nil {
		return Subnet{}, err
	}
	return toServiceSubnet(sub), nil
}

func normalizeSubnetCIDR(cidr string) (string, error) {
	cidr = strings.TrimSpace(cidr)
	if cidr == "" {
		return "", cferrors.Wrap(cferrors.CodeInvalidInput, "cidr is required", cferrors.ErrInvalidInput)
	}
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", cferrors.Wrap(cferrors.CodeInvalidInput, "invalid CIDR", cferrors.ErrInvalidInput)
	}
	if ipnet.IP.To4() == nil {
		return "", cferrors.Wrap(cferrors.CodeInvalidInput, "IPv6 CIDR not supported", cferrors.ErrInvalidInput)
	}
	return ipnet.String(), nil
}

func (s *CFProvisionerService) ListSubnets(ctx context.Context, networkID string) ([]Subnet, error) {
	networkID = strings.TrimSpace(networkID)
	if networkID == "" {
		return nil, cferrors.Wrap(cferrors.CodeInvalidInput, "networkID is required", cferrors.ErrInvalidInput)
	}
	if s.deps.Subnets == nil {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "subnets repository is required", cferrors.ErrInternal)
	}
	list, err := s.deps.Subnets.ListByNetwork(ctx, networkID)
	if err != nil {
		return nil, err
	}
	out := make([]Subnet, 0, len(list))
	for _, sub := range list {
		out = append(out, toServiceSubnet(sub))
	}
	return out, nil
}

func toServiceSubnet(sub subnetsrepo.Subnet) Subnet {
	return Subnet{
		ID:        sub.ID,
		NetworkID: sub.NetworkID,
		Type:      sub.Type,
		CIDR:      sub.CIDR,
		Zone:      sub.Zone,
		CreatedAt: sub.CreatedAt,
	}
}
