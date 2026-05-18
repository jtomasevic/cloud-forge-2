// Package apikeys implements CF-Router's read-only access to API key metadata in ScyllaDB.
//
// CF-Accounts owns credential writes; the router only needs the **hot lookup path** by hash
// (api_keys_by_hash) to map a presented API key to an account id before calling CF-Accounts internal
// tenant resolution.
package apikeys
