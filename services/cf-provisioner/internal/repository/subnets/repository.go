package subnets

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/gocql/gocql"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	cfnetwork "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/network"
	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

type subnetsRepository struct {
	session *scylladbclient.Session
}

func (r *subnetsRepository) Create(ctx context.Context, params CreateSubnetParams) (Subnet, error) {
	if strings.TrimSpace(params.NetworkID) == "" {
		return Subnet{}, cferrors.Wrap(cferrors.CodeInvalidInput, "networkID is required", cferrors.ErrInvalidInput)
	}
	nid, err := parseRequiredUUID("networkID", params.NetworkID)
	if err != nil {
		return Subnet{}, err
	}
	typ, err := normalizeType(params.Type)
	if err != nil {
		return Subnet{}, err
	}
	cidr, err := normalizeCIDR(params.CIDR)
	if err != nil {
		return Subnet{}, err
	}
	sid, err := gocql.RandomUUID()
	if err != nil {
		return Subnet{}, mapInternalErr(err)
	}
	zone := strings.TrimSpace(params.Zone)
	now := time.Now().UTC()

	// Reserve the network/CIDR key first using a lightweight transaction. This is the only key that
	// must be unique for the task; primary and list rows are denormalized copies written after it.
	applied, err := r.session.Query(cqlInsertSubnetByNetworkCIDR, nid, cidr, sid, typ, zone, now).
		WithContext(ctx).
		MapScanCAS(map[string]interface{}{})
	if err != nil {
		return Subnet{}, mapInternalErr(err)
	}
	if !applied {
		return Subnet{}, ErrSubnetCIDRExists
	}

	if err := r.session.Query(cqlInsertSubnet, sid, nid, typ, cidr, zone, now).WithContext(ctx).Exec(); err != nil {
		return Subnet{}, mapInternalErr(err)
	}
	// The remaining denormalized row keeps list reads cheap. Scylla does not make this write atomic
	// with the uniqueness reservation above; a future repair pass can reconcile rare partial writes.
	if err := r.session.Query(cqlInsertSubnetByNetwork, nid, sid, typ, cidr, zone, now).WithContext(ctx).Exec(); err != nil {
		return Subnet{}, mapInternalErr(err)
	}

	return Subnet{
		ID:        sid.String(),
		NetworkID: nid.String(),
		Type:      typ,
		CIDR:      cidr,
		Zone:      zone,
		CreatedAt: now,
	}, nil
}

func (r *subnetsRepository) GetByID(ctx context.Context, networkID, subnetID string) (Subnet, error) {
	nid, err := parseRequiredUUID("networkID", networkID)
	if err != nil {
		return Subnet{}, err
	}
	sid, err := parseRequiredUUID("subnetID", subnetID)
	if err != nil {
		return Subnet{}, err
	}
	sub, err := r.getByID(ctx, sid)
	if err != nil {
		return Subnet{}, err
	}
	if sub.NetworkID != nid.String() {
		return Subnet{}, ErrSubnetNotFound
	}
	return sub, nil
}

func (r *subnetsRepository) ListByNetwork(ctx context.Context, networkID string) ([]Subnet, error) {
	nid, err := parseRequiredUUID("networkID", networkID)
	if err != nil {
		return nil, err
	}
	iter := r.session.Query(cqlSelectSubnetsByNetwork, nid).WithContext(ctx).Iter()
	out := make([]Subnet, 0)
	var sid gocql.UUID
	var typ, cidr, zone string
	var createdAt time.Time
	for iter.Scan(&sid, &typ, &cidr, &zone, &createdAt) {
		out = append(out, Subnet{
			ID:        sid.String(),
			NetworkID: nid.String(),
			Type:      typ,
			CIDR:      cidr,
			Zone:      zone,
			CreatedAt: createdAt,
		})
	}
	if err := iter.Close(); err != nil {
		return nil, mapInternalErr(err)
	}
	return out, nil
}

func (r *subnetsRepository) getByID(ctx context.Context, sid gocql.UUID) (Subnet, error) {
	var rowID, nid gocql.UUID
	var typ, cidr, zone string
	var createdAt time.Time
	if err := r.session.Query(cqlSelectSubnetByID, sid).WithContext(ctx).Scan(
		&rowID, &nid, &typ, &cidr, &zone, &createdAt,
	); err != nil {
		return Subnet{}, mapScanErr(err, ErrSubnetNotFound)
	}
	return Subnet{
		ID:        rowID.String(),
		NetworkID: nid.String(),
		Type:      typ,
		CIDR:      cidr,
		Zone:      zone,
		CreatedAt: createdAt,
	}, nil
}

//func (r *subnetsRepository) getByNetworkCIDR(ctx context.Context, nid gocql.UUID, cidr string) (Subnet, error) {
//	var sid gocql.UUID
//	var typ, zone string
//	var createdAt time.Time
//	if err := r.session.Query(cqlSelectSubnetByNetworkCIDR, nid, cidr).WithContext(ctx).Scan(
//		&sid, &typ, &zone, &createdAt,
//	); err != nil {
//		return Subnet{}, mapScanErr(err, ErrSubnetNotFound)
//	}
//	return Subnet{
//		ID:        sid.String(),
//		NetworkID: nid.String(),
//		Type:      typ,
//		CIDR:      cidr,
//		Zone:      zone,
//		CreatedAt: createdAt,
//	}, nil
//}

func parseUUID(id string) (gocql.UUID, error) {
	u, err := gocql.ParseUUID(strings.TrimSpace(id))
	if err != nil {
		return gocql.UUID{}, cferrors.Wrap(cferrors.CodeInvalidInput, "invalid UUID", cferrors.ErrInvalidInput)
	}
	return u, nil
}

func parseRequiredUUID(field, id string) (gocql.UUID, error) {
	if strings.TrimSpace(id) == "" {
		return gocql.UUID{}, cferrors.Wrap(cferrors.CodeInvalidInput, field+" is required", cferrors.ErrInvalidInput)
	}
	return parseUUID(id)
}

func normalizeType(typ string) (string, error) {
	typ = strings.ToLower(strings.TrimSpace(typ))
	if typ != string(cfnetwork.SubnetTypePrivate) && typ != string(cfnetwork.SubnetTypePublic) {
		return "", cferrors.Wrap(cferrors.CodeInvalidInput, "type must be private or public", cferrors.ErrInvalidInput)
	}
	return typ, nil
}

func normalizeCIDR(cidr string) (string, error) {
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
