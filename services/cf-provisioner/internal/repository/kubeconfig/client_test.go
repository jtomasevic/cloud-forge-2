package kubeconfig

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	openbao "github.com/jtomasevic/cloud-forge-2/libs/openbao/pkg/client"
	"github.com/jtomasevic/cloud-forge-2/libs/openbao/pkg/client/mock"
)

func TestLoad_NotFoundMapsToSentinel(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock.NewMockSecretsClient(ctrl)
	m.EXPECT().
		Read(gomock.Any(), "secret/tenants/t1/kubeconfig").
		Return(nil, openbao.ErrSecretNotFound)

	r := New(m)
	_, err := r.Load(context.Background(), "t1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrKubeconfigNotFound) {
		t.Fatalf("expected ErrKubeconfigNotFound, got %v", err)
	}
}

func TestStore_NilSecretsClient(t *testing.T) {
	r := New(nil)
	err := r.Store(context.Background(), "t1", []byte("x"))
	if err == nil {
		t.Fatal("expected error")
	}
}
