package cilium

import (
	"context"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"unicode"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

var ciliumGVR = schema.GroupVersionResource{Group: "cilium.io", Version: "v2", Resource: "ciliumnetworkpolicies"}

const (
	appServiceIngressPolicyPrefix = "appsvc-ingress-"
	labelTenantID                 = "cloudforge.io/tenant-id"
	labelNetworkID                = "cloudforge.io/network-id"
	labelSubnetID                 = "cloudforge.io/subnet-id"
	labelAppServiceID             = "cloudforge.io/app-service-id"
	labelVisibility               = "cloudforge.io/visibility"
	visibilityPublic              = "public"
)

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

func AppServiceIngressPolicyName(appServiceID string) string {
	return nameWithPrefix(appServiceIngressPolicyPrefix, appServiceID)
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

func (c *cfCiliumClient) ApplyAppServiceIngressPolicy(ctx context.Context, params AppServiceIngressPolicyParams) error {
	normalized, err := normalizeAppServiceIngressPolicyParams(params)
	if err != nil {
		return err
	}
	obj := buildAppServiceIngressPolicy(normalized)
	if err := c.upsertPolicy(ctx, obj); err != nil {
		return cferrors.Wrap(ErrPolicyApplyFailed.Code(), "apply app-service ingress cilium policy", err)
	}
	return nil
}

func normalizeAppServiceIngressPolicyParams(params AppServiceIngressPolicyParams) (AppServiceIngressPolicyParams, error) {
	params.VClusterNamespace = strings.TrimSpace(params.VClusterNamespace)
	params.TenantID = strings.TrimSpace(params.TenantID)
	params.NetworkID = strings.TrimSpace(params.NetworkID)
	params.SubnetID = strings.TrimSpace(params.SubnetID)
	params.AppServiceID = strings.TrimSpace(params.AppServiceID)
	if params.VClusterNamespace == "" || params.TenantID == "" || params.NetworkID == "" || params.SubnetID == "" || params.AppServiceID == "" {
		return AppServiceIngressPolicyParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "namespace, tenantID, networkID, subnetID, and appServiceID are required", cferrors.ErrInvalidInput)
	}
	if errs := validation.IsDNS1123Label(params.VClusterNamespace); len(errs) > 0 {
		return AppServiceIngressPolicyParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "namespace must be a DNS-1123 label: "+strings.Join(errs, "; "), cferrors.ErrInvalidInput)
	}
	for field, value := range map[string]string{
		"tenantID":     params.TenantID,
		"networkID":    params.NetworkID,
		"subnetID":     params.SubnetID,
		"appServiceID": params.AppServiceID,
	} {
		if errs := validation.IsValidLabelValue(value); len(errs) > 0 {
			return AppServiceIngressPolicyParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, field+" must be a valid Kubernetes label value: "+strings.Join(errs, "; "), cferrors.ErrInvalidInput)
		}
	}
	if params.PublicEndpointPort <= 0 || params.PublicEndpointPort > 65535 {
		return AppServiceIngressPolicyParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "PublicEndpointPort must be between 1 and 65535", cferrors.ErrInvalidInput)
	}
	return params, nil
}

func (c *cfCiliumClient) upsertPolicy(ctx context.Context, desired *unstructured.Unstructured) error {
	current, err := c.dyn.Resource(ciliumGVR).Namespace(desired.GetNamespace()).Get(ctx, desired.GetName(), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.dyn.Resource(ciliumGVR).Namespace(desired.GetNamespace()).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	desired.SetResourceVersion(current.GetResourceVersion())
	_, err = c.dyn.Resource(ciliumGVR).Namespace(desired.GetNamespace()).Update(ctx, desired, metav1.UpdateOptions{})
	return err
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

func buildAppServiceIngressPolicy(params AppServiceIngressPolicyParams) *unstructured.Unstructured {
	name := AppServiceIngressPolicyName(params.AppServiceID)
	portStr := strconv.Itoa(params.PublicEndpointPort)
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "cilium.io", Version: "v2", Kind: "CiliumNetworkPolicy"})
	u.SetName(name)
	u.SetNamespace(params.VClusterNamespace)

	// Cilium endpointSelector must target the exact public app-service pod set.
	// Broad namespace ingress would expose private-subnet workloads that happen
	// to share the tenant vCluster, breaking the CF App Service exposure model.
	_ = unstructured.SetNestedMap(u.Object, map[string]interface{}{
		"matchLabels": map[string]interface{}{
			labelTenantID:     params.TenantID,
			labelNetworkID:    params.NetworkID,
			labelSubnetID:     params.SubnetID,
			labelAppServiceID: params.AppServiceID,
			labelVisibility:   visibilityPublic,
		},
	}, "spec", "endpointSelector")
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

func (c *cfCiliumClient) RemoveAppServiceIngressPolicy(ctx context.Context, namespace, appServiceID string) error {
	namespace = strings.TrimSpace(namespace)
	appServiceID = strings.TrimSpace(appServiceID)
	if namespace == "" || appServiceID == "" {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "namespace and appServiceID are required", cferrors.ErrInvalidInput)
	}
	return c.RemovePolicy(ctx, namespace, AppServiceIngressPolicyName(appServiceID))
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

func nameWithPrefix(prefix, raw string) string {
	token := sanitizeDNSLabelToken(raw)
	if token == "" {
		token = "app"
	}
	if len(prefix)+len(token) <= validation.DNS1123LabelMaxLength {
		return prefix + token
	}
	hash := shortHash(token)
	keep := validation.DNS1123LabelMaxLength - len(prefix) - len(hash) - 1
	if keep < 1 {
		return strings.TrimRight(prefix[:validation.DNS1123LabelMaxLength], "-")
	}
	short := strings.TrimRight(token[:keep], "-")
	if short == "" {
		short = "app"
	}
	return prefix + short + "-" + hash
}

func sanitizeDNSLabelToken(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	lastHyphen := false
	for _, r := range raw {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if r == '-' || unicode.IsSpace(r) || r == '_' || r == '.' || r == '/' {
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func shortHash(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%08x", h.Sum32())
}
