package client_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	"github.com/jtomasevic/cloud-forge-2/libs/openbao/pkg/client"
	"github.com/jtomasevic/cloud-forge-2/libs/openbao/pkg/client/mock"
)

const testTenantID = "tenant-abc123"

var testKubeconfig = []byte(`apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: https://k8s.example.com`)

// expectedPath is the canonical vault path for testTenantID.
const expectedPath = "secret/tenants/tenant-abc123/kubeconfig"

// ---------- StoreKubeconfig ----------

func TestStoreKubeconfig_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	encoded := base64.StdEncoding.EncodeToString(testKubeconfig)
	m.EXPECT().
		Write(gomock.Any(), expectedPath, map[string]interface{}{"data": encoded}).
		Return(nil)

	err := client.StoreKubeconfig(context.Background(), m, testTenantID, testKubeconfig)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestStoreKubeconfig_WriteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	m.EXPECT().
		Write(gomock.Any(), expectedPath, gomock.Any()).
		Return(cferrors.ErrUnavailable)

	err := client.StoreKubeconfig(context.Background(), m, testTenantID, testKubeconfig)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, client.ErrVaultWrite) {
		t.Errorf("expected ErrVaultWrite, got %v", err)
	}
}

func TestStoreKubeconfig_EmptyBytes(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	encoded := base64.StdEncoding.EncodeToString([]byte{})
	m.EXPECT().
		Write(gomock.Any(), expectedPath, map[string]interface{}{"data": encoded}).
		Return(nil)

	err := client.StoreKubeconfig(context.Background(), m, testTenantID, []byte{})
	if err != nil {
		t.Errorf("expected nil error for empty kubeconfig, got %v", err)
	}
}

// ---------- LoadKubeconfig ----------

func TestLoadKubeconfig_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	encoded := base64.StdEncoding.EncodeToString(testKubeconfig)
	m.EXPECT().
		Read(gomock.Any(), expectedPath).
		Return(map[string]interface{}{"data": encoded}, nil)

	got, err := client.LoadKubeconfig(context.Background(), m, testTenantID)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if string(got) != string(testKubeconfig) {
		t.Errorf("decoded kubeconfig mismatch\nwant: %s\ngot:  %s", testKubeconfig, got)
	}
}

func TestLoadKubeconfig_SecretNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	m.EXPECT().
		Read(gomock.Any(), expectedPath).
		Return(nil, client.ErrSecretNotFound)

	_, err := client.LoadKubeconfig(context.Background(), m, testTenantID)
	if !errors.Is(err, client.ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestLoadKubeconfig_ReadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	m.EXPECT().
		Read(gomock.Any(), expectedPath).
		Return(nil, cferrors.ErrUnavailable)

	_, err := client.LoadKubeconfig(context.Background(), m, testTenantID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoadKubeconfig_MissingDataKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	m.EXPECT().
		Read(gomock.Any(), expectedPath).
		Return(map[string]interface{}{"other_key": "value"}, nil)

	_, err := client.LoadKubeconfig(context.Background(), m, testTenantID)
	if err == nil {
		t.Fatal("expected error for missing 'data' key")
	}
	if !errors.Is(err, client.ErrVaultCorrupt) {
		t.Errorf("expected ErrVaultCorrupt, got %v", err)
	}
}

func TestLoadKubeconfig_DataNotString(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	m.EXPECT().
		Read(gomock.Any(), expectedPath).
		Return(map[string]interface{}{"data": 12345}, nil)

	_, err := client.LoadKubeconfig(context.Background(), m, testTenantID)
	if err == nil {
		t.Fatal("expected error for non-string 'data' value")
	}
	if !errors.Is(err, client.ErrVaultCorrupt) {
		t.Errorf("expected ErrVaultCorrupt, got %v", err)
	}
}

func TestLoadKubeconfig_InvalidBase64(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	m.EXPECT().
		Read(gomock.Any(), expectedPath).
		Return(map[string]interface{}{"data": "!!!not-valid-base64!!!"}, nil)

	_, err := client.LoadKubeconfig(context.Background(), m, testTenantID)
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
	if !errors.Is(err, client.ErrVaultCorrupt) {
		t.Errorf("expected ErrVaultCorrupt, got %v", err)
	}
}

// ---------- RevokeKubeconfig ----------

func TestRevokeKubeconfig_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	m.EXPECT().
		Delete(gomock.Any(), expectedPath).
		Return(nil)

	err := client.RevokeKubeconfig(context.Background(), m, testTenantID)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRevokeKubeconfig_DeleteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	m.EXPECT().
		Delete(gomock.Any(), expectedPath).
		Return(cferrors.ErrForbidden)

	err := client.RevokeKubeconfig(context.Background(), m, testTenantID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, client.ErrVaultDelete) {
		t.Errorf("expected ErrVaultDelete, got %v", err)
	}
}

func TestRevokeKubeconfig_SecretNotFoundSucceeds(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	m.EXPECT().
		Delete(gomock.Any(), expectedPath).
		Return(client.ErrSecretNotFound)

	err := client.RevokeKubeconfig(context.Background(), m, testTenantID)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRevokeKubeconfig_UsesCorrectPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)

	// Verify the exact path passed to Delete.
	m.EXPECT().
		Delete(gomock.Any(), "secret/tenants/my-tenant/kubeconfig").
		Return(nil)

	err := client.RevokeKubeconfig(context.Background(), m, "my-tenant")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
