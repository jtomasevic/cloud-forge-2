package tenants

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gocql/gocql"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

const listTenantsMaxFetch = 500

type tenantsRepository struct {
	session *scylladbclient.Session
}

func (r *tenantsRepository) Insert(ctx context.Context, row TenantRow) error {
	if err := r.session.Query(cqlInsertTenant,
		row.ID, row.AccountID, row.Slug, row.Region, row.Status, row.CreatedAt, row.UpdatedAt,
	).WithContext(ctx).Exec(); err != nil {
		return mapInsertErr(err, ErrSlugTaken)
	}
	if err := r.session.Query(cqlInsertTenantByAccount,
		row.AccountID, row.ID, row.Slug, row.Status, row.CreatedAt,
	).WithContext(ctx).Exec(); err != nil {
		return mapInsertErr(err, ErrSlugTaken)
	}
	if err := r.session.Query(cqlInsertTenantBySlug, row.Slug, row.ID, row.AccountID, row.Status).WithContext(ctx).Exec(); err != nil {
		return mapInsertErr(err, ErrSlugTaken)
	}
	return nil
}

func (r *tenantsRepository) GetByID(ctx context.Context, id string) (TenantRow, error) {
	u, err := parseUUID(id)
	if err != nil {
		return TenantRow{}, err
	}
	var row TenantRow
	if err := r.session.Query(cqlSelectTenantByID, u).WithContext(ctx).Scan(
		&row.ID, &row.AccountID, &row.Slug, &row.Region, &row.Status, &row.CreatedAt, &row.UpdatedAt,
	); err != nil {
		return TenantRow{}, mapScanErr(err, ErrTenantNotFound)
	}
	return row, nil
}

func (r *tenantsRepository) GetBySlug(ctx context.Context, slug string) (TenantRow, error) {
	var tid gocql.UUID
	var accountID gocql.UUID
	var denormStatus string
	if err := r.session.Query(cqlSelectTenantBySlugLookup, slug).WithContext(ctx).Scan(&tid, &accountID, &denormStatus); err != nil {
		return TenantRow{}, mapScanErr(err, ErrTenantNotFound)
	}
	_ = accountID
	_ = denormStatus
	return r.GetByID(ctx, tid.String())
}

func (r *tenantsRepository) ListByAccount(ctx context.Context, accountID string, limit, offset int) ([]TenantRow, error) {
	if limit <= 0 {
		return nil, cferrors.Wrap(cferrors.CodeInvalidInput, "limit must be positive", cferrors.ErrInvalidInput)
	}
	if offset < 0 {
		return nil, cferrors.Wrap(cferrors.CodeInvalidInput, "offset must be non-negative", cferrors.ErrInvalidInput)
	}
	if offset+limit > listTenantsMaxFetch {
		return nil, cferrors.New(cferrors.CodeInvalidInput, "offset+limit exceeds maximum fetch window")
	}
	aid, err := parseUUID(accountID)
	if err != nil {
		return nil, err
	}
	fetch := offset + limit
	iter := r.session.Query(cqlSelectTenantKeysByAccount, aid, fetch).WithContext(ctx).Iter()
	var keys []gocql.UUID
	var tenantID gocql.UUID
	var slug, status string
	var createdAt time.Time
	for iter.Scan(&tenantID, &slug, &status, &createdAt) {
		_ = slug
		_ = status
		keys = append(keys, tenantID)
	}
	if err := iter.Close(); err != nil {
		return nil, mapInternalErr(err)
	}
	if offset >= len(keys) {
		return nil, nil
	}
	keys = keys[offset:]
	if len(keys) > limit {
		keys = keys[:limit]
	}
	return r.loadTenantsByIDs(ctx, keys)
}

func (r *tenantsRepository) loadTenantsByIDs(ctx context.Context, ids []gocql.UUID) ([]TenantRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph := placeholders(len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	q := cqlSelectTenantsByIDInPrefix + ph + ")"
	iter := r.session.Query(q, args...).WithContext(ctx).Iter()
	byID := make(map[string]TenantRow)
	var row TenantRow
	for iter.Scan(&row.ID, &row.AccountID, &row.Slug, &row.Region, &row.Status, &row.CreatedAt, &row.UpdatedAt) {
		byID[row.ID.String()] = row
	}
	if err := iter.Close(); err != nil {
		return nil, mapInternalErr(err)
	}
	out := make([]TenantRow, 0, len(ids))
	for _, id := range ids {
		if row, ok := byID[id.String()]; ok {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *tenantsRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	u, err := parseUUID(id)
	if err != nil {
		return err
	}
	row, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := r.session.Query(cqlUpdateTenantStatus, status, now, u).WithContext(ctx).Exec(); err != nil {
		return mapInternalErr(err)
	}
	if err := r.session.Query(cqlUpdateTenantBySlugStatus, status, row.Slug).WithContext(ctx).Exec(); err != nil {
		return mapInternalErr(err)
	}
	if err := r.session.Query(cqlUpdateTenantByAccountStatus, status, row.AccountID, row.CreatedAt, row.ID).WithContext(ctx).Exec(); err != nil {
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

func mapInsertErr(err error, exists *cferrors.CFError) error {
	if err == nil {
		return nil
	}
	if isDuplicateOrExists(err) {
		return exists
	}
	return mapInternalErr(err)
}

func isDuplicateOrExists(err error) bool {
	var re gocql.RequestError
	if !errors.As(err, &re) {
		return false
	}
	msg := strings.ToLower(re.Message())
	switch re.Code() {
	case gocql.ErrCodeAlreadyExists:
		return true
	case gocql.ErrCodeInvalid:
		return strings.Contains(msg, "exist") || strings.Contains(msg, "duplicate")
	default:
		return false
	}
}

func mapInternalErr(err error) error {
	if err == nil {
		return nil
	}
	return cferrors.Wrap(cferrors.CodeInternal, "scylladb operation failed", err)
}
