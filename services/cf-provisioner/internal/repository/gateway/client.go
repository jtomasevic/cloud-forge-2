package gateway

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"unicode"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

type cfGatewayClient struct {
	cs gatewayclient.Interface
}

const (
	appServiceHTTPRoutePrefix = "appsvc-"
	labelAppServiceID         = "cloudforge.io/app-service-id"
	labelRouteKind            = "cloudforge.io/route-kind"
	labelRouteKindAppService  = "app-service"
)

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
	normalized, err := normalizeHTTPRouteParams(params)
	if err != nil {
		return HTTPRouteInfo{}, err
	}
	route := buildHTTPRoute(normalized)
	err = c.upsertHTTPRoute(ctx, route)
	if err != nil {
		return HTTPRouteInfo{}, cferrors.Wrap(ErrHTTPRouteCreateFailed.Code(), "create httproute", err)
	}
	return c.GetHTTPRoute(ctx, normalized.Namespace, normalized.Name)
}

func (c *cfGatewayClient) CreateAppServiceHTTPRoute(ctx context.Context, params AppServiceHTTPRouteParams) (HTTPRouteInfo, error) {
	normalized, err := normalizeAppServiceHTTPRouteParams(params)
	if err != nil {
		return HTTPRouteInfo{}, err
	}
	// App-service routes use app service ID in the object name instead of network ID because
	// one public network can expose multiple services under different hostnames. This keeps
	// retries and cleanup scoped to the exact app service exposure.
	return c.CreateHTTPRoute(ctx, httpRouteParamsForAppService(normalized))
}

func (c *cfGatewayClient) upsertHTTPRoute(ctx context.Context, desired *gatewayv1.HTTPRoute) error {
	current, err := c.cs.GatewayV1().HTTPRoutes(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.cs.GatewayV1().HTTPRoutes(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	desired.ResourceVersion = current.ResourceVersion
	_, err = c.cs.GatewayV1().HTTPRoutes(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func normalizeHTTPRouteParams(p HTTPRouteParams) (HTTPRouteParams, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.Namespace = strings.TrimSpace(p.Namespace)
	p.Hostname = strings.TrimSpace(p.Hostname)
	p.BackendService = strings.TrimSpace(p.BackendService)
	p.BackendNamespace = strings.TrimSpace(p.BackendNamespace)
	if p.Name == "" || p.Namespace == "" {
		return HTTPRouteParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "name and namespace are required", cferrors.ErrInvalidInput)
	}
	if errs := validation.IsDNS1123Label(p.Name); len(errs) > 0 {
		return HTTPRouteParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "name must be a DNS-1123 label: "+strings.Join(errs, "; "), cferrors.ErrInvalidInput)
	}
	if errs := validation.IsDNS1123Label(p.Namespace); len(errs) > 0 {
		return HTTPRouteParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "namespace must be a DNS-1123 label: "+strings.Join(errs, "; "), cferrors.ErrInvalidInput)
	}
	if p.Hostname == "" || p.BackendService == "" {
		return HTTPRouteParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "hostname and backendService are required", cferrors.ErrInvalidInput)
	}
	if errs := validation.IsDNS1123Label(p.BackendService); len(errs) > 0 {
		return HTTPRouteParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "backendService must be a DNS-1123 label: "+strings.Join(errs, "; "), cferrors.ErrInvalidInput)
	}
	if p.BackendNamespace != "" {
		if errs := validation.IsDNS1123Label(p.BackendNamespace); len(errs) > 0 {
			return HTTPRouteParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "backendNamespace must be a DNS-1123 label: "+strings.Join(errs, "; "), cferrors.ErrInvalidInput)
		}
	}
	if p.BackendPort <= 0 || p.BackendPort > 65535 {
		return HTTPRouteParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "backendPort must be between 1 and 65535", cferrors.ErrInvalidInput)
	}
	for i := range p.Rules {
		rule, err := normalizeHTTPRouteRule(p, p.Rules[i], i)
		if err != nil {
			return HTTPRouteParams{}, err
		}
		p.Rules[i] = rule
	}
	return p, nil
}

