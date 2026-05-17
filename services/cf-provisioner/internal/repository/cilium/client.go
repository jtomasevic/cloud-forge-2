package cilium

import (
	"context"
	"strconv"
	"strings"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var ciliumGVR = schema.GroupVersionResource{Group: "cilium.io", Version: "v2", Resource: "ciliumnetworkpolicies"}

type cfCiliumClient struct {
	dyn dynamic.Interface
}

func newCiliumClient(hostKubeconfig []byte) (CiliumClient, error) {
	if len(hostKubeconfig) == 0 {
		return nil, cferrors.Wrap(cferrors.CodeInvalidInput, "host kubeconfig is required", cferrors.ErrInvalidInput)
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(hostKubeconfig)
	if err != nil {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "invalid host kubeconfig", cferrors.ErrInternal)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "dynamic client init failed", cferrors.ErrInternal)
	}
	return &cfCiliumClient{dyn: dyn}, nil
}

// newCfCiliumClientForTest wires a fake dynamic client (package tests only).
func newCfCiliumClientForTest(dyn dynamic.Interface) CiliumClient {
	return &cfCiliumClient{dyn: dyn}
}

func defaultDenyName(networkID string) string {
	return "default-deny-egress-" + sanitizeNetworkID(networkID)
}

func ingressPolicyName(networkID string) string {
	return "internet-ingress-" + sanitizeNetworkID(networkID)
}

func sanitizeNetworkID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = strings.ReplaceAll(id, " ", "-")
	return id
}

func (c *cfCiliumClient) ApplyDefaultDenyPolicy(ctx context.Context, vclusterNamespace, networkID string) error {
	if strings.TrimSpace(vclusterNamespace) == "" || strings.TrimSpace(networkID) == "" {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "namespace and networkID are required", cferrors.ErrInvalidInput)
	}
	obj := buildDefaultDenyPolicy(vclusterNamespace, networkID)
	_, err := c.dyn.Resource(ciliumGVR).Namespace(vclusterNamespace).Create(ctx, obj, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return cferrors.Wrap(ErrPolicyApplyFailed.Code(), "apply default-deny cilium policy", err)
	}
	return nil
}

func buildDefaultDenyPolicy(vclusterNamespace, networkID string) *unstructured.Unstructured {
	name := defaultDenyName(networkID)
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "cilium.io", Version: "v2", Kind: "CiliumNetworkPolicy"})
	u.SetName(name)
	u.SetNamespace(vclusterNamespace)
	_ = unstructured.SetNestedMap(u.Object, map[string]interface{}{}, "spec", "endpointSelector")
	_ = unstructured.SetNestedSlice(u.Object, []interface{}{
		map[string]interface{}{
			"toEntities": []interface{}{"cluster"},
		},
	}, "spec", "egressDeny")
	return u
}

func (c *cfCiliumClient) ApplyIngressPolicy(ctx context.Context, params IngressPolicyParams) error {
	if strings.TrimSpace(params.VClusterNamespace) == "" || strings.TrimSpace(params.NetworkID) == "" {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "VClusterNamespace and NetworkID are required", cferrors.ErrInvalidInput)
	}
	if params.PublicEndpointPort <= 0 || params.PublicEndpointPort > 65535 {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "PublicEndpointPort must be between 1 and 65535", cferrors.ErrInvalidInput)
	}
	obj := buildIngressPolicy(params)
	_, err := c.dyn.Resource(ciliumGVR).Namespace(params.VClusterNamespace).Create(ctx, obj, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return cferrors.Wrap(ErrPolicyApplyFailed.Code(), "apply ingress cilium policy", err)
	}
	return nil
}

func buildIngressPolicy(params IngressPolicyParams) *unstructured.Unstructured {
	name := ingressPolicyName(params.NetworkID)
	portStr := strconv.Itoa(params.PublicEndpointPort)
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "cilium.io", Version: "v2", Kind: "CiliumNetworkPolicy"})
	u.SetName(name)
	u.SetNamespace(params.VClusterNamespace)
	_ = unstructured.SetNestedMap(u.Object, map[string]interface{}{}, "spec", "endpointSelector")
	ingress := []interface{}{
		map[string]interface{}{
			"fromEntities": []interface{}{"world"},
			"toPorts": []interface{}{
				map[string]interface{}{
					"ports": []interface{}{
						map[string]interface{}{
							"port":     portStr,
							"protocol": "TCP",
						},
					},
				},
			},
		},
	}
	_ = unstructured.SetNestedSlice(u.Object, ingress, "spec", "ingress")
	return u
}

func (c *cfCiliumClient) RemovePolicy(ctx context.Context, namespace, policyName string) error {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(policyName) == "" {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "namespace and policyName are required", cferrors.ErrInvalidInput)
	}
	err := c.dyn.Resource(ciliumGVR).Namespace(namespace).Delete(ctx, policyName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return cferrors.Wrap(ErrPolicyApplyFailed.Code(), "delete cilium policy", err)
	}
	return nil
}

func (c *cfCiliumClient) GetPolicy(ctx context.Context, namespace, policyName string) (PolicyInfo, error) {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(policyName) == "" {
		return PolicyInfo{}, cferrors.Wrap(cferrors.CodeInvalidInput, "namespace and policyName are required", cferrors.ErrInvalidInput)
	}
	_, err := c.dyn.Resource(ciliumGVR).Namespace(namespace).Get(ctx, policyName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return PolicyInfo{Name: policyName, Namespace: namespace, Exists: false}, nil
	}
	if err != nil {
		return PolicyInfo{}, cferrors.Wrap(cferrors.CodeInternal, "get cilium policy", err)
	}
	return PolicyInfo{Name: policyName, Namespace: namespace, Exists: true}, nil
}
