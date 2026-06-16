package workloads

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

type cfWorkloadClient struct {
	kube kubernetes.Interface
}

func newWorkloadClient(tenantKubeconfig []byte) (WorkloadClient, error) {
	if len(tenantKubeconfig) == 0 {
		return nil, cferrors.Wrap(cferrors.CodeInvalidInput, "tenant kubeconfig is required", cferrors.ErrInvalidInput)
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(tenantKubeconfig)
	if err != nil {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "invalid tenant kubeconfig", cferrors.ErrInternal)
	}
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "kubernetes client init failed", cferrors.ErrInternal)
	}
	return &cfWorkloadClient{kube: kube}, nil
}

// newCfWorkloadClientForTest wires a fake tenant Kubernetes API (package tests only).
func newCfWorkloadClientForTest(kube kubernetes.Interface) WorkloadClient {
	return &cfWorkloadClient{kube: kube}
}

func (c *cfWorkloadClient) Apply(ctx context.Context, params ApplyWorkloadParams) (WorkloadInfo, error) {
	normalized, err := normalizeApplyParams(params)
	if err != nil {
		return WorkloadInfo{}, err
	}
	deployment := buildDeployment(normalized)
	if err := c.upsertDeployment(ctx, deployment); err != nil {
		return WorkloadInfo{}, cferrors.Wrap(ErrWorkloadApplyFailed.Code(), "apply workload deployment", err)
	}

	if len(normalized.Runtime.Ports) > 0 {
		service := buildService(normalized)
		if err := c.upsertService(ctx, service); err != nil {
			return WorkloadInfo{}, cferrors.Wrap(ErrWorkloadApplyFailed.Code(), "apply workload service", err)
		}
	} else if err := c.deleteServiceIfExists(ctx, normalized.Namespace, normalized.Name); err != nil {
		return WorkloadInfo{}, cferrors.Wrap(ErrWorkloadApplyFailed.Code(), "remove stale workload service", err)
	}

	return c.Get(ctx, normalized.Namespace, normalized.Name)
}

func (c *cfWorkloadClient) Get(ctx context.Context, namespace, name string) (WorkloadInfo, error) {
	namespace, name, err := normalizeObjectRef(namespace, name)
	if err != nil {
		return WorkloadInfo{}, err
	}
	deployment, err := c.kube.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return WorkloadInfo{}, cferrors.Wrapf(ErrWorkloadNotFound, "%s/%s", namespace, name)
	}
	if err != nil {
		return WorkloadInfo{}, cferrors.Wrap(cferrors.CodeInternal, "get workload deployment", err)
	}
	info := infoFromDeployment(deployment)
	service, err := c.kube.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		info.ServiceName = service.Name
		return info, nil
	}
	if apierrors.IsNotFound(err) {
		return info, nil
	}
	return WorkloadInfo{}, cferrors.Wrap(cferrors.CodeInternal, "get workload service", err)
}

func (c *cfWorkloadClient) Delete(ctx context.Context, namespace, name string) error {
	namespace, name, err := normalizeObjectRef(namespace, name)
	if err != nil {
		return err
	}
	if err := c.deleteServiceIfExists(ctx, namespace, name); err != nil {
		return cferrors.Wrap(ErrWorkloadDeleteFailed.Code(), "delete workload service", err)
	}
	err = c.kube.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return cferrors.Wrap(ErrWorkloadDeleteFailed.Code(), "delete workload deployment", err)
	}
	return nil
}

