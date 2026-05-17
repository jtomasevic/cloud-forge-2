package cilium

import (
	"context"
	"errors"
	"testing"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestApplyDefaultDenyPolicy_Validation(t *testing.T) {
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	c := newCfCiliumClientForTest(dc)
	err := c.ApplyDefaultDenyPolicy(context.Background(), "", "net-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !isInvalidInput(err) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestApplyIngressPolicy_ValidationPort(t *testing.T) {
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	c := newCfCiliumClientForTest(dc)
	err := c.ApplyIngressPolicy(context.Background(), IngressPolicyParams{
		VClusterNamespace:  "ns",
		NetworkID:          "n",
		PublicEndpointPort: 0,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !isInvalidInput(err) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestGetPolicy_NotExists(t *testing.T) {
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	c := newCfCiliumClientForTest(dc)
	info, err := c.GetPolicy(context.Background(), "ns1", "missing-policy")
	if err != nil {
		t.Fatal(err)
	}
	if info.Exists {
		t.Fatal("expected Exists=false")
	}
}

func isInvalidInput(err error) bool {
	var ce *cferrors.CFError
	if !errors.As(err, &ce) {
		return false
	}
	return ce.Code() == cferrors.CodeInvalidInput
}
