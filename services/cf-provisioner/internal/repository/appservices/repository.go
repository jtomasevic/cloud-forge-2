package appservices

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gocql/gocql"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	scylladbclient "github.com/jtomasevic/cloud-forge-2/libs/scylladb/pkg/client"
)

type cqlStore interface {
	mapScanCAS(ctx context.Context, stmt string, values ...any) (bool, error)
	exec(ctx context.Context, stmt string, values ...any) error
	scan(ctx context.Context, stmt string, values []any, dest ...any) error
	iter(ctx context.Context, stmt string, values ...any) cqlIterator
}

type cqlIterator interface {
	Scan(dest ...any) bool
	Close() error
}

type sessionStore struct {
	session *scylladbclient.Session
}

func (s sessionStore) mapScanCAS(ctx context.Context, stmt string, values ...any) (bool, error) {
	return s.session.Query(stmt, values...).WithContext(ctx).MapScanCAS(map[string]interface{}{})
}

func (s sessionStore) exec(ctx context.Context, stmt string, values ...any) error {
	return s.session.Query(stmt, values...).WithContext(ctx).Exec()
}

func (s sessionStore) scan(ctx context.Context, stmt string, values []any, dest ...any) error {
	return s.session.Query(stmt, values...).WithContext(ctx).Scan(dest...)
}

func (s sessionStore) iter(ctx context.Context, stmt string, values ...any) cqlIterator {
	return s.session.Query(stmt, values...).WithContext(ctx).Iter()
}

type appServicesRepository struct {
	store   cqlStore
	now     func() time.Time
	newUUID func() (gocql.UUID, error)
}

func newWithStore(store cqlStore) *appServicesRepository {
	return &appServicesRepository{
		store: store,
		now: func() time.Time {
			return time.Now().UTC()
		},
		newUUID: gocql.RandomUUID,
	}
}

func (r *appServicesRepository) Create(ctx context.Context, params CreateAppServiceParams) (AppService, error) {
	normalized, err := normalizeCreateParams(params)
	if err != nil {
		return AppService{}, err
	}
	id, err := r.resolveCreateID(normalized.ID)
	if err != nil {
		return AppService{}, err
	}
	tid, err := parseRequiredUUID("tenantID", normalized.TenantID)
	if err != nil {
		return AppService{}, err
	}
	nid, err := parseRequiredUUID("networkID", normalized.NetworkID)
	if err != nil {
		return AppService{}, err
	}
	sid, err := parseRequiredUUID("subnetID", normalized.SubnetID)
	if err != nil {
		return AppService{}, err
	}

	now := r.now().UTC()
	row, err := rowFromCreateParams(id, tid, nid, sid, normalized, now)
	if err != nil {
		return AppService{}, err
	}

	if row.exposure != nil {
		// Reserve public host before creating primary state. This avoids leaving a new app_services row
		// behind when the only invalid part of a create request is a duplicate public hostname.
		applied, err := r.store.mapScanCAS(ctx, cqlInsertExposureByHost, row.exposureInsertValues()...)
		if err != nil {
			return AppService{}, mapInternalErr(err)
		}
		if !applied {
			return AppService{}, ErrAppServiceExists
		}
	}

	// Primary row is the source of truth and uses IF NOT EXISTS so callers can safely retry a create
	// with the same explicit id without accidentally overwriting workload intent.
	applied, err := r.store.mapScanCAS(ctx, cqlInsertAppService, row.insertValues()...)
	if err != nil {
		return AppService{}, mapInternalErr(err)
	}
	if !applied {
		if row.exposure != nil {
			_ = r.store.exec(ctx, cqlDeleteExposureByHost, row.exposure.Host)
		}
		return AppService{}, ErrAppServiceExists
	}

	// Denormalized network row keeps list reads partition-local. This is intentionally a separate
	// write: Scylla does not provide multi-table transactions, so later repair/reconciliation should
	// treat the primary app_services table as authoritative if a partial write ever happens.
	if err := r.store.exec(ctx, cqlInsertAppServiceByNetwork, row.networkInsertValues()...); err != nil {
		return AppService{}, mapInternalErr(err)
	}

	return row.toModel()
}

