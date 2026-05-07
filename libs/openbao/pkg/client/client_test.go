// Package client white-box tests.
// Using package client (not client_test) gives access to the unexported
// newWithLogical constructor, enabling injection of mock.MocklogicalAPI.
package client

import (
	"context"
	"errors"
	"net/http"
	"testing"

	api "github.com/openbao/openbao/api/v2"
	"go.uber.org/mock/gomock"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	"github.com/jtomasevic/cloud-forge-2/libs/openbao/pkg/client/mock"
)

// ---------- New ----------

func TestNew_Success(t *testing.T) {
	c, err := New(Config{Address: "http://localhost:8200", Token: "root"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNew_SuccessWithoutToken(t *testing.T) {
	c, err := New(Config{Address: "http://localhost:8200"})
	if err != nil {
		t.Fatalf("expected nil error for empty token, got %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNew_EmptyAddress(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error for empty address")
	}
	if !errors.Is(err, ErrClientInit) {
		t.Errorf("expected ErrClientInit, got %T: %v", err, err)
	}
}

func TestNew_InvalidScheme(t *testing.T) {
	_, err := New(Config{Address: "://bad-scheme"})
	if err == nil {
		t.Fatal("expected error for invalid address scheme")
	}
}

// ---------- Write ----------

func TestWrite_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		WriteWithContext(gomock.Any(), "secret/app/key", gomock.Any()).
		Return(nil, nil)

	c := newWithLogical(m)
	err := c.Write(context.Background(), "secret/app/key", map[string]interface{}{"value": "hello"})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestWrite_404(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		WriteWithContext(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, &api.ResponseError{StatusCode: http.StatusNotFound})

	c := newWithLogical(m)
	err := c.Write(context.Background(), "secret/app/key", nil)
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestWrite_403(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		WriteWithContext(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, &api.ResponseError{StatusCode: http.StatusForbidden})

	c := newWithLogical(m)
	err := c.Write(context.Background(), "secret/app/key", nil)
	if !errors.Is(err, cferrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestWrite_500(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		WriteWithContext(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, &api.ResponseError{StatusCode: http.StatusInternalServerError})

	c := newWithLogical(m)
	err := c.Write(context.Background(), "secret/app/key", nil)
	if !errors.Is(err, cferrors.ErrInternal) {
		t.Errorf("expected ErrInternal, got %v", err)
	}
}

func TestWrite_NetworkError(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		WriteWithContext(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection refused"))

	c := newWithLogical(m)
	err := c.Write(context.Background(), "secret/app/key", nil)
	if !errors.Is(err, cferrors.ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}

// ---------- Read ----------

func TestRead_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	want := map[string]interface{}{"username": "admin", "password": "secret"}
	m.EXPECT().
		ReadWithContext(gomock.Any(), "secret/creds").
		Return(&api.Secret{Data: want}, nil)

	c := newWithLogical(m)
	got, err := c.Read(context.Background(), "secret/creds")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got["username"] != "admin" {
		t.Errorf("unexpected data: %v", got)
	}
}

func TestRead_NilSecret(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		ReadWithContext(gomock.Any(), gomock.Any()).
		Return(nil, nil)

	c := newWithLogical(m)
	_, err := c.Read(context.Background(), "secret/missing")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound for nil secret, got %v", err)
	}
}

func TestRead_NilSecretData(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		ReadWithContext(gomock.Any(), gomock.Any()).
		Return(&api.Secret{Data: nil}, nil)

	c := newWithLogical(m)
	_, err := c.Read(context.Background(), "secret/missing")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound for nil Data, got %v", err)
	}
}

func TestRead_404(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		ReadWithContext(gomock.Any(), gomock.Any()).
		Return(nil, &api.ResponseError{StatusCode: http.StatusNotFound})

	c := newWithLogical(m)
	_, err := c.Read(context.Background(), "secret/missing")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestRead_403(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		ReadWithContext(gomock.Any(), gomock.Any()).
		Return(nil, &api.ResponseError{StatusCode: http.StatusForbidden})

	c := newWithLogical(m)
	_, err := c.Read(context.Background(), "secret/denied")
	if !errors.Is(err, cferrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestRead_500(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		ReadWithContext(gomock.Any(), gomock.Any()).
		Return(nil, &api.ResponseError{StatusCode: http.StatusInternalServerError})

	c := newWithLogical(m)
	_, err := c.Read(context.Background(), "secret/broken")
	if !errors.Is(err, cferrors.ErrInternal) {
		t.Errorf("expected ErrInternal, got %v", err)
	}
}

func TestRead_NetworkError(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		ReadWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("dial timeout"))

	c := newWithLogical(m)
	_, err := c.Read(context.Background(), "secret/down")
	if !errors.Is(err, cferrors.ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}

// ---------- Delete ----------

func TestDelete_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		DeleteWithContext(gomock.Any(), "secret/app/key").
		Return(nil, nil)

	c := newWithLogical(m)
	err := c.Delete(context.Background(), "secret/app/key")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestDelete_403(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		DeleteWithContext(gomock.Any(), gomock.Any()).
		Return(nil, &api.ResponseError{StatusCode: http.StatusForbidden})

	c := newWithLogical(m)
	err := c.Delete(context.Background(), "secret/app/key")
	if !errors.Is(err, cferrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestDelete_NetworkError(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		DeleteWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("network unreachable"))

	c := newWithLogical(m)
	err := c.Delete(context.Background(), "secret/app/key")
	if !errors.Is(err, cferrors.ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}

// ---------- List ----------

func TestList_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		ListWithContext(gomock.Any(), "secret/tenants").
		Return(&api.Secret{
			Data: map[string]interface{}{
				"keys": []interface{}{"tenant-a/", "tenant-b/"},
			},
		}, nil)

	c := newWithLogical(m)
	keys, err := c.List(context.Background(), "secret/tenants")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(keys) != 2 || keys[0] != "tenant-a/" {
		t.Errorf("unexpected keys: %v", keys)
	}
}

func TestList_NilSecret(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		ListWithContext(gomock.Any(), gomock.Any()).
		Return(nil, nil)

	c := newWithLogical(m)
	keys, err := c.List(context.Background(), "secret/empty")
	if err != nil {
		t.Errorf("expected nil error for empty list, got %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected empty slice, got %v", keys)
	}
}

func TestList_NilData(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		ListWithContext(gomock.Any(), gomock.Any()).
		Return(&api.Secret{Data: nil}, nil)

	c := newWithLogical(m)
	keys, err := c.List(context.Background(), "secret/empty")
	if err != nil {
		t.Errorf("expected nil error for nil data, got %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected empty slice, got %v", keys)
	}
}

func TestList_NoKeysField(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		ListWithContext(gomock.Any(), gomock.Any()).
		Return(&api.Secret{Data: map[string]interface{}{"other": "value"}}, nil)

	c := newWithLogical(m)
	keys, err := c.List(context.Background(), "secret/prefix")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected empty slice for missing keys field, got %v", keys)
	}
}

func TestList_InvalidKeysType(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		ListWithContext(gomock.Any(), gomock.Any()).
		Return(&api.Secret{Data: map[string]interface{}{"keys": "not-a-slice"}}, nil)

	c := newWithLogical(m)
	_, err := c.List(context.Background(), "secret/prefix")
	if !errors.Is(err, cferrors.ErrInternal) {
		t.Errorf("expected ErrInternal for malformed keys, got %v", err)
	}
}

func TestList_NetworkError(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMocklogicalAPI(ctrl)

	m.EXPECT().
		ListWithContext(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection reset"))

	c := newWithLogical(m)
	_, err := c.List(context.Background(), "secret/prefix")
	if !errors.Is(err, cferrors.ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}
