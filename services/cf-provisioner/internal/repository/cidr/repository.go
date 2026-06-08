package cidr

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gocql/gocql"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

type cidrRepository struct {
	session *scylladbclient.Session
}

func (r *cidrRepository) Allocate(ctx context.Context, params AllocateParams) (CIDRAllocation, error) {
	if strings.TrimSpace(params.NetworkID) == "" {
		return CIDRAllocation{}, cferrors.Wrap(cferrors.CodeInvalidInput, "networkID is required", cferrors.ErrInvalidInput)
	}
	nid, err := parseUUID(params.NetworkID)
	if err != nil {
		return CIDRAllocation{}, err
	}
	if _, err := r.getRow(ctx, nid); err == nil {
		return CIDRAllocation{}, ErrCIDRAlreadyAllocated
	} else if !errors.Is(err, ErrCIDRNotFound) {
		return CIDRAllocation{}, err
	}
	all, err := r.listAllRows(ctx)
	if err != nil {
		return CIDRAllocation{}, err
	}
	pod, svc, err := resolveAllocateCID(params, toAllocations(all))
	if err != nil {
		return CIDRAllocation{}, err
	}
	at := time.Now().UTC()
	if err := r.session.Query(cqlInsertAllocation, nid, pod, svc, at).WithContext(ctx).Exec(); err != nil {
		return CIDRAllocation{}, mapInternalErr(err)
	}
	return CIDRAllocation{
		NetworkID:   params.NetworkID,
		PodCIDR:     pod,
		SvcCIDR:     svc,
		AllocatedAt: at,
	}, nil
}

func (r *cidrRepository) Get(ctx context.Context, networkID string) (CIDRAllocation, error) {
	nid, err := parseUUID(networkID)
	if err != nil {
		return CIDRAllocation{}, err
	}
	return r.getRow(ctx, nid)
}

func (r *cidrRepository) getRow(ctx context.Context, nid gocql.UUID) (CIDRAllocation, error) {
	var pid gocql.UUID
	var pod, svc string
	var at time.Time
	if err := r.session.Query(cqlSelectAllocationByNetwork, nid).WithContext(ctx).Scan(&pid, &pod, &svc, &at); err != nil {
		return CIDRAllocation{}, mapScanErr(err, ErrCIDRNotFound)
	}
	return CIDRAllocation{
		NetworkID:   pid.String(),
		PodCIDR:     pod,
		SvcCIDR:     svc,
		AllocatedAt: at,
	}, nil
}

func (r *cidrRepository) Release(ctx context.Context, networkID string) error {
	nid, err := parseUUID(networkID)
	if err != nil {
		return err
	}
	if err := r.session.Query(cqlDeleteAllocation, nid).WithContext(ctx).Exec(); err != nil {
		return mapInternalErr(err)
	}
	return nil
}

func (r *cidrRepository) ListAll(ctx context.Context) ([]CIDRAllocation, error) {
	rows, err := r.listAllRows(ctx)
	if err != nil {
		return nil, err
	}
	return toAllocations(rows), nil
}

type cidrRow struct {
	networkID gocql.UUID
	podCIDR   string
	svcCIDR   string
	at        time.Time
}

func (r *cidrRepository) listAllRows(ctx context.Context) ([]cidrRow, error) {
	iter := r.session.Query(cqlSelectAllAllocations).WithContext(ctx).Iter()
	var out []cidrRow
	var row cidrRow
	for iter.Scan(&row.networkID, &row.podCIDR, &row.svcCIDR, &row.at) {
		out = append(out, row)
	}
	if err := iter.Close(); err != nil {
		return nil, mapInternalErr(err)
	}
	return out, nil
}

func toAllocations(rows []cidrRow) []CIDRAllocation {
	out := make([]CIDRAllocation, 0, len(rows))
	for _, r := range rows {
		out = append(out, CIDRAllocation{
			NetworkID:   r.networkID.String(),
			PodCIDR:     r.podCIDR,
			SvcCIDR:     r.svcCIDR,
			AllocatedAt: r.at,
		})
	}
	return out
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
