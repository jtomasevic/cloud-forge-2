package accounts

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gocql/gocql"
	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

const listAccountsMaxScan = 500

type accountsRepository struct {
	session *scylladbclient.Session
}

func (r *accountsRepository) Insert(ctx context.Context, row AccountRow) error {
	if err := r.session.Query(cqlInsertAccount,
		row.ID, row.Email, row.Status, row.PasswordHash, row.CreatedAt, row.UpdatedAt,
	).WithContext(ctx).Exec(); err != nil {
		return mapInsertErr(err, ErrAccountExists)
	}
	if err := r.session.Query(cqlInsertAccountByEmail, row.Email, row.ID, row.Status).WithContext(ctx).Exec(); err != nil {
		return mapInsertErr(err, ErrAccountExists)
	}
	return nil
}

func (r *accountsRepository) GetByID(ctx context.Context, id string) (AccountRow, error) {
	u, err := parseUUID(id)
	if err != nil {
		return AccountRow{}, err
	}
	var row AccountRow
	if err := r.session.Query(cqlSelectAccountByID, u).WithContext(ctx).Scan(
		&row.ID, &row.Email, &row.Status, &row.PasswordHash, &row.CreatedAt, &row.UpdatedAt,
	); err != nil {
		return AccountRow{}, mapScanErr(err, ErrAccountNotFound)
	}
	return row, nil
}

func (r *accountsRepository) GetByEmail(ctx context.Context, email string) (AccountRow, error) {
	var accountID gocql.UUID
	var denormStatus string
	if err := r.session.Query(cqlSelectAccountByEmailLookup, email).WithContext(ctx).Scan(&accountID, &denormStatus); err != nil {
		return AccountRow{}, mapScanErr(err, ErrAccountNotFound)
	}
	row, err := r.GetByID(ctx, accountID.String())
	if err != nil {
		return AccountRow{}, err
	}
	_ = denormStatus // v1: authoritative fields come from accounts
	return row, nil
}

func (r *accountsRepository) List(ctx context.Context, limit, offset int) ([]AccountRow, int, error) {
	if limit <= 0 {
		return nil, 0, cferrors.Wrap(cferrors.CodeInvalidInput, "limit must be positive", cferrors.ErrInvalidInput)
	}
	if offset < 0 {
		return nil, 0, cferrors.Wrap(cferrors.CodeInvalidInput, "offset must be non-negative", cferrors.ErrInvalidInput)
	}
	if offset+limit > listAccountsMaxScan {
		return nil, 0, cferrors.New(cferrors.CodeInvalidInput, "offset+limit exceeds maximum scan window for v1 list")
	}
	// v1: accounts table is keyed only by id; listing requires ALLOW FILTERING.
	// Suitable for small dev datasets only.
	fetch := offset + limit
	iter := r.session.Query(cqlSelectAccountsList, fetch).WithContext(ctx).Iter()
	var buf []AccountRow
	var row AccountRow
	for iter.Scan(&row.ID, &row.Email, &row.Status, &row.PasswordHash, &row.CreatedAt, &row.UpdatedAt) {
		buf = append(buf, row)
	}
	if err := iter.Close(); err != nil {
		return nil, 0, mapInternalErr(err)
	}
	if offset > len(buf) {
		return nil, -1, nil
	}
	end := offset + limit
	if end > len(buf) {
		end = len(buf)
	}
	out := buf[offset:end]
	return out, -1, nil
}

func (r *accountsRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	u, err := parseUUID(id)
	if err != nil {
		return err
	}
	row, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := r.session.Query(cqlUpdateAccountStatus, status, now, u).WithContext(ctx).Exec(); err != nil {
		return mapInternalErr(err)
	}
	if err := r.session.Query(cqlUpdateAccountByEmailStatus, status, row.Email).WithContext(ctx).Exec(); err != nil {
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
