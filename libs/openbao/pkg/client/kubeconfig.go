package client

import (
	"context"
	"encoding/base64"
	"fmt"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

// kubeconfigPathPrefix is the KV v1 mount prefix used for all tenant
// kubeconfigs. This assumes an OpenBao/Vault KV v1 mount at "secret/".
const kubeconfigPathPrefix = "secret/tenants"

// kubeconfigPath returns the canonical storage path for a tenant's kubeconfig.
func kubeconfigPath(tenantID string) string {
	return fmt.Sprintf("%s/%s/kubeconfig", kubeconfigPathPrefix, tenantID)
}

// StoreKubeconfig stores a tenant kubeconfig at the canonical path
// "secret/tenants/{tenantID}/kubeconfig" using a KV v1 mount.
//
// The raw kubeconfig bytes are base64-encoded and stored under the "data" key
// so the value is safe for transport and inspection without binary concerns.
func StoreKubeconfig(ctx context.Context, c SecretsClient, tenantID string, kubeconfigBytes []byte) error {
	encoded := base64.StdEncoding.EncodeToString(kubeconfigBytes)
	if err := c.Write(ctx, kubeconfigPath(tenantID), map[string]interface{}{"data": encoded}); err != nil {
		return cferrors.Wrapf(ErrVaultWrite, "tenant %s kubeconfig: %v", tenantID, err)
	}
	return nil
}

// LoadKubeconfig retrieves a tenant kubeconfig from the canonical path
// "secret/tenants/{tenantID}/kubeconfig" using a KV v1 mount.
//
// Returns ErrSecretNotFound if the kubeconfig has been revoked (via
// RevokeKubeconfig) or was never stored, immediately preventing further
// control-plane access to that tenant environment.
func LoadKubeconfig(ctx context.Context, c SecretsClient, tenantID string) ([]byte, error) {
	data, err := c.Read(ctx, kubeconfigPath(tenantID))
	if err != nil {
		return nil, err
	}

	raw, ok := data["data"]
	if !ok {
		return nil, cferrors.Wrapf(ErrVaultCorrupt, "tenant %s kubeconfig: missing 'data' key", tenantID)
	}

	encoded, ok := raw.(string)
	if !ok {
		return nil, cferrors.Wrapf(ErrVaultCorrupt, "tenant %s kubeconfig: 'data' value is not a string", tenantID)
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, cferrors.Wrapf(ErrVaultCorrupt, "tenant %s kubeconfig: base64 decode failed: %v", tenantID, err)
	}

	return decoded, nil
}

// RevokeKubeconfig deletes the tenant kubeconfig from the canonical path
// "secret/tenants/{tenantID}/kubeconfig".
//
// Deletion is immediate and irreversible via the OpenBao KV v1 API. After this
// call, any component that calls LoadKubeconfig for this tenant will receive
// ErrSecretNotFound, cutting off all control-plane management access to that
// tenant environment.
func RevokeKubeconfig(ctx context.Context, c SecretsClient, tenantID string) error {
	if err := c.Delete(ctx, kubeconfigPath(tenantID)); err != nil {
		return cferrors.Wrapf(ErrVaultDelete, "tenant %s kubeconfig: %v", tenantID, err)
	}
	return nil
}
