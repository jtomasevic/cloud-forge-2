package appservices

// CQL for CF App Service durable state and denormalized lookup tables.
// Bind order matches each constant's comment.

const (
	// cqlInsertAppService inserts the primary durable workload row with LWT duplicate protection.
	// Parameters:
	// (1) id UUID, (2) tenant_id UUID, (3) network_id UUID, (4) subnet_id UUID, (5) name text,
	// (6) status text, (7) service_type text, (8) image text, (9) build_context text,
	// (10) dockerfile text, (11) command_json text, (12) args_json text, (13) cpu text,
	// (14) memory text, (15) replicas int, (16) env_json text, (17) ports_json text,
	// (18) exposure_type text, (19) exposure_status text, (20) exposure_json text,
	// (21) swagger_json text, (22) created_at timestamp, (23) updated_at timestamp.
	cqlInsertAppService = `INSERT INTO app_services (id, tenant_id, network_id, subnet_id, name, status, service_type, image, build_context, dockerfile, command_json, args_json, cpu, memory, replicas, env_json, ports_json, exposure_type, exposure_status, exposure_json, swagger_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) IF NOT EXISTS`

	// cqlInsertAppServiceByNetwork writes the denormalized network listing row.
	// Parameters:
	// (1) network_id UUID, (2) created_at timestamp, (3) app_service_id UUID, (4) tenant_id UUID,
	// (5) name text, (6) subnet_id UUID, (7) status text, (8) service_type text,
	// (9) exposure_type text, (10) exposure_status text, (11) public_host text,
	// (12) updated_at timestamp.
	cqlInsertAppServiceByNetwork = `INSERT INTO app_services_by_network (network_id, created_at, app_service_id, tenant_id, name, subnet_id, status, service_type, exposure_type, exposure_status, public_host, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// cqlInsertExposureByHost reserves a public host for one app service. The host is unique across
	// all networks because public internet gateway routing resolves by hostname first.
	// Parameters:
	// (1) host text, (2) app_service_id UUID, (3) tenant_id UUID, (4) network_id UUID,
	// (5) port_name text, (6) tls_enabled boolean, (7) exposure_status text,
	// (8) swagger_json text, (9) updated_at timestamp.
	cqlInsertExposureByHost = `INSERT INTO app_service_exposures_by_host (host, app_service_id, tenant_id, network_id, port_name, tls_enabled, exposure_status, swagger_json, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) IF NOT EXISTS`

	// cqlUpdateExposureByHost refreshes the owner/status metadata for an already-reserved host.
	// Parameters:
	// (1) app_service_id UUID, (2) tenant_id UUID, (3) network_id UUID, (4) port_name text,
	// (5) tls_enabled boolean, (6) exposure_status text, (7) swagger_json text,
	// (8) updated_at timestamp, (9) host text.
	cqlUpdateExposureByHost = `UPDATE app_service_exposures_by_host SET app_service_id = ?, tenant_id = ?, network_id = ?, port_name = ?, tls_enabled = ?, exposure_status = ?, swagger_json = ?, updated_at = ? WHERE host = ?`

	// cqlSelectAppServiceByID loads a primary app service row by id.
	// Parameters: (1) id UUID.
	cqlSelectAppServiceByID = `SELECT id, tenant_id, network_id, subnet_id, name, status, service_type, image, build_context, dockerfile, command_json, args_json, cpu, memory, replicas, env_json, ports_json, exposure_type, exposure_status, exposure_json, swagger_json, created_at, updated_at FROM app_services WHERE id = ?`

	// cqlSelectAppServicesByNetwork lists app service ids from the denormalized network partition.
	// Parameters: (1) network_id UUID.
	cqlSelectAppServicesByNetwork = `SELECT app_service_id FROM app_services_by_network WHERE network_id = ?`

	// cqlUpdateAppServiceStatus updates the primary lifecycle status.
	// Parameters: (1) status text, (2) updated_at timestamp, (3) id UUID.
	cqlUpdateAppServiceStatus = `UPDATE app_services SET status = ?, updated_at = ? WHERE id = ?`

	// cqlUpdateAppServiceByNetworkStatus updates the status summary in the network listing row.
	// Parameters: (1) status text, (2) updated_at timestamp, (3) network_id UUID,
	// (4) created_at timestamp, (5) app_service_id UUID.
	cqlUpdateAppServiceByNetworkStatus = `UPDATE app_services_by_network SET status = ?, updated_at = ? WHERE network_id = ? AND created_at = ? AND app_service_id = ?`

	// cqlUpdateAppServiceExposure updates exposure and Swagger metadata in the primary row.
	// Parameters:
	// (1) exposure_type text, (2) exposure_status text, (3) exposure_json text,
	// (4) swagger_json text, (5) updated_at timestamp, (6) id UUID.
	cqlUpdateAppServiceExposure = `UPDATE app_services SET exposure_type = ?, exposure_status = ?, exposure_json = ?, swagger_json = ?, updated_at = ? WHERE id = ?`

	// cqlUpdateAppServiceByNetworkExposure updates public exposure summary fields for list calls.
	// Parameters:
	// (1) exposure_type text, (2) exposure_status text, (3) public_host text,
	// (4) updated_at timestamp, (5) network_id UUID, (6) created_at timestamp,
	// (7) app_service_id UUID.
	cqlUpdateAppServiceByNetworkExposure = `UPDATE app_services_by_network SET exposure_type = ?, exposure_status = ?, public_host = ?, updated_at = ? WHERE network_id = ? AND created_at = ? AND app_service_id = ?`

	// cqlDeleteExposureByHost removes a public host lookup row when exposure is removed or deleted.
	// Parameters: (1) host text.
	cqlDeleteExposureByHost = `DELETE FROM app_service_exposures_by_host WHERE host = ?`

	// cqlDeleteAppService removes the primary row after higher layers have cleaned up runtime objects.
	// Parameters: (1) id UUID.
	cqlDeleteAppService = `DELETE FROM app_services WHERE id = ?`

	// cqlDeleteAppServiceByNetwork removes the denormalized list row.
	// Parameters: (1) network_id UUID, (2) created_at timestamp, (3) app_service_id UUID.
	cqlDeleteAppServiceByNetwork = `DELETE FROM app_services_by_network WHERE network_id = ? AND created_at = ? AND app_service_id = ?`
)