func (r *appServicesRepository) Get(ctx context.Context, appServiceID string) (AppService, error) {
	id, err := parseRequiredUUID("appServiceID", appServiceID)
	if err != nil {
		return AppService{}, err
	}
	row, err := r.getRow(ctx, id)
	if err != nil {
		return AppService{}, err
	}
	return row.toModel()
}

func (r *appServicesRepository) ListByNetwork(ctx context.Context, networkID string) ([]AppService, error) {
	nid, err := parseRequiredUUID("networkID", networkID)
	if err != nil {
		return nil, err
	}
	iter := r.store.iter(ctx, cqlSelectAppServicesByNetwork, nid)
	var ids []gocql.UUID
	var id gocql.UUID
	for iter.Scan(&id) {
		ids = append(ids, id)
	}
	if err := iter.Close(); err != nil {
		return nil, mapInternalErr(err)
	}

	out := make([]AppService, 0, len(ids))
	for _, appID := range ids {
		row, err := r.getRow(ctx, appID)
		if err != nil {
			if errors.Is(err, ErrAppServiceNotFound) {
				// Listing rows are denormalized. If a primary row was removed first, skip the stale row;
				// a future repair pass can delete it without breaking list callers.
				continue
			}
			return nil, err
		}
		if row.networkID != nid {
			continue
		}
		model, err := row.toModel()
		if err != nil {
			return nil, err
		}
		out = append(out, model)
	}
	return out, nil
}

func (r *appServicesRepository) UpdateStatus(ctx context.Context, params UpdateStatusParams) (AppService, error) {
	if strings.TrimSpace(string(params.Status)) == "" {
		return AppService{}, invalid("status is required")
	}
	id, err := parseRequiredUUID("appServiceID", params.AppServiceID)
	if err != nil {
		return AppService{}, err
	}
	row, err := r.getRow(ctx, id)
	if err != nil {
		return AppService{}, err
	}
	now := r.now().UTC()
	status := string(params.Status)
	if err := r.store.exec(ctx, cqlUpdateAppServiceStatus, status, now, id); err != nil {
		return AppService{}, mapInternalErr(err)
	}
	// Keep the network list row in step with the primary lifecycle status where possible. If this
	// write fails, callers get an error because the list view would otherwise show stale state.
	if err := r.store.exec(ctx, cqlUpdateAppServiceByNetworkStatus, status, now, row.networkID, row.createdAt, id); err != nil {
		return AppService{}, mapInternalErr(err)
	}
	row.status = status
	row.updatedAt = now
	return row.toModel()
}

func (r *appServicesRepository) UpdateExposure(ctx context.Context, params UpdateExposureParams) (AppService, error) {
	if params.Exposure != nil && strings.TrimSpace(string(params.Status)) == "" {
		return AppService{}, invalid("exposure status is required")
	}
	id, err := parseRequiredUUID("appServiceID", params.AppServiceID)
	if err != nil {
		return AppService{}, err
	}
	row, err := r.getRow(ctx, id)
	if err != nil {
		return AppService{}, err
	}
	now := r.now().UTC()
	oldHost := exposureHost(row.exposure)
	updated, err := row.withExposure(params.Exposure, params.Status, now)
	if err != nil {
		return AppService{}, err
	}

	if oldHost != "" && oldHost != exposureHost(updated.exposure) {
		if err := r.store.exec(ctx, cqlDeleteExposureByHost, oldHost); err != nil {
			return AppService{}, mapInternalErr(err)
		}
	}
	if updated.exposure != nil && oldHost == exposureHost(updated.exposure) && oldHost != "" {
		if err := r.store.exec(ctx, cqlUpdateExposureByHost, updated.exposureUpdateValues()...); err != nil {
			return AppService{}, mapInternalErr(err)
		}
	} else if updated.exposure != nil {
		applied, err := r.store.mapScanCAS(ctx, cqlInsertExposureByHost, updated.exposureInsertValues()...)
		if err != nil {
			return AppService{}, mapInternalErr(err)
		}
		if !applied {
			return AppService{}, ErrAppServiceExists
		}
	}

	if err := r.store.exec(ctx, cqlUpdateAppServiceExposure, updated.exposureType, updated.exposureStatus, updated.exposureJSON, updated.swaggerJSON, now, id); err != nil {
		return AppService{}, mapInternalErr(err)
	}
	// The list row carries only exposure summary fields and public host. Full route and Swagger
	// metadata stays on the primary row and host lookup table.
	if err := r.store.exec(ctx, cqlUpdateAppServiceByNetworkExposure, updated.exposureType, updated.exposureStatus, exposureHost(updated.exposure), now, row.networkID, row.createdAt, id); err != nil {
		return AppService{}, mapInternalErr(err)
	}
	return updated.toModel()
}

