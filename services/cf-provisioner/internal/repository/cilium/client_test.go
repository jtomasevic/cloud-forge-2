package cilium

import (
	"context"
	"errors"
	"strconv"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
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

func TestApplyAppServiceIngressPolicy_ScopesToAppLabelsAndPort(t *testing.T) {
	ctx := context.Background()
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	c := newCfCiliumClientForTest(dc)
	params := validAppServiceIngressParams()

	if err := c.ApplyAppServiceIngressPolicy(ctx, params); err != nil {
		t.Fatalf("apply app-service ingress policy: %v", err)
	}
	obj := getPolicy(t, dc, params.VClusterNamespace, AppServiceIngressPolicyName(params.AppServiceID))
	selector, ok, err := unstructured.NestedMap(obj.Object, "spec", "endpointSelector", "matchLabels")
	if err != nil || !ok {
		t.Fatalf("selector missing: ok=%v err=%v object=%#v", ok, err, obj.Object)
	}
	wantLabels := map[string]any{
		labelTenantID:     params.TenantID,
		labelNetworkID:    params.NetworkID,
		labelSubnetID:     params.SubnetID,
		labelAppServiceID: params.AppServiceID,
		labelVisibility:   visibilityPublic,
	}
	for key, want := range wantLabels {
		if got := selector[key]; got != want {
			t.Fatalf("selector[%s] = %v, want %v (selector=%#v)", key, got, want, selector)
		}
	}
	if len(selector) != len(wantLabels) {
		t.Fatalf("selector has broad or unexpected labels: %#v", selector)
	}
	assertPolicyPort(t, obj, params.PublicEndpointPort)
}

func TestApplyAppServiceIngressPolicy_IsIdempotentAndUpdatesPort(t *testing.T) {
	ctx := context.Background()
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	c := newCfCiliumClientForTest(dc)
	params := validAppServiceIngressParams()
	if err := c.ApplyAppServiceIngressPolicy(ctx, params); err != nil {
		t.Fatalf("apply app-service ingress policy: %v", err)
	}
	params.PublicEndpointPort = 9090
	if err := c.ApplyAppServiceIngressPolicy(ctx, params); err != nil {
		t.Fatalf("update app-service ingress policy: %v", err)
	}
	obj := getPolicy(t, dc, params.VClusterNamespace, AppServiceIngressPolicyName(params.AppServiceID))
	assertPolicyPort(t, obj, 9090)
}

func TestRemoveAppServiceIngressPolicy_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	c := newCfCiliumClientForTest(dc)
	params := validAppServiceIngressParams()
	if err := c.ApplyAppServiceIngressPolicy(ctx, params); err != nil {
		t.Fatalf("apply app-service ingress policy: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := c.RemoveAppServiceIngressPolicy(ctx, params.VClusterNamespace, params.AppServiceID); err != nil {
			t.Fatalf("delete attempt %d: %v", i+1, err)
		}
	}
	info, err := c.GetPolicy(ctx, params.VClusterNamespace, AppServiceIngressPolicyName(params.AppServiceID))
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if info.Exists {
		t.Fatalf("expected app-service policy removed: %+v", info)
	}
}

func TestApplyAppServiceIngressPolicy_Validation(t *testing.T) {
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	c := newCfCiliumClientForTest(dc)
	params := validAppServiceIngressParams()
	params.AppServiceID = ""
	err := c.ApplyAppServiceIngressPolicy(context.Background(), params)
	if err == nil {
		t.Fatal("expected error")
	}
	if !isInvalidInput(err) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func isInvalidInput(err error) bool {
	var ce *cferrors.CFError
	if !errors.As(err, &ce) {
		return false
	}
	return ce.Code() == cferrors.CodeInvalidInput
}

func validAppServiceIngressParams() AppServiceIngressPolicyParams {
	return AppServiceIngressPolicyParams{
		VClusterNamespace:  "tenant-a",
		TenantID:           "550e8400-e29b-41d4-a716-446655440000",
		NetworkID:          "550e8400-e29b-41d4-a716-446655440001",
		SubnetID:           "550e8400-e29b-41d4-a716-446655440002",
		AppServiceID:       "550e8400-e29b-41d4-a716-446655440003",
		PublicEndpointPort: 8080,
	}
}

func getPolicy(t *testing.T, dc *fake.FakeDynamicClient, namespace, name string) *unstructured.Unstructured {
	t.Helper()
	obj, err := dc.Resource(ciliumGVR).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get policy %s/%s: %v", namespace, name, err)
	}
	return obj
}

func assertPolicyPort(t *testing.T, obj *unstructured.Unstructured, wantPort int) {
	t.Helper()
	ingress, ok, err := unstructured.NestedSlice(obj.Object, "spec", "ingress")
	if err != nil || !ok || len(ingress) != 1 {
		t.Fatalf("ingress missing: ok=%v err=%v object=%#v", ok, err, obj.Object)
	}
	entry, ok := ingress[0].(map[string]interface{})
	if !ok {
		t.Fatalf("ingress[0] has unexpected type: %#v", ingress[0])
	}
	ports, ok := entry["toPorts"].([]interface{})
	if !ok || len(ports) != 1 {
		t.Fatalf("toPorts missing: %#v", entry)
	}
	toPort, ok := ports[0].(map[string]interface{})
	if !ok {
		t.Fatalf("toPorts[0] has unexpected type: %#v", ports[0])
	}
	portBlocks, ok := toPort["ports"].([]interface{})
	if !ok || len(portBlocks) != 1 {
		t.Fatalf("ports block missing: %#v", toPort)
	}
	port, ok := portBlocks[0].(map[string]interface{})
	if !ok {
		t.Fatalf("ports[0] has unexpected type: %#v", portBlocks[0])
	}
	if got := port["port"]; got != strconv.Itoa(wantPort) {
		t.Fatalf("policy port = %v, want %d", got, wantPort)
	}
	if got := port["protocol"]; got != "TCP" {
		t.Fatalf("policy protocol = %v, want TCP", got)
	}
}
