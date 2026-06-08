package gateway

import (
	"context"
	"os"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

type cfGatewayClient struct {
	cs gatewayclient.Interface
}

func newGatewayClient(hostKubeconfig []byte) (GatewayClient, error) {
	if len(hostKubeconfig) == 0 {
		return nil, cferrors.Wrap(cferrors.CodeInvalidInput, "host kubeconfig is required", cferrors.ErrInvalidInput)
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(hostKubeconfig)
	if err != nil {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "invalid host kubeconfig", cferrors.ErrInternal)
	}
	cs, err := gatewayclient.NewForConfig(cfg)
	if err != nil {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "gateway-api client init failed", cferrors.ErrInternal)
	}
	return &cfGatewayClient{cs: cs}, nil
}

// newCfGatewayClientForTest wires a fake Gateway API clientset (package tests only).
func newCfGatewayClientForTest(cs gatewayclient.Interface) GatewayClient {
	return &cfGatewayClient{cs: cs}
}

func parentGatewayRef() gatewayv1.ParentReference {
	name := os.Getenv("CF_GATEWAY_PARENT_NAME")
	if name == "" {
		name = "envoy-public"
	}
	ns := os.Getenv("CF_GATEWAY_PARENT_NAMESPACE")
	if ns == "" {
		ns = "envoy-gateway-system"
	}
	return gatewayv1.ParentReference{
		Name:      gatewayv1.ObjectName(name),
		Namespace: ptr.To(gatewayv1.Namespace(ns)),
	}
}

func (c *cfGatewayClient) CreateHTTPRoute(ctx context.Context, params HTTPRouteParams) (HTTPRouteInfo, error) {
	if err := validateHTTPRouteParams(params); err != nil {
		return HTTPRouteInfo{}, err
	}
	route := buildHTTPRoute(params)
	_, err := c.cs.GatewayV1().HTTPRoutes(params.Namespace).Create(ctx, route, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return c.GetHTTPRoute(ctx, params.Namespace, params.Name)
		}
		return HTTPRouteInfo{}, cferrors.Wrap(ErrHTTPRouteCreateFailed.Code(), "create httproute", err)
	}
	return c.GetHTTPRoute(ctx, params.Namespace, params.Name)
}

func validateHTTPRouteParams(p HTTPRouteParams) error {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Namespace) == "" {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "name and namespace are required", cferrors.ErrInvalidInput)
	}
	if strings.TrimSpace(p.Hostname) == "" || strings.TrimSpace(p.BackendService) == "" {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "hostname and backendService are required", cferrors.ErrInvalidInput)
	}
	if p.BackendPort <= 0 || p.BackendPort > 65535 {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "backendPort must be between 1 and 65535", cferrors.ErrInvalidInput)
	}
	return nil
}

func buildHTTPRoute(params HTTPRouteParams) *gatewayv1.HTTPRoute {
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Namespace,
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					parentGatewayRef(),
				},
			},
			Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(params.Hostname)},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName(params.BackendService),
									Port: ptr.To(gatewayv1.PortNumber(params.BackendPort)),
								},
							},
						},
					},
				},
			},
		},
	}
	// TLSEnabled is reserved for future TLSRoute / certificateRefs wiring; hostname
	// routing still works when the shared Gateway terminates TLS.
	_ = params.TLSEnabled
	return route
}

func (c *cfGatewayClient) GetHTTPRoute(ctx context.Context, namespace, name string) (HTTPRouteInfo, error) {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(name) == "" {
		return HTTPRouteInfo{}, cferrors.Wrap(cferrors.CodeInvalidInput, "namespace and name are required", cferrors.ErrInvalidInput)
	}
	route, err := c.cs.GatewayV1().HTTPRoutes(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return HTTPRouteInfo{}, cferrors.Wrapf(ErrHTTPRouteNotFound, "%s/%s", namespace, name)
	}
	if err != nil {
		return HTTPRouteInfo{}, cferrors.Wrap(cferrors.CodeInternal, "get httproute", err)
	}
	return httpRouteInfoFrom(route), nil
}

func httpRouteInfoFrom(route *gatewayv1.HTTPRoute) HTTPRouteInfo {
	info := HTTPRouteInfo{
		Name:      route.Name,
		Namespace: route.Namespace,
		Status:    HTTPRouteStatusPending,
	}
	if len(route.Spec.Hostnames) > 0 {
		info.Hostname = string(route.Spec.Hostnames[0])
	}
	for _, p := range route.Status.Parents {
		for _, c := range p.Conditions {
			switch c.Type {
			case string(gatewayv1.RouteConditionAccepted):
				if c.Status == metav1.ConditionFalse {
					info.Status = HTTPRouteStatusFailed
					return info
				}
			case string(gatewayv1.RouteConditionResolvedRefs):
				if c.Status == metav1.ConditionTrue {
					info.Status = HTTPRouteStatusReady
				}
			}
		}
	}
	return info
}

func (c *cfGatewayClient) DeleteHTTPRoute(ctx context.Context, namespace, name string) error {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(name) == "" {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "namespace and name are required", cferrors.ErrInvalidInput)
	}
	err := c.cs.GatewayV1().HTTPRoutes(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return cferrors.Wrap(cferrors.CodeInternal, "delete httproute", err)
	}
	return nil
}
