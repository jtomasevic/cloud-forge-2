//go:build integration

package client

import (
	"context"
	"errors"
	"fmt"
	"testing"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

// ---------- New ----------

func TestNew_Integration_ConnectsToOpenBao(t *testing.T) {
	c, err := New(Config{
		Address: integrationBaoAddr,
		Token:   integrationBaoToken,
	})
	if err != nil {
		t.Fatalf("expected successful client creation, got: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNew_Integration_FailsWithEmptyAddress(t *testing.T) {
	_, err := New(Config{Token: integrationBaoToken})
	if err == nil {
		t.Fatal("expected error for empty address")
	}
	if !errors.Is(err, ErrClientInit) {
		t.Errorf("expected ErrClientInit, got %T: %v", err, err)
	}
}

// ---------- Write / Read ----------

func TestWriteRead_Integration_RoundTrip(t *testing.T) {
	ctx := context.Background()
	c := integrationClient(t)

	path := uniquePath(t)
	data := map[string]interface{}{"key": "value", "num": "42"}

	if err := c.Write(ctx, path, data); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := c.Read(ctx, path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	// KV v1 returns the data directly; check one field.
	if got["key"] != "value" {
		t.Errorf("expected key=value, got %v", got["key"])
	}
}

func TestWrite_Integration_OverwritesExistingSecret(t *testing.T) {
	ctx := context.Background()
	c := integrationClient(t)
	path := uniquePath(t)

	if err := c.Write(ctx, path, map[string]interface{}{"v": "first"}); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if err := c.Write(ctx, path, map[string]interface{}{"v": "second"}); err != nil {
		t.Fatalf("second Write: %v", err)
	}

	data, err := c.Read(ctx, path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if data["v"] != "second" {
		t.Errorf("expected overwritten value 'second', got %v", data["v"])
	}
}

// ---------- Read — not found ----------

func TestRead_Integration_MissingPathReturnsErrSecretNotFound(t *testing.T) {
	ctx := context.Background()
	c := integrationClient(t)

	_, err := c.Read(ctx, "secret/nonexistent/path/that/was/never/written")
	if err == nil {
		t.Fatal("expected ErrSecretNotFound, got nil")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %T: %v", err, err)
	}
	if !errors.Is(err, cferrors.ErrNotFound) {
		t.Errorf("expected CodeNotFound in error chain, got %v", err)
	}
}

// ---------- Delete ----------

func TestDelete_Integration_RemovesSecret(t *testing.T) {
	ctx := context.Background()
	c := integrationClient(t)
	path := uniquePath(t)

	if err := c.Write(ctx, path, map[string]interface{}{"x": "y"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := c.Delete(ctx, path); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := c.Read(ctx, path)
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound after delete, got %v", err)
	}
}

func TestDelete_Integration_NonExistentPathSucceeds(t *testing.T) {
	// Deleting a path that never existed must not return an error.
	ctx := context.Background()
	c := integrationClient(t)

	if err := c.Delete(ctx, "secret/never/existed/"+t.Name()); err != nil {
		t.Errorf("expected nil error for delete of non-existent path, got: %v", err)
	}
}

// ---------- List ----------

func TestList_Integration_ReturnsWrittenKeys(t *testing.T) {
	ctx := context.Background()
	c := integrationClient(t)

	// Use a unique prefix per test to avoid interference.
	prefix := fmt.Sprintf("secret/cf-int-test-list/%s", t.Name())

	// Write three keys under the prefix.
	for _, key := range []string{"alpha", "beta", "gamma"} {
		if err := c.Write(ctx, prefix+"/"+key, map[string]interface{}{"k": key}); err != nil {
			t.Fatalf("Write %s: %v", key, err)
		}
	}

	keys, err := c.List(ctx, prefix)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d: %v", len(keys), keys)
	}
}

func TestList_Integration_EmptyPrefixReturnsEmptySlice(t *testing.T) {
	ctx := context.Background()
	c := integrationClient(t)

	// A prefix that was never written under should return empty, not an error.
	keys, err := c.List(ctx, "secret/cf-int-empty-prefix/"+t.Name())
	if err != nil {
		t.Fatalf("expected nil error for empty list, got: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected empty slice, got %v", keys)
	}
}

// ---------- helpers ----------

// uniquePath returns a unique KV v1 path for this test to avoid cross-test
// interference when tests run against a shared OpenBao instance.
func uniquePath(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("secret/cf-int-test/%s", t.Name())
}
