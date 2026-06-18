package gateway

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
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

func TestCreateHTTPRoute_NetworkFlowStillUsesSingleBackendRule(t *testing.T) {
	ctx := context.Background()
	cs := gatewayfake.NewClientset()
	c := newCfGatewayClientForTest(cs)

	if _, err := c.CreateHTTPRoute(ctx, HTTPRouteParams{
		Name:           "gw-550e8400",
		Namespace:      "cf-provisioner",
		Hostname:       "api.local.cloudforge.dev",
		BackendService: "cf-backend-placeholder",
		BackendPort:    80,
	}); err != nil {
		t.Fatalf("create network route: %v", err)
	}

	route, err := cs.GatewayV1().HTTPRoutes("cf-provisioner").Get(ctx, "gw-550e8400", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get route: %v", err)
	}
	if len(route.Spec.Rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(route.Spec.Rules))
	}
	ref := route.Spec.Rules[0].BackendRefs[0].BackendObjectReference
	if string(ref.Name) != "cf-backend-placeholder" || ref.Port == nil || *ref.Port != 80 {
		t.Fatalf("network backend ref = %+v, want placeholder:80", ref)
	}
	if len(route.Spec.Rules[0].Matches) != 0 {
		t.Fatalf("network route should keep Gateway API default / match, got %+v", route.Spec.Rules[0].Matches)
	}
}

func TestCreateAppServiceHTTPRoute_TargetsSpecificBackendAndSwaggerPaths(t *testing.T) {
	ctx := context.Background()
	cs := gatewayfake.NewClientset()
	c := newCfGatewayClientForTest(cs)
	appServiceID := "550e8400-e29b-41d4-a716-446655440003"

	info, err := c.CreateAppServiceHTTPRoute(ctx, AppServiceHTTPRouteParams{
		Namespace:      "cf-provisioner",
		AppServiceID:   appServiceID,
		Hostname:       "orders.local.cloudforge.dev",
		BackendService: "orders-api",
		BackendPort:    8080,
	})
	if err != nil {
		t.Fatalf("create app-service route: %v", err)
	}
	if info.Name != AppServiceHTTPRouteName(appServiceID) {
		t.Fatalf("route name = %q, want %q", info.Name, AppServiceHTTPRouteName(appServiceID))
	}

	route, err := cs.GatewayV1().HTTPRoutes("cf-provisioner").Get(ctx, AppServiceHTTPRouteName(appServiceID), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get route: %v", err)
	}
	if route.Labels[labelAppServiceID] != appServiceID || route.Labels[labelRouteKind] != labelRouteKindAppService {
		t.Fatalf("route labels not scoped to app service: %+v", route.Labels)
	}
	if got := string(route.Spec.Hostnames[0]); got != "orders.local.cloudforge.dev" {
		t.Fatalf("hostname = %q", got)
	}
	assertRule(t, route.Spec.Rules[0], gatewayv1.PathMatchExact, "/swagger", "orders-api", 8080)
	assertRule(t, route.Spec.Rules[1], gatewayv1.PathMatchPathPrefix, "/swagger/", "orders-api", 8080)
	assertRule(t, route.Spec.Rules[2], gatewayv1.PathMatchExact, "/openapi.json", "orders-api", 8080)
	assertRule(t, route.Spec.Rules[3], gatewayv1.PathMatchPathPrefix, "/", "orders-api", 8080)
	for _, rule := range route.Spec.Rules {
		for _, ref := range rule.BackendRefs {
			if ref.Name == "cf-backend-placeholder" {
				t.Fatalf("app-service route must not target placeholder backend: %+v", route.Spec.Rules)
			}
		}
	}
}

