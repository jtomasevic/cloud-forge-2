//go:build integration

package client

import (
	"context"
	"errors"
	"testing"
)

// ---------- StoreKubeconfig / LoadKubeconfig / RevokeKubeconfig ----------

func TestStoreLoadKubeconfig_Integration_RoundTrip(t *testing.T) {
	ctx := context.Background()
	c := integrationClient(t)
	tenantID := "tenant-" + t.Name()

	kubeconfig := []byte("apiVersion: v1\nkind: Config\n# integration test kubeconfig")

	if err := StoreKubeconfig(ctx, c, tenantID, kubeconfig); err != nil {
		t.Fatalf("StoreKubeconfig: %v", err)
	}

	got, err := LoadKubeconfig(ctx, c, tenantID)
	if err != nil {
		t.Fatalf("LoadKubeconfig: %v", err)
	}

	if string(got) != string(kubeconfig) {
		t.Errorf("kubeconfig mismatch\n  want: %q\n   got: %q", kubeconfig, got)
	}
}

func TestStoreKubeconfig_Integration_OverwritesExisting(t *testing.T) {
	ctx := context.Background()
	c := integrationClient(t)
	tenantID := "tenant-" + t.Name()

	first := []byte("first-kubeconfig")
	second := []byte("second-kubeconfig")

	if err := StoreKubeconfig(ctx, c, tenantID, first); err != nil {
		t.Fatalf("first StoreKubeconfig: %v", err)
	}
	if err := StoreKubeconfig(ctx, c, tenantID, second); err != nil {
		t.Fatalf("second StoreKubeconfig: %v", err)
	}

	got, err := LoadKubeconfig(ctx, c, tenantID)
	if err != nil {
		t.Fatalf("LoadKubeconfig: %v", err)
	}
	if string(got) != string(second) {
		t.Errorf("expected overwritten kubeconfig %q, got %q", second, got)
	}
}

func TestLoadKubeconfig_Integration_NotFoundReturnsErrSecretNotFound(t *testing.T) {
	ctx := context.Background()
	c := integrationClient(t)

	// A tenant that was never stored.
	_, err := LoadKubeconfig(ctx, c, "tenant-never-stored-"+t.Name())
	if err == nil {
		t.Fatal("expected error for missing kubeconfig, got nil")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %T: %v", err, err)
	}
}

func TestRevokeKubeconfig_Integration_RemovesSecret(t *testing.T) {
	ctx := context.Background()
	c := integrationClient(t)
	tenantID := "tenant-" + t.Name()

	if err := StoreKubeconfig(ctx, c, tenantID, []byte("cfg")); err != nil {
		t.Fatalf("StoreKubeconfig: %v", err)
	}
	if err := RevokeKubeconfig(ctx, c, tenantID); err != nil {
		t.Fatalf("RevokeKubeconfig: %v", err)
	}

	_, err := LoadKubeconfig(ctx, c, tenantID)
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound after revoke, got %v", err)
	}
}

func TestRevokeKubeconfig_Integration_NeverStoredSucceeds(t *testing.T) {
	// Revoking a kubeconfig that was never stored must not return an error
	// (Vault/OpenBao KV v1 DELETE is idempotent).
	ctx := context.Background()
	c := integrationClient(t)

	if err := RevokeKubeconfig(ctx, c, "tenant-never-stored-"+t.Name()); err != nil {
		t.Errorf("expected nil error for revoke of non-existent kubeconfig, got: %v", err)
	}
}

func TestStoreLoadRevoke_Integration_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	c := integrationClient(t)
	tenantID := "tenant-lifecycle-" + t.Name()

	original := []byte("---\napiVersion: v1\nkind: Config")

	// 1. store
	if err := StoreKubeconfig(ctx, c, tenantID, original); err != nil {
		t.Fatalf("StoreKubeconfig: %v", err)
	}

	// 2. load and verify
	loaded, err := LoadKubeconfig(ctx, c, tenantID)
	if err != nil {
		t.Fatalf("LoadKubeconfig: %v", err)
	}
	if string(loaded) != string(original) {
		t.Fatalf("load mismatch: want %q got %q", original, loaded)
	}

	// 3. revoke
	if err := RevokeKubeconfig(ctx, c, tenantID); err != nil {
		t.Fatalf("RevokeKubeconfig: %v", err)
	}

	// 4. subsequent load must fail with ErrSecretNotFound
	_, err = LoadKubeconfig(ctx, c, tenantID)
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound after revoke, got %v", err)
	}
}
