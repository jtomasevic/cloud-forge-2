package networks

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gocql/gocql"
	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

const listNetworksMaxFetch = 500

type networksRepository struct {
	session *scylladbclient.Session
}

func (r *networksRepository) Insert(ctx context.Context, row NetworkRow) error {
	if err := r.session.Query(cqlInsertNetwork,
		row.ID, row.TenantID, row.Region, row.PodCIDR, row.SvcCIDR, row.Status, row.CreatedAt, row.UpdatedAt,
	).WithContext(ctx).Exec(); err != nil {
		return mapInternalErr(err)
	}
	if err := r.session.Query(cqlInsertNetworkByTenant,
		row.TenantID, row.ID, row.Region, row.Status, row.CreatedAt,
	).WithContext(ctx).Exec(); err != nil {
		return mapInternalErr(err)
	}
	return nil
}

func (r *networksRepository) GetByID(ctx context.Context, id string) (NetworkRow, error) {
	u, err := parseUUID(id)
	if err != nil {
		return NetworkRow{}, err
	}
	var row NetworkRow
	if err := r.session.Query(cqlSelectNetworkByID, u).WithContext(ctx).Scan(
		&row.ID, &row.TenantID, &row.Region, &row.PodCIDR, &row.SvcCIDR, &row.Status, &row.CreatedAt, &row.UpdatedAt,
	); err != nil {
		return NetworkRow{}, mapScanErr(err, ErrNetworkNotFound)
	}
	return row, nil
}

func (r *networksRepository) ListByTenant(ctx context.Context, tenantID string) ([]NetworkRow, error) {
	tid, err := parseUUID(tenantID)
	if err != nil {
		return nil, err
	}
	iter := r.session.Query(cqlSelectNetworkIDsByTenant, tid, listNetworksMaxFetch).WithContext(ctx).Iter()
	var ids []gocql.UUID
	var nid gocql.UUID
	var region string
	var status string
	var createdAt time.Time
	for iter.Scan(&nid, &region, &status, &createdAt) {
		_ = region
		_ = status
		_ = createdAt
		ids = append(ids, nid)
	}
	if err := iter.Close(); err != nil {
		return nil, mapInternalErr(err)
	}
	return r.loadNetworksByIDs(ctx, ids)
}

func (r *networksRepository) loadNetworksByIDs(ctx context.Context, ids []gocql.UUID) ([]NetworkRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph := placeholders(len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	q := cqlSelectNetworksByIDInPrefix + ph + ")"
	iter := r.session.Query(q, args...).WithContext(ctx).Iter()
	byID := make(map[string]NetworkRow)
	var row NetworkRow
	for iter.Scan(&row.ID, &row.TenantID, &row.Region, &row.PodCIDR, &row.SvcCIDR, &row.Status, &row.CreatedAt, &row.UpdatedAt) {
		byID[row.ID.String()] = row
	}
	if err := iter.Close(); err != nil {
		return nil, mapInternalErr(err)
	}
	out := make([]NetworkRow, 0, len(ids))
	for _, id := range ids {
		if row, ok := byID[id.String()]; ok {
			out = append(out, row)
		}
	}
	return out, nil
}

// UpdateStatus updates the primary networks row and best-effort updates the
// denormalized networks_by_tenant row. If the lookup row is missing, the
// primary row still reflects the new status (eventual consistency for reads
// that only hit networks_by_tenant until repaired).
func (r *networksRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	u, err := parseUUID(id)
	if err != nil {
		return err
	}
	row, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := r.session.Query(cqlUpdateNetworkStatus, status, now, u).WithContext(ctx).Exec(); err != nil {
		return mapInternalErr(err)
	}
	if err := r.session.Query(cqlUpdateNetworkByTenantStatus, status, row.TenantID, row.CreatedAt, row.ID).WithContext(ctx).Exec(); err != nil {
		return mapInternalErr(err)
	}
	return nil
}

func placeholders(n int) string {
	b := strings.Builder{}
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("?")
	}
	return b.String()
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
