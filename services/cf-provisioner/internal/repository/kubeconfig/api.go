package kubeconfig

import (
	"context"

	openbao "github.com/jtomasevic/cloud-forge-2/libs/openbao/pkg/client"
)

// KubeconfigRepository stores and retrieves tenant kubeconfigs via OpenBao.
type KubeconfigRepository interface {
	Store(ctx context.Context, tenantID string, kubeconfigBytes []byte) error
	Load(ctx context.Context, tenantID string) ([]byte, error)
	Revoke(ctx context.Context, tenantID string) error
}

// New returns a repository backed by the shared [openbao.SecretsClient].
func New(secretsClient openbao.SecretsClient) KubeconfigRepository {
	return &cfKubeconfigRepository{secrets: secretsClient}
}
