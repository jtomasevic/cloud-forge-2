package jobs

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gocql/gocql"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

type jobsRepository struct {
	session *scylladbclient.Session
}

// zeroTenantID is stored in provisioning_jobs.tenant_id until the control plane threads tenant context into job creation.
var zeroTenantID gocql.UUID

func (r *jobsRepository) Create(ctx context.Context, params CreateJobParams) (Job, error) {
	if strings.TrimSpace(params.NetworkID) == "" {
		return Job{}, cferrors.Wrap(cferrors.CodeInvalidInput, "networkID is required", cferrors.ErrInvalidInput)
	}
	if strings.TrimSpace(string(params.Type)) == "" {
		return Job{}, cferrors.Wrap(cferrors.CodeInvalidInput, "job type is required", cferrors.ErrInvalidInput)
	}
	nid, err := parseUUID(params.NetworkID)
	if err != nil {
		return Job{}, err
	}
	jid, err := gocql.RandomUUID()
	if err != nil {
		return Job{}, mapInternalErr(err)
	}
	now := time.Now().UTC()
	status := string(JobStatusPending)
	jt := string(params.Type)
	if err := r.session.Query(cqlInsertJob, jid, zeroTenantID, nid, jt, status, "", now, now).WithContext(ctx).Exec(); err != nil {
		return Job{}, mapInternalErr(err)
	}
	if err := r.session.Query(cqlInsertJobByNetwork, nid, jid, jt, status, now).WithContext(ctx).Exec(); err != nil {
		return Job{}, mapInternalErr(err)
	}
	return Job{
		ID:           jid.String(),
		NetworkID:    params.NetworkID,
		Type:         params.Type,
		Status:       JobStatusPending,
		ErrorMessage: "",
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (r *jobsRepository) Get(ctx context.Context, jobID string) (Job, error) {
	id, err := parseUUID(jobID)
	if err != nil {
		return Job{}, err
	}
	return r.scanJob(ctx, id)
}

func (r *jobsRepository) scanJob(ctx context.Context, id gocql.UUID) (Job, error) {
	var rowID gocql.UUID
	var tenantID gocql.UUID
	var nid gocql.UUID
	var jt, status, errMsg string
	var createdAt, updatedAt time.Time
	if err := r.session.Query(cqlSelectJobByID, id).WithContext(ctx).Scan(
		&rowID, &tenantID, &nid, &jt, &status, &errMsg, &createdAt, &updatedAt,
	); err != nil {
		return Job{}, mapScanErr(err, ErrJobNotFound)
	}
	_ = tenantID
	return Job{
		ID:           rowID.String(),
		NetworkID:    nid.String(),
		Type:         JobType(jt),
		Status:       JobStatus(status),
		ErrorMessage: errMsg,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}, nil
}

func (r *jobsRepository) ListByNetwork(ctx context.Context, networkID string) ([]Job, error) {
	nid, err := parseUUID(networkID)
	if err != nil {
		return nil, err
	}
	iter := r.session.Query(cqlSelectJobsByNetwork, nid).WithContext(ctx).Iter()
	var out []Job
	var jid gocql.UUID
	var jt, status string
	var createdAt time.Time
	for iter.Scan(&jid, &jt, &status, &createdAt) {
		full, err := r.scanJob(ctx, jid)
		if err != nil {
			if errors.Is(err, ErrJobNotFound) {
				// Denormalized row exists without primary (repair inconsistency); skip.
				continue
			}
			return nil, err
		}
		out = append(out, full)
	}
	if err := iter.Close(); err != nil {
		return nil, mapInternalErr(err)
	}
	return out, nil
}

func (r *jobsRepository) UpdateStatus(ctx context.Context, jobID string, status JobStatus, errorMsg string) error {
	id, err := parseUUID(jobID)
	if err != nil {
		return err
	}
	job, err := r.scanJob(ctx, id)
	if err != nil {
		return err
	}
	nid, err := parseUUID(job.NetworkID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	st := string(status)
	if err := r.session.Query(cqlUpdateJobStatus, st, errorMsg, now, id).WithContext(ctx).Exec(); err != nil {
		return mapInternalErr(err)
	}
	// Best-effort denormalized update; same eventual-consistency caveat as CF-Accounts networks_by_tenant.
	if err := r.session.Query(cqlUpdateJobByNetworkStatus, st, nid, job.CreatedAt, id).WithContext(ctx).Exec(); err != nil {
		return mapInternalErr(err)
	}
	return nil
}

func parseUUID(id string) (gocql.UUID, error) {
	u, err := gocql.ParseUUID(id)
	if err != nil {
		return gocql.UUID{}, cferrors.Wrap(cferrors.CodeInvalidInput, "invalid UUID", cferrors.ErrInvalidInput)
	}
	return u, nil
}

func mapScanErr(err error, notFound *cferrors.CFError) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gocql.ErrNotFound) {
		return notFound
	}
	return mapInternalErr(err)
}

func mapInternalErr(err error) error {
	if err == nil {
		return nil
	}
	return cferrors.Wrap(cferrors.CodeInternal, "scylladb operation failed", err)
}