func normalizeHTTPRouteRule(parent HTTPRouteParams, rule HTTPRouteRuleParams, index int) (HTTPRouteRuleParams, error) {
	rule.Path = strings.TrimSpace(rule.Path)
	if rule.PathType == "" {
		rule.PathType = HTTPRoutePathMatchPrefix
	}
	if rule.Path != "" && !strings.HasPrefix(rule.Path, "/") {
		return HTTPRouteRuleParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, fmt.Sprintf("rules[%d].path must start with /", index), cferrors.ErrInvalidInput)
	}
	switch rule.PathType {
	case HTTPRoutePathMatchExact, HTTPRoutePathMatchPrefix:
	default:
		return HTTPRouteRuleParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, fmt.Sprintf("rules[%d].pathType must be Exact or PathPrefix", index), cferrors.ErrInvalidInput)
	}
	rule.BackendService = strings.TrimSpace(rule.BackendService)
	rule.BackendNamespace = strings.TrimSpace(rule.BackendNamespace)
	if rule.BackendService == "" {
		rule.BackendService = parent.BackendService
	}
	if rule.BackendNamespace == "" {
		rule.BackendNamespace = parent.BackendNamespace
	}
	if rule.BackendPort == 0 {
		rule.BackendPort = parent.BackendPort
	}
	if errs := validation.IsDNS1123Label(rule.BackendService); len(errs) > 0 {
		return HTTPRouteRuleParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, fmt.Sprintf("rules[%d].backendService must be a DNS-1123 label: %s", index, strings.Join(errs, "; ")), cferrors.ErrInvalidInput)
	}
	if rule.BackendNamespace != "" {
		if errs := validation.IsDNS1123Label(rule.BackendNamespace); len(errs) > 0 {
			return HTTPRouteRuleParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, fmt.Sprintf("rules[%d].backendNamespace must be a DNS-1123 label: %s", index, strings.Join(errs, "; ")), cferrors.ErrInvalidInput)
		}
	}
	if rule.BackendPort <= 0 || rule.BackendPort > 65535 {
		return HTTPRouteRuleParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, fmt.Sprintf("rules[%d].backendPort must be between 1 and 65535", index), cferrors.ErrInvalidInput)
	}
	return rule, nil
}

func normalizeAppServiceHTTPRouteParams(p AppServiceHTTPRouteParams) (AppServiceHTTPRouteParams, error) {
	p.Namespace = strings.TrimSpace(p.Namespace)
	p.AppServiceID = strings.TrimSpace(p.AppServiceID)
	p.Hostname = strings.TrimSpace(p.Hostname)
	p.BackendService = strings.TrimSpace(p.BackendService)
	p.BackendNamespace = strings.TrimSpace(p.BackendNamespace)
	p.ServicePath = defaultHTTPPath(p.ServicePath, "/")
	p.SwaggerPath = defaultHTTPPath(p.SwaggerPath, "/swagger/")
	p.OpenAPIPath = defaultHTTPPath(p.OpenAPIPath, "/openapi.json")
	if p.Namespace == "" || p.AppServiceID == "" {
		return AppServiceHTTPRouteParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "namespace and appServiceID are required", cferrors.ErrInvalidInput)
	}
	if p.Hostname == "" || p.BackendService == "" {
		return AppServiceHTTPRouteParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "hostname and backendService are required", cferrors.ErrInvalidInput)
	}
	if p.BackendPort <= 0 || p.BackendPort > 65535 {
		return AppServiceHTTPRouteParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "backendPort must be between 1 and 65535", cferrors.ErrInvalidInput)
	}
	if p.DocsBackend != nil {
		p.DocsBackend.Service = strings.TrimSpace(p.DocsBackend.Service)
		p.DocsBackend.Namespace = strings.TrimSpace(p.DocsBackend.Namespace)
		if p.DocsBackend.Service == "" {
			return AppServiceHTTPRouteParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "docsBackend.service is required", cferrors.ErrInvalidInput)
		}
		if p.DocsBackend.Port <= 0 || p.DocsBackend.Port > 65535 {
			return AppServiceHTTPRouteParams{}, cferrors.Wrap(cferrors.CodeInvalidInput, "docsBackend.port must be between 1 and 65535", cferrors.ErrInvalidInput)
		}
	}
	_, err := normalizeHTTPRouteParams(httpRouteParamsForAppService(p))
	if err != nil {
		return AppServiceHTTPRouteParams{}, err
	}
	return p, nil
}

func buildHTTPRoute(params HTTPRouteParams) *gatewayv1.HTTPRoute {
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Namespace,
			Labels:    copyStringMap(params.Labels),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					parentGatewayRef(),
				},
			},
			Hostnames: []gatewayv1.Hostname{gatewayv1.Hostname(params.Hostname)},
			Rules:     buildHTTPRouteRules(params),
		},
	}
	// TLSEnabled is reserved for future TLSRoute / certificateRefs wiring; hostname
	// routing still works when the shared Gateway terminates TLS.
	_ = params.TLSEnabled
	return route
}