func (r *appServicesRepository) MarkDeleted(ctx context.Context, appServiceID string) (AppService, error) {
	return r.UpdateStatus(ctx, UpdateStatusParams{
		AppServiceID: appServiceID,
		Status:       AppServiceStatusDeleted,
	})
}

func (r *appServicesRepository) Delete(ctx context.Context, appServiceID string) error {
	id, err := parseRequiredUUID("appServiceID", appServiceID)
	if err != nil {
		return err
	}
	row, err := r.getRow(ctx, id)
	if err != nil {
		return err
	}
	if host := exposureHost(row.exposure); host != "" {
		if err := r.store.exec(ctx, cqlDeleteExposureByHost, host); err != nil {
			return mapInternalErr(err)
		}
	}
	// Delete the network listing row before the primary row so a racing list does not discover an id
	// that is already gone. ListByNetwork still tolerates stale rows for crash recovery.
	if err := r.store.exec(ctx, cqlDeleteAppServiceByNetwork, row.networkID, row.createdAt, id); err != nil {
		return mapInternalErr(err)
	}
	if err := r.store.exec(ctx, cqlDeleteAppService, id); err != nil {
		return mapInternalErr(err)
	}
	return nil
}

func (r *appServicesRepository) resolveCreateID(raw string) (gocql.UUID, error) {
	if strings.TrimSpace(raw) != "" {
		return parseRequiredUUID("id", raw)
	}
	id, err := r.newUUID()
	if err != nil {
		return gocql.UUID{}, mapInternalErr(err)
	}
	return id, nil
}

func (r *appServicesRepository) getRow(ctx context.Context, id gocql.UUID) (appServiceRow, error) {
	var row appServiceRow
	if err := r.store.scan(ctx, cqlSelectAppServiceByID, []any{id},
		&row.id,
		&row.tenantID,
		&row.networkID,
		&row.subnetID,
		&row.name,
		&row.status,
		&row.serviceType,
		&row.image,
		&row.buildContext,
		&row.dockerfile,
		&row.commandJSON,
		&row.argsJSON,
		&row.cpu,
		&row.memory,
		&row.replicas,
		&row.envJSON,
		&row.portsJSON,
		&row.exposureType,
		&row.exposureStatus,
		&row.exposureJSON,
		&row.swaggerJSON,
		&row.createdAt,
		&row.updatedAt,
	); err != nil {
		return appServiceRow{}, mapScanErr(err, ErrAppServiceNotFound)
	}
	if err := row.decodeJSON(); err != nil {
		return appServiceRow{}, err
	}
	return row, nil
}

type appServiceRow struct {
	id             gocql.UUID
	tenantID       gocql.UUID
	networkID      gocql.UUID
	subnetID       gocql.UUID
	name           string
	status         string
	serviceType    string
	image          string
	buildContext   string
	dockerfile     string
	commandJSON    string
	argsJSON       string
	cpu            string
	memory         string
	replicas       int
	envJSON        string
	portsJSON      string
	exposureType   string
	exposureStatus string
	exposureJSON   string
	swaggerJSON    string
	createdAt      time.Time
	updatedAt      time.Time

	command  []string
	args     []string
	env      []AppServiceEnvVar
	ports    []AppServicePort
	exposure *AppServiceExposure
}