func TestCreateAppServiceHTTPRoute_CanSendDocsToAdapter(t *testing.T) {
	ctx := context.Background()
	cs := gatewayfake.NewClientset()
	c := newCfGatewayClientForTest(cs)
	appServiceID := "550e8400-e29b-41d4-a716-446655440003"

	if _, err := c.CreateAppServiceHTTPRoute(ctx, AppServiceHTTPRouteParams{
		Namespace:      "cf-provisioner",
		AppServiceID:   appServiceID,
		Hostname:       "orders.local.cloudforge.dev",
		BackendService: "orders-api",
		BackendPort:    8080,
		DocsBackend: &HTTPRouteBackend{
			Service: "cf-docs-adapter",
			Port:    8090,
		},
	}); err != nil {
		t.Fatalf("create app-service route: %v", err)
	}

	route, err := cs.GatewayV1().HTTPRoutes("cf-provisioner").Get(ctx, AppServiceHTTPRouteName(appServiceID), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get route: %v", err)
	}
	assertRule(t, route.Spec.Rules[0], gatewayv1.PathMatchExact, "/swagger", "cf-docs-adapter", 8090)
	assertRule(t, route.Spec.Rules[1], gatewayv1.PathMatchPathPrefix, "/swagger/", "cf-docs-adapter", 8090)
	assertRule(t, route.Spec.Rules[2], gatewayv1.PathMatchExact, "/openapi.json", "cf-docs-adapter", 8090)
	assertRule(t, route.Spec.Rules[3], gatewayv1.PathMatchPathPrefix, "/", "orders-api", 8080)
}

func TestCreateAppServiceHTTPRoute_IsIdempotentAndUpdatesBackend(t *testing.T) {
	ctx := context.Background()
	cs := gatewayfake.NewClientset()
	c := newCfGatewayClientForTest(cs)
	params := AppServiceHTTPRouteParams{
		Namespace:      "cf-provisioner",
		AppServiceID:   "550e8400-e29b-41d4-a716-446655440003",
		Hostname:       "orders.local.cloudforge.dev",
		BackendService: "orders-api",
		BackendPort:    8080,
	}
	if _, err := c.CreateAppServiceHTTPRoute(ctx, params); err != nil {
		t.Fatalf("create app-service route: %v", err)
	}
	params.BackendPort = 9090
	if _, err := c.CreateAppServiceHTTPRoute(ctx, params); err != nil {
		t.Fatalf("update app-service route: %v", err)
	}

	route, err := cs.GatewayV1().HTTPRoutes("cf-provisioner").Get(ctx, AppServiceHTTPRouteName(params.AppServiceID), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get route: %v", err)
	}
	assertRule(t, route.Spec.Rules[3], gatewayv1.PathMatchPathPrefix, "/", "orders-api", 9090)
}

func TestDeleteAppServiceHTTPRoute_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	cs := gatewayfake.NewClientset()
	c := newCfGatewayClientForTest(cs)
	appServiceID := "550e8400-e29b-41d4-a716-446655440003"
	if _, err := c.CreateAppServiceHTTPRoute(ctx, AppServiceHTTPRouteParams{
		Namespace:      "cf-provisioner",
		AppServiceID:   appServiceID,
		Hostname:       "orders.local.cloudforge.dev",
		BackendService: "orders-api",
		BackendPort:    8080,
	}); err != nil {
		t.Fatalf("create app-service route: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := c.DeleteAppServiceHTTPRoute(ctx, "cf-provisioner", appServiceID); err != nil {
			t.Fatalf("delete attempt %d: %v", i+1, err)
		}
	}
	if _, err := c.GetHTTPRoute(ctx, "cf-provisioner", AppServiceHTTPRouteName(appServiceID)); !errors.Is(err, ErrHTTPRouteNotFound) {
		t.Fatalf("expected typed not found after delete, got %v", err)
	}
}

func assertRule(t *testing.T, rule gatewayv1.HTTPRouteRule, wantType gatewayv1.PathMatchType, wantPath, wantBackend string, wantPort int) {
	t.Helper()
	if len(rule.Matches) != 1 || rule.Matches[0].Path == nil {
		t.Fatalf("rule matches = %+v, want one path match", rule.Matches)
	}
	if rule.Matches[0].Path.Type == nil || *rule.Matches[0].Path.Type != wantType {
		t.Fatalf("path type = %v, want %s", rule.Matches[0].Path.Type, wantType)
	}
	if rule.Matches[0].Path.Value == nil || *rule.Matches[0].Path.Value != wantPath {
		t.Fatalf("path = %v, want %s", rule.Matches[0].Path.Value, wantPath)
	}
	if len(rule.BackendRefs) != 1 {
		t.Fatalf("backend refs = %+v, want one", rule.BackendRefs)
	}
	ref := rule.BackendRefs[0].BackendObjectReference
	if string(ref.Name) != wantBackend || ref.Port == nil || int(*ref.Port) != wantPort {
		t.Fatalf("backend ref = %+v, want %s:%d", ref, wantBackend, wantPort)
	}
}