func (c *cfWorkloadClient) upsertDeployment(ctx context.Context, desired *appsv1.Deployment) error {
	current, err := c.kube.AppsV1().Deployments(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.kube.AppsV1().Deployments(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	desired.ResourceVersion = current.ResourceVersion
	_, err = c.kube.AppsV1().Deployments(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (c *cfWorkloadClient) upsertService(ctx context.Context, desired *corev1.Service) error {
	current, err := c.kube.CoreV1().Services(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.kube.CoreV1().Services(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	desired.ResourceVersion = current.ResourceVersion

	// ClusterIP allocation fields are assigned by the API server and immutable
	// after creation. Preserve them during updates so changing ports/env/labels
	// does not accidentally ask Kubernetes to replace the service identity.
	desired.Spec.ClusterIP = current.Spec.ClusterIP
	desired.Spec.ClusterIPs = append([]string(nil), current.Spec.ClusterIPs...)
	desired.Spec.IPFamilies = append([]corev1.IPFamily(nil), current.Spec.IPFamilies...)
	desired.Spec.IPFamilyPolicy = current.Spec.IPFamilyPolicy
	desired.Spec.InternalTrafficPolicy = current.Spec.InternalTrafficPolicy

	_, err = c.kube.CoreV1().Services(desired.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func (c *cfWorkloadClient) deleteServiceIfExists(ctx context.Context, namespace, name string) error {
	err := c.kube.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func buildDeployment(params ApplyWorkloadParams) *appsv1.Deployment {
	labels := workloadLabels(params)
	selector := selectorLabels(labels)
	replicas := params.Runtime.Replicas
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Namespace,
			Labels:    copyStringMap(labels),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: copyStringMap(labels),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{buildContainer(params.Runtime)},
				},
			},
		},
	}
}

func buildContainer(runtime WorkloadRuntime) corev1.Container {
	containerPorts := make([]corev1.ContainerPort, 0, len(runtime.Ports))
	for _, port := range runtime.Ports {
		// HTTP, gRPC, and app-level TCP are all transported through Kubernetes
		// TCP Service ports. Protocol-specific public routing is handled later
		// by Gateway API repositories using durable app-service metadata.
		containerPorts = append(containerPorts, corev1.ContainerPort{
			Name:          port.Name,
			ContainerPort: port.ContainerPort,
			Protocol:      corev1.ProtocolTCP,
		})
	}
	env := make([]corev1.EnvVar, 0, len(runtime.Env))
	for _, item := range runtime.Env {
		env = append(env, corev1.EnvVar{Name: item.Name, Value: item.Value})
	}
	cpu := resource.MustParse(runtime.Resources.CPU)
	memory := resource.MustParse(runtime.Resources.Memory)
	quantity := corev1.ResourceList{
		corev1.ResourceCPU:    cpu,
		corev1.ResourceMemory: memory,
	}
	return corev1.Container{
		Name:    "app",
		Image:   runtime.Image,
		Command: append([]string(nil), runtime.Command...),
		Args:    append([]string(nil), runtime.Args...),
		Env:     env,
		Ports:   containerPorts,
		Resources: corev1.ResourceRequirements{
			Requests: quantity.DeepCopy(),
			Limits:   quantity.DeepCopy(),
		},
	}
}

func buildService(params ApplyWorkloadParams) *corev1.Service {
	labels := workloadLabels(params)
	ports := make([]corev1.ServicePort, 0, len(params.Runtime.Ports))
	for _, port := range params.Runtime.Ports {
		ports = append(ports, corev1.ServicePort{
			Name:       port.Name,
			Protocol:   corev1.ProtocolTCP,
			Port:       port.ContainerPort,
			TargetPort: intstr.FromInt(int(port.ContainerPort)),
		})
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Namespace,
			Labels:    copyStringMap(labels),
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: selectorLabels(labels),
			Ports:    ports,
		},
	}
}

func infoFromDeployment(deployment *appsv1.Deployment) WorkloadInfo {
	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
	}
	conditions := make([]WorkloadCondition, 0, len(deployment.Status.Conditions))
	available := false
	replicaFailure := false
	progressing := false
	for _, condition := range deployment.Status.Conditions {
		conditions = append(conditions, WorkloadCondition{
			Type:    string(condition.Type),
			Status:  string(condition.Status),
			Reason:  condition.Reason,
			Message: condition.Message,
		})
		switch condition.Type {
		case appsv1.DeploymentAvailable:
			available = condition.Status == corev1.ConditionTrue
		case appsv1.DeploymentReplicaFailure:
			replicaFailure = condition.Status == corev1.ConditionTrue
		case appsv1.DeploymentProgressing:
			progressing = condition.Status == corev1.ConditionTrue
		}
	}
	ready := available && deployment.Status.ReadyReplicas >= desiredReplicas
	status := WorkloadStatusPending
	switch {
	case deployment.DeletionTimestamp != nil:
		status = WorkloadStatusDeleting
	case replicaFailure:
		status = WorkloadStatusFailed
	case ready:
		status = WorkloadStatusReady
	case progressing || deployment.Status.ReadyReplicas > 0:
		status = WorkloadStatusProgressing
	}
	return WorkloadInfo{
		Namespace:         deployment.Namespace,
		Name:              deployment.Name,
		DeploymentName:    deployment.Name,
		Labels:            copyStringMap(deployment.Labels),
		Status:            status,
		Ready:             ready,
		DesiredReplicas:   desiredReplicas,
		ReadyReplicas:     deployment.Status.ReadyReplicas,
		AvailableReplicas: deployment.Status.AvailableReplicas,
		Conditions:        conditions,
	}
}

func normalizeApplyParams(params ApplyWorkloadParams) (ApplyWorkloadParams, error) {
	var err error
	params.Namespace, params.Name, err = normalizeObjectRef(params.Namespace, params.Name)
	if err != nil {
		return ApplyWorkloadParams{}, err
	}
	params.TenantID = strings.TrimSpace(params.TenantID)
	params.NetworkID = strings.TrimSpace(params.NetworkID)
	params.SubnetID = strings.TrimSpace(params.SubnetID)
	params.AppServiceID = strings.TrimSpace(params.AppServiceID)
	params.Visibility = WorkloadVisibility(strings.ToLower(strings.TrimSpace(string(params.Visibility))))
	params.Runtime.ServiceType = strings.TrimSpace(params.Runtime.ServiceType)
	params.Runtime.Image = strings.TrimSpace(params.Runtime.Image)
	params.Runtime.Resources.CPU = strings.TrimSpace(params.Runtime.Resources.CPU)
	params.Runtime.Resources.Memory = strings.TrimSpace(params.Runtime.Resources.Memory)
	if params.Runtime.Replicas == 0 {
		params.Runtime.Replicas = 1
	}
	if params.TenantID == "" {
		return ApplyWorkloadParams{}, invalidWorkload("tenantID is required")
	}
	if params.NetworkID == "" {
		return ApplyWorkloadParams{}, invalidWorkload("networkID is required")
	}
	if params.SubnetID == "" {
		return ApplyWorkloadParams{}, invalidWorkload("subnetID is required")
	}
	if params.AppServiceID == "" {
		return ApplyWorkloadParams{}, invalidWorkload("appServiceID is required")
	}
	if params.Visibility != WorkloadVisibilityPrivate && params.Visibility != WorkloadVisibilityPublic {
		return ApplyWorkloadParams{}, invalidWorkload("visibility must be private or public")
	}
	if params.Runtime.Image == "" {
		return ApplyWorkloadParams{}, invalidWorkload("runtime.image is required")
	}
	if params.Runtime.Resources.CPU == "" {
		return ApplyWorkloadParams{}, invalidWorkload("runtime.resources.cpu is required")
	}
	if _, err := resource.ParseQuantity(params.Runtime.Resources.CPU); err != nil {
		return ApplyWorkloadParams{}, invalidWorkload("runtime.resources.cpu is invalid")
	}
	if params.Runtime.Resources.Memory == "" {
		return ApplyWorkloadParams{}, invalidWorkload("runtime.resources.memory is required")
	}
	if _, err := resource.ParseQuantity(params.Runtime.Resources.Memory); err != nil {
		return ApplyWorkloadParams{}, invalidWorkload("runtime.resources.memory is invalid")
	}
	if params.Runtime.Replicas < 1 {
		return ApplyWorkloadParams{}, invalidWorkload("runtime.replicas must be at least 1")
	}
	for i := range params.Runtime.Env {
		params.Runtime.Env[i].Name = strings.TrimSpace(params.Runtime.Env[i].Name)
		if errs := validation.IsEnvVarName(params.Runtime.Env[i].Name); len(errs) > 0 {
			return ApplyWorkloadParams{}, invalidWorkload(fmt.Sprintf("runtime.env[%d].name is invalid: %s", i, strings.Join(errs, "; ")))
		}
	}
	for i := range params.Runtime.Ports {
		params.Runtime.Ports[i].Name = strings.TrimSpace(params.Runtime.Ports[i].Name)
		params.Runtime.Ports[i].Protocol = WorkloadPortProtocol(strings.ToUpper(strings.TrimSpace(string(params.Runtime.Ports[i].Protocol))))
		if errs := validation.IsDNS1123Label(params.Runtime.Ports[i].Name); len(errs) > 0 {
			return ApplyWorkloadParams{}, invalidWorkload(fmt.Sprintf("runtime.ports[%d].name must be a DNS-1123 label: %s", i, strings.Join(errs, "; ")))
		}
		if params.Runtime.Ports[i].ContainerPort < 1 || params.Runtime.Ports[i].ContainerPort > 65535 {
			return ApplyWorkloadParams{}, invalidWorkload(fmt.Sprintf("runtime.ports[%d].containerPort must be between 1 and 65535", i))
		}
		switch params.Runtime.Ports[i].Protocol {
		case WorkloadPortProtocolHTTP, WorkloadPortProtocolGRPC, WorkloadPortProtocolTCP:
		default:
			return ApplyWorkloadParams{}, invalidWorkload(fmt.Sprintf("runtime.ports[%d].protocol must be HTTP, GRPC, or TCP", i))
		}
	}
	if err := validateLabelValues(params); err != nil {
		return ApplyWorkloadParams{}, err
	}
	return params, nil
}

func normalizeObjectRef(namespace, name string) (string, string, error) {
	namespace = strings.TrimSpace(namespace)
	name = strings.TrimSpace(name)
	if namespace == "" || name == "" {
		return "", "", invalidWorkload("namespace and name are required")
	}
	if errs := validation.IsDNS1123Label(namespace); len(errs) > 0 {
		return "", "", invalidWorkload("namespace must be a DNS-1123 label: " + strings.Join(errs, "; "))
	}
	if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
		return "", "", invalidWorkload("name must be a DNS-1123 label: " + strings.Join(errs, "; "))
	}
	return namespace, name, nil
}

func validateLabelValues(params ApplyWorkloadParams) error {
	values := []struct {
		field string
		value string
	}{
		{"tenantID", params.TenantID},
		{"networkID", params.NetworkID},
		{"subnetID", params.SubnetID},
		{"appServiceID", params.AppServiceID},
		{"visibility", string(params.Visibility)},
	}
	for _, item := range values {
		if errs := validation.IsValidLabelValue(item.value); len(errs) > 0 {
			return invalidWorkload(item.field + " must be a valid Kubernetes label value: " + strings.Join(errs, "; "))
		}
	}
	return nil
}

func workloadLabels(params ApplyWorkloadParams) map[string]string {
	return map[string]string{
		LabelTenantID:     params.TenantID,
		LabelNetworkID:    params.NetworkID,
		LabelSubnetID:     params.SubnetID,
		LabelAppServiceID: params.AppServiceID,
		LabelVisibility:   string(params.Visibility),
	}
}

func selectorLabels(labels map[string]string) map[string]string {
	// Deployment selectors are immutable after creation. Use the app-service ID
	// as the narrow stable owner key, while object/pod labels still carry richer
	// placement metadata for policy and observability.
	return map[string]string{
		LabelAppServiceID: labels[LabelAppServiceID],
	}
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
