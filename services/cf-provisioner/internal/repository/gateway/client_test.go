package gateway

import (
	"context"
	"errors"
	"testing"

	gatewayfake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

func TestGetHTTPRoute_NotFound(t *testing.T) {
	cs := gatewayfake.NewClientset()
	c := newCfGatewayClientForTest(cs)
	_, err := c.GetHTTPRoute(context.Background(), "ns", "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrHTTPRouteNotFound) {
		t.Fatalf("expected ErrHTTPRouteNotFound, got %v", err)
	}
}

func TestCreateHTTPRoute_Validation(t *testing.T) {
	cs := gatewayfake.NewClientset()
	c := newCfGatewayClientForTest(cs)
	_, err := c.CreateHTTPRoute(context.Background(), HTTPRouteParams{})
	if err == nil {
		t.Fatal("expected error")
	}
	var ce *cferrors.CFError
	if !errors.As(err, &ce) || ce.Code() != cferrors.CodeInvalidInput {
		t.Fatalf("expected invalid input, got %v", err)
	}
}