func buildHTTPRouteRules(params HTTPRouteParams) []gatewayv1.HTTPRouteRule {
	rules := params.Rules
	if len(rules) == 0 {
		rules = []HTTPRouteRuleParams{{
			BackendService:   params.BackendService,
			BackendNamespace: params.BackendNamespace,
			BackendPort:      params.BackendPort,
		}}
	}
	out := make([]gatewayv1.HTTPRouteRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, gatewayv1.HTTPRouteRule{
			Matches:     buildHTTPRouteMatches(rule),
			BackendRefs: []gatewayv1.HTTPBackendRef{buildHTTPBackendRef(rule)},
		})
	}
	return out
}

func buildHTTPRouteMatches(rule HTTPRouteRuleParams) []gatewayv1.HTTPRouteMatch {
	if rule.Path == "" {
		return nil
	}
	matchType := gatewayv1.PathMatchPathPrefix
	if rule.PathType == HTTPRoutePathMatchExact {
		matchType = gatewayv1.PathMatchExact
	}
	return []gatewayv1.HTTPRouteMatch{
		{
			Path: &gatewayv1.HTTPPathMatch{
				Type:  ptr.To(matchType),
				Value: ptr.To(rule.Path),
			},
		},
	}
}

func buildHTTPBackendRef(rule HTTPRouteRuleParams) gatewayv1.HTTPBackendRef {
	ref := gatewayv1.BackendObjectReference{
		Name: gatewayv1.ObjectName(rule.BackendService),
		Port: ptr.To(gatewayv1.PortNumber(rule.BackendPort)),
	}
	if rule.BackendNamespace != "" {
		ref.Namespace = ptr.To(gatewayv1.Namespace(rule.BackendNamespace))
	}
	return gatewayv1.HTTPBackendRef{
		BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: ref,
		},
	}
}

func httpRouteParamsForAppService(p AppServiceHTTPRouteParams) HTTPRouteParams {
	docs := HTTPRouteBackend{
		Service:   p.BackendService,
		Namespace: p.BackendNamespace,
		Port:      p.BackendPort,
	}
	if p.DocsBackend != nil {
		docs = *p.DocsBackend
	}
	swaggerExact, swaggerPrefix := swaggerPathMatches(p.SwaggerPath)
	return HTTPRouteParams{
		Name:           AppServiceHTTPRouteName(p.AppServiceID),
		Namespace:      p.Namespace,
		Hostname:       p.Hostname,
		BackendService: p.BackendService,
		BackendPort:    p.BackendPort,
		TLSEnabled:     p.TLSEnabled,
		Labels: map[string]string{
			labelAppServiceID: p.AppServiceID,
			labelRouteKind:    labelRouteKindAppService,
		},
		Rules: []HTTPRouteRuleParams{
			{
				Path:             swaggerExact,
				PathType:         HTTPRoutePathMatchExact,
				BackendService:   docs.Service,
				BackendNamespace: docs.Namespace,
				BackendPort:      docs.Port,
			},
			{
				Path:             swaggerPrefix,
				PathType:         HTTPRoutePathMatchPrefix,
				BackendService:   docs.Service,
				BackendNamespace: docs.Namespace,
				BackendPort:      docs.Port,
			},
			{
				Path:             p.OpenAPIPath,
				PathType:         HTTPRoutePathMatchExact,
				BackendService:   docs.Service,
				BackendNamespace: docs.Namespace,
				BackendPort:      docs.Port,
			},
			{
				Path:             p.ServicePath,
				PathType:         HTTPRoutePathMatchPrefix,
				BackendService:   p.BackendService,
				BackendNamespace: p.BackendNamespace,
				BackendPort:      p.BackendPort,
			},
		},
	}
}

func defaultHTTPPath(path, fallback string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return fallback
	}
	return path
}

func swaggerPathMatches(path string) (string, string) {
	exact := strings.TrimRight(path, "/")
	if exact == "" {
		exact = "/"
	}
	prefix := exact
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return exact, prefix
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

func (c *cfGatewayClient) DeleteAppServiceHTTPRoute(ctx context.Context, namespace, appServiceID string) error {
	namespace = strings.TrimSpace(namespace)
	appServiceID = strings.TrimSpace(appServiceID)
	if namespace == "" || appServiceID == "" {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "namespace and appServiceID are required", cferrors.ErrInvalidInput)
	}
	return c.DeleteHTTPRoute(ctx, namespace, AppServiceHTTPRouteName(appServiceID))
}

func AppServiceHTTPRouteName(appServiceID string) string {
	return nameWithPrefix(appServiceHTTPRoutePrefix, appServiceID)
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

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