func rowFromCreateParams(id, tid, nid, sid gocql.UUID, params CreateAppServiceParams, now time.Time) (appServiceRow, error) {
	replicas := params.Runtime.Replicas
	if replicas == 0 {
		replicas = 1
	}
	status := params.Status
	if status == "" {
		status = AppServiceStatusCreating
	}
	row := appServiceRow{
		id:          id,
		tenantID:    tid,
		networkID:   nid,
		subnetID:    sid,
		name:        params.Name,
		status:      string(status),
		serviceType: params.Runtime.ServiceType,
		image:       params.Runtime.Image,
		cpu:         params.Runtime.Resources.CPU,
		memory:      params.Runtime.Resources.Memory,
		replicas:    replicas,
		command:     append([]string(nil), params.Runtime.Command...),
		args:        append([]string(nil), params.Runtime.Args...),
		env:         append([]AppServiceEnvVar(nil), params.Runtime.Env...),
		ports:       append([]AppServicePort(nil), params.Runtime.Ports...),
		exposure:    cloneExposure(params.Exposure),
		createdAt:   now,
		updatedAt:   now,
	}
	if params.Runtime.Build != nil {
		row.buildContext = strings.TrimSpace(params.Runtime.Build.Context)
		row.dockerfile = strings.TrimSpace(params.Runtime.Build.Dockerfile)
	}
	if row.exposure != nil {
		row.exposureType = string(row.exposure.Type)
		row.exposureStatus = string(AppServiceExposureStatusPending)
	}
	if err := row.encodeJSON(); err != nil {
		return appServiceRow{}, err
	}
	return row, nil
}

func (r appServiceRow) insertValues() []any {
	return []any{
		r.id, r.tenantID, r.networkID, r.subnetID, r.name, r.status, r.serviceType, r.image,
		r.buildContext, r.dockerfile, r.commandJSON, r.argsJSON, r.cpu, r.memory, r.replicas,
		r.envJSON, r.portsJSON, r.exposureType, r.exposureStatus, r.exposureJSON, r.swaggerJSON,
		r.createdAt, r.updatedAt,
	}
}

func (r appServiceRow) networkInsertValues() []any {
	return []any{
		r.networkID, r.createdAt, r.id, r.tenantID, r.name, r.subnetID, r.status, r.serviceType,
		r.exposureType, r.exposureStatus, exposureHost(r.exposure), r.updatedAt,
	}
}

func (r appServiceRow) exposureInsertValues() []any {
	if r.exposure == nil {
		return nil
	}
	return []any{
		r.exposure.Host, r.id, r.tenantID, r.networkID, r.exposure.PortRef, r.exposure.TLSEnabled,
		r.exposureStatus, r.swaggerJSON, r.updatedAt,
	}
}

func (r appServiceRow) exposureUpdateValues() []any {
	if r.exposure == nil {
		return nil
	}
	return []any{
		r.id, r.tenantID, r.networkID, r.exposure.PortRef, r.exposure.TLSEnabled,
		r.exposureStatus, r.swaggerJSON, r.updatedAt, r.exposure.Host,
	}
}

func (r appServiceRow) withExposure(exposure *AppServiceExposure, status AppServiceExposureStatus, updatedAt time.Time) (appServiceRow, error) {
	out := r
	out.exposure = cloneExposure(exposure)
	out.updatedAt = updatedAt
	if exposure == nil {
		out.exposureType = ""
		out.exposureStatus = string(AppServiceExposureStatusRemoved)
	} else {
		out.exposureType = string(exposure.Type)
		out.exposureStatus = string(status)
	}
	if err := out.encodeJSON(); err != nil {
		return appServiceRow{}, err
	}
	return out, nil
}

