// Package client provides a typed Go client for OpenBao (the open-source
// Vault fork). It wraps the official OpenBao SDK
// (github.com/openbao/openbao/api/v2) which is wire-compatible with the
// HashiCorp Vault API. All errors are mapped to CloudForge typed CFError values
// so that callers always work with a single, consistent error hierarchy.
package client

//go:generate mockgen -destination mock/mock_secrets_client.go -package mock . SecretsClient

import (
	"context"
	"errors"
	"net/http"

	api "github.com/openbao/openbao/api/v2"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

// Config holds the parameters needed to connect to an OpenBao server.
type Config struct {
	// Address is the base URL of the OpenBao server, e.g. "http://localhost:8200".
	// Must be non-empty.
	Address string
	// Token is the root or service token used to authenticate every request.
	Token string
}

// SecretsClient is the public interface for CloudForge secret-storage
// operations. All CF services that need to store or retrieve secrets depend on
// this interface, making the concrete implementation easy to replace in tests.
type SecretsClient interface {
	// Write stores data at the given path. data is a map of arbitrary
	// key-value pairs. Overwrites any existing secret at that path.
	Write(ctx context.Context, path string, data map[string]interface{}) error

	// Read retrieves data stored at the given path. Returns ErrSecretNotFound
	// if the path does not exist or has been deleted.
	Read(ctx context.Context, path string) (map[string]interface{}, error)

	// Delete permanently removes the secret at the given path.
	Delete(ctx context.Context, path string) error

	// List returns the keys available under the given path prefix.
	// Returns an empty slice (not an error) when no keys exist.
	List(ctx context.Context, pathPrefix string) ([]string, error)
}

// logicalAPI is the subset of *api.Logical used by CFSecretsClient.
// Keeping it as an interface lets tests inject a fake without a live server.
// The real *api.Logical satisfies this interface automatically.
type logicalAPI interface {
	WriteWithContext(ctx context.Context, path string, data map[string]interface{}) (*api.Secret, error)
	ReadWithContext(ctx context.Context, path string) (*api.Secret, error)
	DeleteWithContext(ctx context.Context, path string) (*api.Secret, error)
	ListWithContext(ctx context.Context, path string) (*api.Secret, error)
}

// CFSecretsClient is the production implementation of SecretsClient backed by
// the OpenBao / Vault API.
type CFSecretsClient struct {
	logical logicalAPI
}

// New creates a configured OpenBao client from cfg.
// Returns ErrClientInit if cfg.Address is empty or if the SDK rejects the
// configuration (e.g. malformed URL or TLS error).
func New(cfg Config) (SecretsClient, error) {
	if cfg.Address == "" {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "openbao client initialization failed: address is required", ErrClientInit)
	}

	vaultCfg := api.DefaultConfig()
	vaultCfg.Address = cfg.Address

	c, err := api.NewClient(vaultCfg)
	if err != nil {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "openbao client initialization failed", err)
	}

	if cfg.Token != "" {
		c.SetToken(cfg.Token)
	}

	return newWithLogical(c.Logical()), nil
}

// newWithLogical creates a CFSecretsClient with the provided logicalAPI backend.
// Intended for unit tests; production code uses New.
func newWithLogical(l logicalAPI) *CFSecretsClient {
	return &CFSecretsClient{logical: l}
}

// Write stores data at path in OpenBao.
func (c *CFSecretsClient) Write(ctx context.Context, path string, data map[string]interface{}) error {
	_, err := c.logical.WriteWithContext(ctx, path, data)
	return mapAPIError(err)
}

// Read retrieves data stored at path. Returns ErrSecretNotFound for a missing
// path (both 404 responses and nil-secret responses).
func (c *CFSecretsClient) Read(ctx context.Context, path string) (map[string]interface{}, error) {
	secret, err := c.logical.ReadWithContext(ctx, path)
	if err != nil {
		return nil, mapAPIError(err)
	}
	if secret == nil || secret.Data == nil {
		return nil, cferrors.Wrap(cferrors.CodeNotFound, "secret not found", ErrSecretNotFound)
	}
	return secret.Data, nil
}

// Delete removes the secret at path.
func (c *CFSecretsClient) Delete(ctx context.Context, path string) error {
	_, err := c.logical.DeleteWithContext(ctx, path)
	return mapAPIError(err)
}

// List returns the key names available at pathPrefix.
// An empty or nil response from OpenBao is treated as an empty (non-error)
// list rather than a not-found error.
func (c *CFSecretsClient) List(ctx context.Context, pathPrefix string) ([]string, error) {
	secret, err := c.logical.ListWithContext(ctx, pathPrefix)
	if err != nil {
		return nil, mapAPIError(err)
	}
	if secret == nil || secret.Data == nil {
		return []string{}, nil
	}

	keysRaw, ok := secret.Data["keys"]
	if !ok {
		return []string{}, nil
	}

	raw, ok := keysRaw.([]interface{})
	if !ok {
		return nil, cferrors.New(cferrors.CodeInternal, "unexpected keys format in vault list response")
	}

	keys := make([]string, 0, len(raw))
	for _, k := range raw {
		if s, ok := k.(string); ok {
			keys = append(keys, s)
		}
	}
	return keys, nil
}

// mapAPIError translates an OpenBao SDK error into the appropriate CFError.
// The mapping follows HTTP semantics:
//
//   - nil              → nil (no error)
//   - 404 response     → ErrSecretNotFound (CodeNotFound)
//   - 403 response     → cferrors.ErrForbidden (CodeForbidden)
//   - other 4xx/5xx    → cferrors.ErrInternal (CodeInternal)
//   - network/timeout  → cferrors.ErrUnavailable (CodeUnavailable)
func mapAPIError(err error) error {
	if err == nil {
		return nil
	}

	var re *api.ResponseError
	if errors.As(err, &re) {
		switch re.StatusCode {
		case http.StatusNotFound:
			return cferrors.Wrap(cferrors.CodeNotFound, "secret not found", ErrSecretNotFound)
		case http.StatusForbidden:
			return cferrors.Wrap(cferrors.CodeForbidden, "access denied to vault path", cferrors.ErrForbidden)
		default:
			return cferrors.Wrap(cferrors.CodeInternal, "vault API error", err)
		}
	}

	// Non-ResponseError: network failure, TLS error, or context cancellation.
	return cferrors.Wrap(cferrors.CodeUnavailable, "openbao unreachable", err)
}
