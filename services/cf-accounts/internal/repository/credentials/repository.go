package credentials

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gocql/gocql"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

const listAPIKeysMaxFetch = 500

type credentialsRepository struct {
	session *scylladbclient.Session
}

func (r *credentialsRepository) Insert(ctx context.Context, row APIKeyRow) error {
	var revoked any
	if row.RevokedAt != nil {
		revoked = *row.RevokedAt
	} else {
		revoked = nil
	}

	if err := r.session.Query(cqlInsertAPIKey,
		row.ID, row.AccountID, row.KeyHash, row.KeyPrefix, row.CreatedAt, revoked,
	).WithContext(ctx).Exec(); err != nil {
		return mapInsertErr(err)
	}
	if err := r.session.Query(cqlInsertAPIKeyByAccount,
		row.AccountID, row.ID, row.KeyPrefix, row.CreatedAt, revoked,
	).WithContext(ctx).Exec(); err != nil {
		return mapInsertErr(err)
	}
	if err := r.session.Query(cqlInsertAPIKeyByHash, row.KeyHash, row.ID, row.AccountID, revoked).WithContext(ctx).Exec(); err != nil {
		return mapInsertErr(err)
	}
	return nil
}

func (r *credentialsRepository) GetByID(ctx context.Context, id string) (APIKeyRow, error) {
	u, err := parseUUID(id)
	if err != nil {
		return APIKeyRow{}, err
	}
	var row APIKeyRow
	var revoked time.Time
	if err := r.session.Query(cqlSelectAPIKeyByID, u).WithContext(ctx).Scan(
		&row.ID, &row.AccountID, &row.KeyHash, &row.KeyPrefix, &row.CreatedAt, &revoked,
	); err != nil {
		return APIKeyRow{}, mapScanErr(err, ErrCredentialNotFound)
	}
	if !revoked.IsZero() {
		t := revoked
		row.RevokedAt = &t
	}
	return row, nil
}

func (r *credentialsRepository) GetByHash(ctx context.Context, keyHash string) (APIKeyRow, error) {
	var keyID gocql.UUID
	var accountID gocql.UUID
	var revokedAt time.Time
	if err := r.session.Query(cqlSelectAPIKeyByHashLookup, keyHash).WithContext(ctx).Scan(&keyID, &accountID, &revokedAt); err != nil {
		return APIKeyRow{}, mapScanErr(err, ErrCredentialNotFound)
	}
	_ = accountID
	if err := errIfCredentialRevoked(revokedAt); err != nil {
		return APIKeyRow{}, err
	}
	return r.GetByID(ctx, keyID.String())
}

// errIfCredentialRevoked maps a non-null CQL timestamp (represented as non-zero time)
// to ErrCredentialRevoked for the hot GetByHash path.
func errIfCredentialRevoked(revokedAt time.Time) error {
	if !revokedAt.IsZero() {
		return ErrCredentialRevoked
	}
	return nil
}

func (r *credentialsRepository) ListByAccount(ctx context.Context, accountID string) ([]APIKeyRow, error) {
	aid, err := parseUUID(accountID)
	if err != nil {
		return nil, err
	}
	iter := r.session.Query(cqlSelectAPIKeyIDsByAccount, aid, listAPIKeysMaxFetch).WithContext(ctx).Iter()
	var ids []gocql.UUID
	var kid gocql.UUID
	var prefix string
	var createdAt time.Time
	var revokedAt time.Time
	for iter.Scan(&kid, &prefix, &createdAt, &revokedAt) {
		_ = prefix
		_ = createdAt
		_ = revokedAt
		ids = append(ids, kid)
	}
	if err := iter.Close(); err != nil {
		return nil, mapInternalErr(err)
	}
	return r.loadAPIKeysByIDs(ctx, ids)
}

func (r *credentialsRepository) loadAPIKeysByIDs(ctx context.Context, ids []gocql.UUID) ([]APIKeyRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph := placeholders(len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	q := cqlSelectAPIKeysByIDInPrefix + ph + ")"
	iter := r.session.Query(q, args...).WithContext(ctx).Iter()
	byID := make(map[string]APIKeyRow)
	for {
		var row APIKeyRow
		var revoked time.Time
		if !iter.Scan(&row.ID, &row.AccountID, &row.KeyHash, &row.KeyPrefix, &row.CreatedAt, &revoked) {
			break
		}
		if !revoked.IsZero() {
			t := revoked
			row.RevokedAt = &t
		}
		byID[row.ID.String()] = row
	}
	if err := iter.Close(); err != nil {
		return nil, mapInternalErr(err)
	}
	out := make([]APIKeyRow, 0, len(ids))
	for _, id := range ids {
		if row, ok := byID[id.String()]; ok {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *credentialsRepository) Revoke(ctx context.Context, id string, revokedAt time.Time) error {
	u, err := parseUUID(id)
	if err != nil {
		return err
	}
	row, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row.RevokedAt != nil && !row.RevokedAt.IsZero() {
		return ErrCredentialRevoked
	}

	if err := r.session.Query(cqlUpdateAPIKeyRevoke, revokedAt, u).WithContext(ctx).Exec(); err != nil {
		return mapInternalErr(err)
	}
	if err := r.session.Query(cqlUpdateAPIKeyByHashRevoke, revokedAt, row.KeyHash).WithContext(ctx).Exec(); err != nil {
		return mapInternalErr(err)
	}
	if err := r.session.Query(cqlUpdateAPIKeyByAccountRevoke, revokedAt, row.AccountID, row.CreatedAt, row.ID).WithContext(ctx).Exec(); err != nil {
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

func mapInsertErr(err error) error {
	if err == nil {
		return nil
	}
	return mapInternalErr(err)
}

func mapInternalErr(err error) error {
	if err == nil {
		return nil
	}
	return cferrors.Wrap(cferrors.CodeInternal, "scylladb operation failed", err)
}