func (r *appServiceRow) encodeJSON() error {
	var err error
	if r.commandJSON, err = marshalJSON(r.command); err != nil {
		return err
	}
	if r.argsJSON, err = marshalJSON(r.args); err != nil {
		return err
	}
	if r.envJSON, err = marshalJSON(r.env); err != nil {
		return err
	}
	if r.portsJSON, err = marshalJSON(r.ports); err != nil {
		return err
	}
	if r.exposure != nil {
		if r.exposureJSON, err = marshalJSON(r.exposure); err != nil {
			return err
		}
		if r.swaggerJSON, err = marshalJSON(r.exposure.Swagger); err != nil {
			return err
		}
	} else {
		r.exposureJSON = ""
		r.swaggerJSON = ""
	}
	return nil
}

func (r *appServiceRow) decodeJSON() error {
	if err := unmarshalJSON(r.commandJSON, &r.command); err != nil {
		return err
	}
	if err := unmarshalJSON(r.argsJSON, &r.args); err != nil {
		return err
	}
	if err := unmarshalJSON(r.envJSON, &r.env); err != nil {
		return err
	}
	if err := unmarshalJSON(r.portsJSON, &r.ports); err != nil {
		return err
	}
	if strings.TrimSpace(r.exposureJSON) != "" {
		var exposure AppServiceExposure
		if err := unmarshalJSON(r.exposureJSON, &exposure); err != nil {
			return err
		}
		r.exposure = &exposure
	}
	return nil
}

func (r appServiceRow) toModel() (AppService, error) {
	build := (*AppServiceBuild)(nil)
	if r.buildContext != "" || r.dockerfile != "" {
		build = &AppServiceBuild{Context: r.buildContext, Dockerfile: r.dockerfile}
	}
	return AppService{
		ID:        r.id.String(),
		TenantID:  r.tenantID.String(),
		NetworkID: r.networkID.String(),
		SubnetID:  r.subnetID.String(),
		Name:      r.name,
		Status:    AppServiceStatus(r.status),
		Runtime: AppServiceRuntime{
			ServiceType: r.serviceType,
			Image:       r.image,
			Build:       build,
			Command:     append([]string(nil), r.command...),
			Args:        append([]string(nil), r.args...),
			Resources: AppServiceResources{
				CPU:    r.cpu,
				Memory: r.memory,
			},
			Env:      append([]AppServiceEnvVar(nil), r.env...),
			Ports:    append([]AppServicePort(nil), r.ports...),
			Replicas: r.replicas,
		},
		Exposure:       cloneExposure(r.exposure),
		ExposureStatus: AppServiceExposureStatus(r.exposureStatus),
		CreatedAt:      r.createdAt,
		UpdatedAt:      r.updatedAt,
	}, nil
}

