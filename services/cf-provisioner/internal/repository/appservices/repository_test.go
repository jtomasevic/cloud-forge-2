package appservices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gocql/gocql"
)

const (
	testTenantID     = "10000000-0000-0000-0000-000000000001"
	testNetworkID    = "20000000-0000-0000-0000-000000000001"
	testSubnetID     = "30000000-0000-0000-0000-000000000001"
	testAppServiceID = "40000000-0000-0000-0000-000000000001"
	testSecondAppID  = "40000000-0000-0000-0000-000000000002"
)

func TestRepositoryCreateGetListUpdateDeletePaths(t *testing.T) {
	repo, store := newTestRepository(t, testAppServiceID)
	ctx := context.Background()

	created, err := repo.Create(ctx, validCreateParams(""))
	if err != nil {
		t.Fatalf("create app service: %v", err)
	}
	if created.ID != testAppServiceID {
		t.Fatalf("id: got %q want %q", created.ID, testAppServiceID)
	}
	if created.Status != AppServiceStatusCreating {
		t.Fatalf("status: got %q", created.Status)
	}

	got, err := repo.Get(ctx, testAppServiceID)
	if err != nil {
		t.Fatalf("get app service: %v", err)
	}
	if got.Name != "orders-api" || got.Runtime.ServiceType != "rest" {
		t.Fatalf("unexpected app service: %+v", got)
	}

	listed, err := repo.ListByNetwork(ctx, testNetworkID)
	if err != nil {
		t.Fatalf("list by network: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != testAppServiceID {
		t.Fatalf("listed: %+v", listed)
	}

	running, err := repo.UpdateStatus(ctx, UpdateStatusParams{
		AppServiceID: testAppServiceID,
		Status:       AppServiceStatusRunning,
	})
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	if running.Status != AppServiceStatusRunning {
		t.Fatalf("running status: got %q", running.Status)
	}
	if store.network[testNetworkID][0].status != string(AppServiceStatusRunning) {
		t.Fatalf("network status not updated: %+v", store.network[testNetworkID][0])
	}

	exposed, err := repo.UpdateExposure(ctx, UpdateExposureParams{
		AppServiceID: testAppServiceID,
		Exposure:     sampleExposure("orders.local.cloudforge.dev"),
		Status:       AppServiceExposureStatusActive,
	})
	if err != nil {
		t.Fatalf("update exposure: %v", err)
	}
	if exposed.Exposure == nil || exposed.Exposure.Host != "orders.local.cloudforge.dev" {
		t.Fatalf("exposure not returned: %+v", exposed.Exposure)
	}
	if exposed.ExposureStatus != AppServiceExposureStatusActive {
		t.Fatalf("exposure status: got %q", exposed.ExposureStatus)
	}
	if _, ok := store.exposures["orders.local.cloudforge.dev"]; !ok {
		t.Fatal("public host lookup was not written")
	}

	if _, err := repo.UpdateExposure(ctx, UpdateExposureParams{
		AppServiceID: testAppServiceID,
		Exposure:     sampleExposure("orders.local.cloudforge.dev"),
		Status:       AppServiceExposureStatusActive,
	}); err != nil {
		t.Fatalf("same-host exposure update should refresh host row, got: %v", err)
	}

	removedExposure, err := repo.UpdateExposure(ctx, UpdateExposureParams{
		AppServiceID: testAppServiceID,
		Exposure:     nil,
	})
	if err != nil {
		t.Fatalf("remove exposure: %v", err)
	}
	if removedExposure.Exposure != nil || removedExposure.ExposureStatus != AppServiceExposureStatusRemoved {
		t.Fatalf("exposure was not removed: %+v", removedExposure)
	}
	if _, ok := store.exposures["orders.local.cloudforge.dev"]; ok {
		t.Fatal("public host lookup still exists after exposure removal")
	}

	marked, err := repo.MarkDeleted(ctx, testAppServiceID)
	if err != nil {
		t.Fatalf("mark deleted: %v", err)
	}
	if marked.Status != AppServiceStatusDeleted {
		t.Fatalf("mark deleted status: got %q", marked.Status)
	}

	if err := repo.Delete(ctx, testAppServiceID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := store.rows[testAppServiceID]; ok {
		t.Fatal("primary row still exists after delete")
	}
	if len(store.network[testNetworkID]) != 0 {
		t.Fatalf("network index row still exists: %+v", store.network[testNetworkID])
	}
	if _, ok := store.exposures["orders.local.cloudforge.dev"]; ok {
		t.Fatal("exposure host row still exists after delete")
	}
	if _, err := repo.Get(ctx, testAppServiceID); !errors.Is(err, ErrAppServiceNotFound) {
		t.Fatalf("get deleted app service: got %v want ErrAppServiceNotFound", err)
	}
}

func TestRepositoryCreateDuplicateIDReturnsExists(t *testing.T) {
	repo, _ := newTestRepository(t, testAppServiceID)
	ctx := context.Background()

	params := validCreateParams(testAppServiceID)
	if _, err := repo.Create(ctx, params); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := repo.Create(ctx, params); !errors.Is(err, ErrAppServiceExists) {
		t.Fatalf("duplicate create: got %v want ErrAppServiceExists", err)
	}
}

func TestRepositoryDuplicateExposureHostDoesNotWritePrimary(t *testing.T) {
	repo, store := newTestRepository(t, testAppServiceID, testSecondAppID)
	ctx := context.Background()

	first := validCreateParams("")
	first.Exposure = sampleExposure("shared.local.cloudforge.dev")
	if _, err := repo.Create(ctx, first); err != nil {
		t.Fatalf("first create: %v", err)
	}
	second := validCreateParams("")
	second.Name = "billing-api"
	second.Exposure = sampleExposure("shared.local.cloudforge.dev")
	if _, err := repo.Create(ctx, second); !errors.Is(err, ErrAppServiceExists) {
		t.Fatalf("duplicate host create: got %v want ErrAppServiceExists", err)
	}
	if _, ok := store.rows[testSecondAppID]; ok {
		t.Fatal("duplicate host create wrote primary app_services row")
	}
}

func TestRepositorySerializesRuntimeExposureAndSwaggerJSON(t *testing.T) {
	repo, store := newTestRepository(t, testAppServiceID)
	ctx := context.Background()

	params := validCreateParams("")
	params.Exposure = sampleExposure("orders.local.cloudforge.dev")
	if _, err := repo.Create(ctx, params); err != nil {
		t.Fatalf("create: %v", err)
	}
	row := store.rows[testAppServiceID]

	var command []string
	if err := json.Unmarshal([]byte(row.commandJSON), &command); err != nil {
		t.Fatalf("command json: %v", err)
	}
	if !reflect.DeepEqual(command, []string{"/app/orders"}) {
		t.Fatalf("command json round trip: %+v", command)
	}
	var env []AppServiceEnvVar
	if err := json.Unmarshal([]byte(row.envJSON), &env); err != nil {
		t.Fatalf("env json: %v", err)
	}
	if len(env) != 2 || env[0].Name != "LOG_LEVEL" || env[1].SecretRef != "secret/db-url" {
		t.Fatalf("env json round trip: %+v", env)
	}
	var ports []AppServicePort
	if err := json.Unmarshal([]byte(row.portsJSON), &ports); err != nil {
		t.Fatalf("ports json: %v", err)
	}
	if len(ports) != 1 || ports[0].Name != "http" || ports[0].ContainerPort != 8080 {
		t.Fatalf("ports json round trip: %+v", ports)
	}
	var exposure AppServiceExposure
	if err := json.Unmarshal([]byte(row.exposureJSON), &exposure); err != nil {
		t.Fatalf("exposure json: %v", err)
	}
	if exposure.Host != "orders.local.cloudforge.dev" || exposure.Swagger.OpenAPIPath != "/openapi.json" {
		t.Fatalf("exposure json round trip: %+v", exposure)
	}
	var swagger AppServiceSwagger
	if err := json.Unmarshal([]byte(row.swaggerJSON), &swagger); err != nil {
		t.Fatalf("swagger json: %v", err)
	}
	if swagger.PublicPath != "/swagger/" || swagger.InlineSpec["openapi"] != "3.0.3" {
		t.Fatalf("swagger json round trip: %+v", swagger)
	}

	got, err := repo.Get(ctx, testAppServiceID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Exposure == nil || got.Exposure.Swagger.InlineSpec["info"].(map[string]any)["title"] != "Orders API" {
		t.Fatalf("stored swagger did not reconstruct: %+v", got.Exposure)
	}
}

func TestRepositoryValidationAndNotFound(t *testing.T) {
	repo, _ := newTestRepository(t, testAppServiceID)
	ctx := context.Background()

	missingTenant := validCreateParams("")
	missingTenant.TenantID = " "
	if _, err := repo.Create(ctx, missingTenant); !errors.Is(err, ErrInvalidAppService) {
		t.Fatalf("missing tenant: got %v want ErrInvalidAppService", err)
	}
	invalidID := validCreateParams("not-a-uuid")
	if _, err := repo.Create(ctx, invalidID); !errors.Is(err, ErrInvalidAppService) {
		t.Fatalf("invalid id: got %v want ErrInvalidAppService", err)
	}
	if _, err := repo.Get(ctx, testSecondAppID); !errors.Is(err, ErrAppServiceNotFound) {
		t.Fatalf("not found: got %v want ErrAppServiceNotFound", err)
	}
}

func TestCQLConstantsUseExpectedDenormalizedTables(t *testing.T) {
	for name, stmt := range map[string]string{
		"insert-list":      cqlInsertAppServiceByNetwork,
		"update-list":      cqlUpdateAppServiceByNetworkStatus,
		"exposure-host":    cqlInsertExposureByHost,
		"delete-list":      cqlDeleteAppServiceByNetwork,
		"same-host-update": cqlUpdateExposureByHost,
	} {
		if !strings.Contains(stmt, "app_services_by_network") && !strings.Contains(stmt, "app_service_exposures_by_host") {
			t.Fatalf("%s does not target an app-service denormalized table: %s", name, stmt)
		}
	}
	if !strings.Contains(cqlInsertAppService, "IF NOT EXISTS") {
		t.Fatalf("primary create must protect duplicate app service ids: %s", cqlInsertAppService)
	}
	if !strings.Contains(cqlInsertExposureByHost, "IF NOT EXISTS") {
		t.Fatalf("host create must protect duplicate public hostnames: %s", cqlInsertExposureByHost)
	}
}

func newTestRepository(t *testing.T, ids ...string) (*appServicesRepository, *fakeStore) {
	t.Helper()
	store := &fakeStore{
		rows:      map[string]appServiceRow{},
		network:   map[string][]fakeNetworkRow{},
		exposures: map[string]fakeExposureRow{},
	}
	repo := newWithStore(store)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	repo.now = func() time.Time {
		now = now.Add(time.Minute)
		return now
	}
	var parsedIDs []gocql.UUID
	for _, raw := range ids {
		parsedIDs = append(parsedIDs, mustUUID(t, raw))
	}
	repo.newUUID = func() (gocql.UUID, error) {
		if len(parsedIDs) == 0 {
			return gocql.UUID{}, fmt.Errorf("no test uuid left")
		}
		id := parsedIDs[0]
		parsedIDs = parsedIDs[1:]
		return id, nil
	}
	return repo, store
}

func validCreateParams(id string) CreateAppServiceParams {
	return CreateAppServiceParams{
		ID:        id,
		TenantID:  testTenantID,
		NetworkID: testNetworkID,
		SubnetID:  testSubnetID,
		Name:      "orders-api",
		Runtime: AppServiceRuntime{
			ServiceType: "rest",
			Image:       "registry.local/cloudforge/orders-api:dev",
			Command:     []string{"/app/orders"},
			Args:        []string{"serve"},
			Resources: AppServiceResources{
				CPU:    "250m",
				Memory: "256Mi",
			},
			Env: []AppServiceEnvVar{
				{Name: "LOG_LEVEL", Value: "info"},
				{Name: "DATABASE_URL", SecretRef: "secret/db-url"},
			},
			Ports: []AppServicePort{
				{Name: "http", ContainerPort: 8080, Protocol: "HTTP"},
			},
			Replicas: 1,
		},
	}
}

func sampleExposure(host string) *AppServiceExposure {
	return &AppServiceExposure{
		Type:       AppServiceExposureTypeInternetGateway,
		Host:       host,
		PortRef:    "http",
		TLSEnabled: true,
		Swagger: AppServiceSwagger{
			PublicPath:  "/swagger/",
			OpenAPIPath: "/openapi.json",
			InlineSpec: map[string]any{
				"openapi": "3.0.3",
				"info": map[string]any{
					"title": "Orders API",
				},
			},
		},
	}
}

func mustUUID(t *testing.T, raw string) gocql.UUID {
	t.Helper()
	id, err := gocql.ParseUUID(raw)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", raw, err)
	}
	return id
}

type fakeStore struct {
	rows      map[string]appServiceRow
	network   map[string][]fakeNetworkRow
	exposures map[string]fakeExposureRow
}

type fakeNetworkRow struct {
	networkID      gocql.UUID
	createdAt      time.Time
	appServiceID   gocql.UUID
	tenantID       gocql.UUID
	name           string
	subnetID       gocql.UUID
	status         string
	serviceType    string
	exposureType   string
	exposureStatus string
	publicHost     string
	updatedAt      time.Time
}

type fakeExposureRow struct {
	host           string
	appServiceID   gocql.UUID
	tenantID       gocql.UUID
	networkID      gocql.UUID
	portName       string
	tlsEnabled     bool
	exposureStatus string
	swaggerJSON    string
	updatedAt      time.Time
}

func (s *fakeStore) mapScanCAS(_ context.Context, stmt string, values ...any) (bool, error) {
	switch stmt {
	case cqlInsertAppService:
		row := appServiceRow{
			id:             values[0].(gocql.UUID),
			tenantID:       values[1].(gocql.UUID),
			networkID:      values[2].(gocql.UUID),
			subnetID:       values[3].(gocql.UUID),
			name:           values[4].(string),
			status:         values[5].(string),
			serviceType:    values[6].(string),
			image:          values[7].(string),
			buildContext:   values[8].(string),
			dockerfile:     values[9].(string),
			commandJSON:    values[10].(string),
			argsJSON:       values[11].(string),
			cpu:            values[12].(string),
			memory:         values[13].(string),
			replicas:       values[14].(int),
			envJSON:        values[15].(string),
			portsJSON:      values[16].(string),
			exposureType:   values[17].(string),
			exposureStatus: values[18].(string),
			exposureJSON:   values[19].(string),
			swaggerJSON:    values[20].(string),
			createdAt:      values[21].(time.Time),
			updatedAt:      values[22].(time.Time),
		}
		if err := row.decodeJSON(); err != nil {
			return false, err
		}
		key := row.id.String()
		if _, ok := s.rows[key]; ok {
			return false, nil
		}
		s.rows[key] = row
		return true, nil
	case cqlInsertExposureByHost:
		row := fakeExposureRow{
			host:           values[0].(string),
			appServiceID:   values[1].(gocql.UUID),
			tenantID:       values[2].(gocql.UUID),
			networkID:      values[3].(gocql.UUID),
			portName:       values[4].(string),
			tlsEnabled:     values[5].(bool),
			exposureStatus: values[6].(string),
			swaggerJSON:    values[7].(string),
			updatedAt:      values[8].(time.Time),
		}
		if _, ok := s.exposures[row.host]; ok {
			return false, nil
		}
		s.exposures[row.host] = row
		return true, nil
	default:
		return false, fmt.Errorf("unexpected CAS statement: %s", stmt)
	}
}

func (s *fakeStore) exec(_ context.Context, stmt string, values ...any) error {
	switch stmt {
	case cqlInsertAppServiceByNetwork:
		row := fakeNetworkRow{
			networkID:      values[0].(gocql.UUID),
			createdAt:      values[1].(time.Time),
			appServiceID:   values[2].(gocql.UUID),
			tenantID:       values[3].(gocql.UUID),
			name:           values[4].(string),
			subnetID:       values[5].(gocql.UUID),
			status:         values[6].(string),
			serviceType:    values[7].(string),
			exposureType:   values[8].(string),
			exposureStatus: values[9].(string),
			publicHost:     values[10].(string),
			updatedAt:      values[11].(time.Time),
		}
		s.network[row.networkID.String()] = append(s.network[row.networkID.String()], row)
	case cqlUpdateAppServiceStatus:
		status := values[0].(string)
		updatedAt := values[1].(time.Time)
		id := values[2].(gocql.UUID).String()
		row := s.rows[id]
		row.status = status
		row.updatedAt = updatedAt
		s.rows[id] = row
	case cqlUpdateAppServiceByNetworkStatus:
		status := values[0].(string)
		updatedAt := values[1].(time.Time)
		nid := values[2].(gocql.UUID).String()
		createdAt := values[3].(time.Time)
		id := values[4].(gocql.UUID)
		for i := range s.network[nid] {
			if s.network[nid][i].createdAt.Equal(createdAt) && s.network[nid][i].appServiceID == id {
				s.network[nid][i].status = status
				s.network[nid][i].updatedAt = updatedAt
			}
		}
	case cqlUpdateExposureByHost:
		host := values[8].(string)
		s.exposures[host] = fakeExposureRow{
			host:           host,
			appServiceID:   values[0].(gocql.UUID),
			tenantID:       values[1].(gocql.UUID),
			networkID:      values[2].(gocql.UUID),
			portName:       values[3].(string),
			tlsEnabled:     values[4].(bool),
			exposureStatus: values[5].(string),
			swaggerJSON:    values[6].(string),
			updatedAt:      values[7].(time.Time),
		}
	case cqlUpdateAppServiceExposure:
		id := values[5].(gocql.UUID).String()
		row := s.rows[id]
		row.exposureType = values[0].(string)
		row.exposureStatus = values[1].(string)
		row.exposureJSON = values[2].(string)
		row.swaggerJSON = values[3].(string)
		row.updatedAt = values[4].(time.Time)
		row.exposure = nil
		if err := row.decodeJSON(); err != nil {
			return err
		}
		s.rows[id] = row
	case cqlUpdateAppServiceByNetworkExposure:
		nid := values[4].(gocql.UUID).String()
		createdAt := values[5].(time.Time)
		id := values[6].(gocql.UUID)
		for i := range s.network[nid] {
			if s.network[nid][i].createdAt.Equal(createdAt) && s.network[nid][i].appServiceID == id {
				s.network[nid][i].exposureType = values[0].(string)
				s.network[nid][i].exposureStatus = values[1].(string)
				s.network[nid][i].publicHost = values[2].(string)
				s.network[nid][i].updatedAt = values[3].(time.Time)
			}
		}
	case cqlDeleteExposureByHost:
		delete(s.exposures, values[0].(string))
	case cqlDeleteAppServiceByNetwork:
		nid := values[0].(gocql.UUID).String()
		createdAt := values[1].(time.Time)
		id := values[2].(gocql.UUID)
		rows := s.network[nid]
		filtered := rows[:0]
		for _, row := range rows {
			if row.createdAt.Equal(createdAt) && row.appServiceID == id {
				continue
			}
			filtered = append(filtered, row)
		}
		s.network[nid] = filtered
	case cqlDeleteAppService:
		delete(s.rows, values[0].(gocql.UUID).String())
	default:
		return fmt.Errorf("unexpected exec statement: %s", stmt)
	}
	return nil
}

func (s *fakeStore) scan(_ context.Context, stmt string, values []any, dest ...any) error {
	if stmt != cqlSelectAppServiceByID {
		return fmt.Errorf("unexpected scan statement: %s", stmt)
	}
	row, ok := s.rows[values[0].(gocql.UUID).String()]
	if !ok {
		return gocql.ErrNotFound
	}
	assign(dest[0], row.id)
	assign(dest[1], row.tenantID)
	assign(dest[2], row.networkID)
	assign(dest[3], row.subnetID)
	assign(dest[4], row.name)
	assign(dest[5], row.status)
	assign(dest[6], row.serviceType)
	assign(dest[7], row.image)
	assign(dest[8], row.buildContext)
	assign(dest[9], row.dockerfile)
	assign(dest[10], row.commandJSON)
	assign(dest[11], row.argsJSON)
	assign(dest[12], row.cpu)
	assign(dest[13], row.memory)
	assign(dest[14], row.replicas)
	assign(dest[15], row.envJSON)
	assign(dest[16], row.portsJSON)
	assign(dest[17], row.exposureType)
	assign(dest[18], row.exposureStatus)
	assign(dest[19], row.exposureJSON)
	assign(dest[20], row.swaggerJSON)
	assign(dest[21], row.createdAt)
	assign(dest[22], row.updatedAt)
	return nil
}

func (s *fakeStore) iter(_ context.Context, stmt string, values ...any) cqlIterator {
	if stmt != cqlSelectAppServicesByNetwork {
		return &fakeIter{err: fmt.Errorf("unexpected iter statement: %s", stmt)}
	}
	rows := s.network[values[0].(gocql.UUID).String()]
	ids := make([]gocql.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.appServiceID)
	}
	return &fakeIter{ids: ids}
}

type fakeIter struct {
	ids []gocql.UUID
	pos int
	err error
}

func (i *fakeIter) Scan(dest ...any) bool {
	if i.pos >= len(i.ids) {
		return false
	}
	assign(dest[0], i.ids[i.pos])
	i.pos++
	return true
}

func (i *fakeIter) Close() error {
	return i.err
}

func assign(dest any, value any) {
	switch d := dest.(type) {
	case *gocql.UUID:
		*d = value.(gocql.UUID)
	case *string:
		*d = value.(string)
	case *int:
		*d = value.(int)
	case *time.Time:
		*d = value.(time.Time)
	default:
		panic(fmt.Sprintf("unsupported dest %T", dest))
	}
}
