package main

import (
	"os"
	"strings"
	"testing"
)

const appServiceMigrationPath = "scripts/20240101009_create_app_services.cql"

func TestAppServiceMigrationCreatesRequiredTables(t *testing.T) {
	cql := readAppServiceMigration(t)
	normalized := normalizeCQL(cql)

	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS app_services",
		"CREATE TABLE IF NOT EXISTS app_services_by_network",
		"CREATE TABLE IF NOT EXISTS app_service_exposures_by_host",
		"CREATE TABLE IF NOT EXISTS app_service_jobs_by_app_service",
	} {
		if !strings.Contains(normalized, normalizeCQL(want)) {
			t.Fatalf("migration missing %q:\n%s", want, cql)
		}
	}
}

func TestAppServiceMigrationStoresReconstructableRuntimeAndExposureState(t *testing.T) {
	normalized := normalizeCQL(readAppServiceMigration(t))

	// These columns are the MVP persistence contract for rebuilding the desired workload
	// and its public route after process restart. JSON columns are allowed here because
	// the OpenAPI and service layers validate the nested shapes before persistence.
	for _, want := range []string{
		"tenant_id UUID",
		"network_id UUID",
		"subnet_id UUID",
		"service_type TEXT",
		"image TEXT",
		"build_context TEXT",
		"dockerfile TEXT",
		"command_json TEXT",
		"args_json TEXT",
		"env_json TEXT",
		"ports_json TEXT",
		"exposure_type TEXT",
		"exposure_status TEXT",
		"exposure_json TEXT",
		"swagger_json TEXT",
	} {
		if !strings.Contains(normalized, normalizeCQL(want)) {
			t.Fatalf("migration missing column %q", want)
		}
	}
}

func TestAppServiceMigrationSupportsNetworkListingAndJobCorrelation(t *testing.T) {
	normalized := normalizeCQL(readAppServiceMigration(t))

	for _, want := range []string{
		"PRIMARY KEY (network_id, created_at, app_service_id)",
		"WITH CLUSTERING ORDER BY (created_at DESC)",
		"PRIMARY KEY ((network_id, app_service_id), created_at, job_id)",
	} {
		if !strings.Contains(normalized, normalizeCQL(want)) {
			t.Fatalf("migration missing network/app-service index shape %q", want)
		}
	}
}

func readAppServiceMigration(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(appServiceMigrationPath)
	if err != nil {
		t.Fatalf("read %s: %v", appServiceMigrationPath, err)
	}
	return string(b)
}

func normalizeCQL(s string) string {
	return strings.Join(strings.Fields(strings.ToUpper(s)), " ")
}