func normalizeCreateParams(params CreateAppServiceParams) (CreateAppServiceParams, error) {
	params.TenantID = strings.TrimSpace(params.TenantID)
	params.NetworkID = strings.TrimSpace(params.NetworkID)
	params.SubnetID = strings.TrimSpace(params.SubnetID)
	params.Name = strings.TrimSpace(params.Name)
	params.Runtime.ServiceType = strings.TrimSpace(params.Runtime.ServiceType)
	params.Runtime.Image = strings.TrimSpace(params.Runtime.Image)
	params.Runtime.Resources.CPU = strings.TrimSpace(params.Runtime.Resources.CPU)
	params.Runtime.Resources.Memory = strings.TrimSpace(params.Runtime.Resources.Memory)
	if params.Runtime.Build != nil {
		params.Runtime.Build.Context = strings.TrimSpace(params.Runtime.Build.Context)
		params.Runtime.Build.Dockerfile = strings.TrimSpace(params.Runtime.Build.Dockerfile)
	}
	if params.TenantID == "" {
		return CreateAppServiceParams{}, invalid("tenantID is required")
	}
	if params.NetworkID == "" {
		return CreateAppServiceParams{}, invalid("networkID is required")
	}
	if params.SubnetID == "" {
		return CreateAppServiceParams{}, invalid("subnetID is required")
	}
	if params.Name == "" {
		return CreateAppServiceParams{}, invalid("name is required")
	}
	if params.Runtime.ServiceType == "" {
		return CreateAppServiceParams{}, invalid("runtime.serviceType is required")
	}
	if params.Runtime.Image == "" && params.Runtime.Build == nil {
		return CreateAppServiceParams{}, invalid("runtime.image or runtime.build is required")
	}
	if params.Runtime.Image != "" && params.Runtime.Build != nil {
		return CreateAppServiceParams{}, invalid("runtime.image and runtime.build are mutually exclusive")
	}
	if params.Runtime.Build != nil && (params.Runtime.Build.Context == "" || params.Runtime.Build.Dockerfile == "") {
		return CreateAppServiceParams{}, invalid("runtime.build.context and runtime.build.dockerfile are required")
	}
	if params.Runtime.Resources.CPU == "" {
		return CreateAppServiceParams{}, invalid("runtime.resources.cpu is required")
	}
	if params.Runtime.Resources.Memory == "" {
		return CreateAppServiceParams{}, invalid("runtime.resources.memory is required")
	}
	if params.Runtime.Replicas < 0 {
		return CreateAppServiceParams{}, invalid("runtime.replicas must be non-negative")
	}
	if params.Exposure != nil {
		if err := validateExposure(params.Exposure); err != nil {
			return CreateAppServiceParams{}, err
		}
	}
	return params, nil
}

func validateExposure(exposure *AppServiceExposure) error {
	if strings.TrimSpace(string(exposure.Type)) == "" {
		return invalid("exposure.type is required")
	}
	exposure.Host = strings.TrimSpace(exposure.Host)
	exposure.PortRef = strings.TrimSpace(exposure.PortRef)
	exposure.Swagger.PublicPath = strings.TrimSpace(exposure.Swagger.PublicPath)
	exposure.Swagger.OpenAPIPath = strings.TrimSpace(exposure.Swagger.OpenAPIPath)
	exposure.Swagger.SpecURL = strings.TrimSpace(exposure.Swagger.SpecURL)
	if exposure.Host == "" {
		return invalid("exposure.host is required")
	}
	if exposure.PortRef == "" {
		return invalid("exposure.portRef is required")
	}
	if exposure.Swagger.PublicPath == "" || exposure.Swagger.OpenAPIPath == "" {
		return invalid("exposure.swagger publicPath and openapiPath are required")
	}
	if exposure.Swagger.SpecURL == "" && len(exposure.Swagger.InlineSpec) == 0 {
		return invalid("exposure.swagger specUrl or inlineSpec is required")
	}
	return nil
}

func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", cferrors.Wrap(cferrors.CodeInvalidInput, "invalid app service JSON field", err)
	}
	return string(b), nil
}

func unmarshalJSON(raw string, dest any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return cferrors.Wrap(cferrors.CodeInternal, "stored app service JSON is invalid", err)
	}
	return nil
}

func parseRequiredUUID(field, id string) (gocql.UUID, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return gocql.UUID{}, invalid(field + " is required")
	}
	u, err := gocql.ParseUUID(id)
	if err != nil {
		return gocql.UUID{}, invalid("invalid " + field)
	}
	return u, nil
}

func cloneExposure(in *AppServiceExposure) *AppServiceExposure {
	if in == nil {
		return nil
	}
	out := *in
	if in.Swagger.InlineSpec != nil {
		out.Swagger.InlineSpec = make(map[string]any, len(in.Swagger.InlineSpec))
		for k, v := range in.Swagger.InlineSpec {
			out.Swagger.InlineSpec[k] = v
		}
	}
	return &out
}

func exposureHost(exposure *AppServiceExposure) string {
	if exposure == nil {
		return ""
	}
	return strings.TrimSpace(exposure.Host)
}

func invalid(message string) error {
	return cferrors.Wrap(ErrInvalidAppService.Code(), message, ErrInvalidAppService)
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
