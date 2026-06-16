package workloads

import (
	"context"
	"errors"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

func TestApplyCreatesDeploymentAndService(t *testing.T) {
	ctx := context.Background()
	cs := fake.NewSimpleClientset()
	client := newCfWorkloadClientForTest(cs)

	info, err := client.Apply(ctx, validApplyParams())
	if err != nil {
		t.Fatalf("apply workload: %v", err)
	}
	if info.DeploymentName != "orders-api" || info.ServiceName != "orders-api" {
		t.Fatalf("unexpected info names: %+v", info)
	}
	if info.DesiredReplicas != 2 || info.ReadyReplicas != 0 {
		t.Fatalf("unexpected initial replica status: %+v", info)
	}

	deployment, err := cs.AppsV1().Deployments("tenant-a").Get(ctx, "orders-api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	wantLabels := expectedLabels()
	if !reflect.DeepEqual(deployment.Labels, wantLabels) {
		t.Fatalf("deployment labels:\n got %#v\nwant %#v", deployment.Labels, wantLabels)
	}
	if !reflect.DeepEqual(deployment.Spec.Template.Labels, wantLabels) {
		t.Fatalf("pod template labels:\n got %#v\nwant %#v", deployment.Spec.Template.Labels, wantLabels)
	}
	if got, want := deployment.Spec.Selector.MatchLabels, map[string]string{LabelAppServiceID: testAppServiceID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deployment selector:\n got %#v\nwant %#v", got, want)
	}
	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 2 {
		t.Fatalf("replicas = %v, want 2", deployment.Spec.Replicas)
	}
	container := deployment.Spec.Template.Spec.Containers[0]
	if container.Name != "app" || container.Image != "registry.local/cloudforge/orders-api:v1" {
		t.Fatalf("unexpected container identity: %+v", container)
	}
	if got, want := container.Command, []string{"/bin/orders"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("command got %#v want %#v", got, want)
	}
	if got, want := container.Args, []string{"serve"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("args got %#v want %#v", got, want)
	}
	if got := container.Env; len(got) != 2 || got[0].Name != "LOG_LEVEL" || got[0].Value != "debug" || got[1].Name != "REGION" {
		t.Fatalf("env not preserved: %+v", got)
	}
	if got := container.Ports; len(got) != 1 || got[0].Name != "http" || got[0].ContainerPort != 8080 || got[0].Protocol != corev1.ProtocolTCP {
		t.Fatalf("ports not mapped to container TCP: %+v", got)
	}
	assertQuantity(t, container.Resources.Requests[corev1.ResourceCPU], "500m")
	assertQuantity(t, container.Resources.Limits[corev1.ResourceCPU], "500m")
	assertQuantity(t, container.Resources.Requests[corev1.ResourceMemory], "512Mi")
	assertQuantity(t, container.Resources.Limits[corev1.ResourceMemory], "512Mi")

	service, err := cs.CoreV1().Services("tenant-a").Get(ctx, "orders-api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if service.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("service type = %s, want ClusterIP", service.Spec.Type)
	}
	if !reflect.DeepEqual(service.Labels, wantLabels) {
		t.Fatalf("service labels:\n got %#v\nwant %#v", service.Labels, wantLabels)
	}
	if got, want := service.Spec.Selector, map[string]string{LabelAppServiceID: testAppServiceID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("service selector:\n got %#v\nwant %#v", got, want)
	}
	if got := service.Spec.Ports; len(got) != 1 || got[0].Name != "http" || got[0].Port != 8080 || got[0].TargetPort.IntVal != 8080 {
		t.Fatalf("service ports not mapped: %+v", got)
	}
}

func TestApplyWorkerWithoutPortsDoesNotCreateService(t *testing.T) {
	ctx := context.Background()
	cs := fake.NewSimpleClientset()
	client := newCfWorkloadClientForTest(cs)
	params := validApplyParams()
	params.Runtime.ServiceType = "worker"
	params.Runtime.Ports = nil

	info, err := client.Apply(ctx, params)
	if err != nil {
		t.Fatalf("apply worker: %v", err)
	}
	if info.ServiceName != "" {
		t.Fatalf("worker should not report service name: %+v", info)
	}
	if _, err := cs.CoreV1().Services("tenant-a").Get(ctx, "orders-api", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("service lookup got %v, want not found", err)
	}
	deployment, err := cs.AppsV1().Deployments("tenant-a").Get(ctx, "orders-api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("worker deployment missing: %v", err)
	}
	if len(deployment.Spec.Template.Spec.Containers[0].Ports) != 0 {
		t.Fatalf("worker container should have no ports: %+v", deployment.Spec.Template.Spec.Containers[0].Ports)
	}
}

func TestApplyRemovesServiceWhenPortsAreRemoved(t *testing.T) {
	ctx := context.Background()
	cs := fake.NewSimpleClientset()
	client := newCfWorkloadClientForTest(cs)
	params := validApplyParams()
	if _, err := client.Apply(ctx, params); err != nil {
		t.Fatalf("initial apply: %v", err)
	}

	params.Runtime.Ports = nil
	params.Runtime.ServiceType = "worker"
	if _, err := client.Apply(ctx, params); err != nil {
		t.Fatalf("apply without ports: %v", err)
	}
	if _, err := cs.CoreV1().Services("tenant-a").Get(ctx, "orders-api", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("service lookup got %v, want not found", err)
	}
}

func TestGetReportsDeploymentReadinessFromConditions(t *testing.T) {
	ctx := context.Background()
	cs := fake.NewSimpleClientset()
	client := newCfWorkloadClientForTest(cs)
	if _, err := client.Apply(ctx, validApplyParams()); err != nil {
		t.Fatalf("apply workload: %v", err)
	}
	deployment, err := cs.AppsV1().Deployments("tenant-a").Get(ctx, "orders-api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	deployment.Status.Replicas = 2
	deployment.Status.ReadyReplicas = 2
	deployment.Status.AvailableReplicas = 2
	deployment.Status.Conditions = []appsv1.DeploymentCondition{
		{
			Type:    appsv1.DeploymentAvailable,
			Status:  corev1.ConditionTrue,
			Reason:  "MinimumReplicasAvailable",
			Message: "Deployment has minimum availability.",
		},
		{
			Type:   appsv1.DeploymentProgressing,
			Status: corev1.ConditionTrue,
			Reason: "NewReplicaSetAvailable",
		},
	}
	if _, err := cs.AppsV1().Deployments("tenant-a").UpdateStatus(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update deployment status: %v", err)
	}

	info, err := client.Get(ctx, "tenant-a", "orders-api")
	if err != nil {
		t.Fatalf("get workload: %v", err)
	}
	if !info.Ready || info.Status != WorkloadStatusReady {
		t.Fatalf("readiness not reported from conditions: %+v", info)
	}
	if info.DesiredReplicas != 2 || info.ReadyReplicas != 2 || info.AvailableReplicas != 2 {
		t.Fatalf("replica status mismatch: %+v", info)
	}
	if len(info.Conditions) != 2 || info.Conditions[0].Type != string(appsv1.DeploymentAvailable) {
		t.Fatalf("conditions not copied: %+v", info.Conditions)
	}
}

func TestDeleteIsIdempotent(t *testing.T) {
	ctx := context.Background()
	cs := fake.NewSimpleClientset()
	client := newCfWorkloadClientForTest(cs)
	if _, err := client.Apply(ctx, validApplyParams()); err != nil {
		t.Fatalf("apply workload: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := client.Delete(ctx, "tenant-a", "orders-api"); err != nil {
			t.Fatalf("delete attempt %d: %v", i+1, err)
		}
	}
	if _, err := cs.CoreV1().Services("tenant-a").Get(ctx, "orders-api", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("service lookup got %v, want not found", err)
	}
	if _, err := cs.AppsV1().Deployments("tenant-a").Get(ctx, "orders-api", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("deployment lookup got %v, want not found", err)
	}
}

func TestApplyValidation(t *testing.T) {
	ctx := context.Background()
	cs := fake.NewSimpleClientset()
	client := newCfWorkloadClientForTest(cs)
	params := validApplyParams()
	params.Runtime.Image = ""

	_, err := client.Apply(ctx, params)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, ErrInvalidWorkload) {
		t.Fatalf("expected ErrInvalidWorkload, got %v", err)
	}
	var ce *cferrors.CFError
	if !errors.As(err, &ce) || ce.Code() != cferrors.CodeInvalidInput {
		t.Fatalf("expected invalid input code, got %v", err)
	}
}

func TestGetMissingDeploymentReturnsNotFound(t *testing.T) {
	cs := fake.NewSimpleClientset()
	client := newCfWorkloadClientForTest(cs)

	_, err := client.Get(context.Background(), "tenant-a", "orders-api")
	if err == nil {
		t.Fatal("expected not found")
	}
	if !errors.Is(err, ErrWorkloadNotFound) {
		t.Fatalf("expected ErrWorkloadNotFound, got %v", err)
	}
}

func TestWorkloadLabelsAreDeterministic(t *testing.T) {
	params := validApplyParams()
	if got, want := workloadLabels(params), expectedLabels(); !reflect.DeepEqual(got, want) {
		t.Fatalf("labels:\n got %#v\nwant %#v", got, want)
	}
}

func assertQuantity(t *testing.T, got resource.Quantity, want string) {
	t.Helper()
	expected := resource.MustParse(want)
	if got.Cmp(expected) != 0 {
		t.Fatalf("quantity got %s want %s", got.String(), expected.String())
	}
}

const (
	testTenantID     = "550e8400-e29b-41d4-a716-446655440000"
	testNetworkID    = "550e8400-e29b-41d4-a716-446655440001"
	testSubnetID     = "550e8400-e29b-41d4-a716-446655440002"
	testAppServiceID = "550e8400-e29b-41d4-a716-446655440003"
)

func validApplyParams() ApplyWorkloadParams {
	return ApplyWorkloadParams{
		Namespace:    "tenant-a",
		Name:         "orders-api",
		TenantID:     testTenantID,
		NetworkID:    testNetworkID,
		SubnetID:     testSubnetID,
		AppServiceID: testAppServiceID,
		Visibility:   WorkloadVisibilityPublic,
		Runtime: WorkloadRuntime{
			ServiceType: "rest",
			Image:       "registry.local/cloudforge/orders-api:v1",
			Command:     []string{"/bin/orders"},
			Args:        []string{"serve"},
			Resources: WorkloadResources{
				CPU:    "500m",
				Memory: "512Mi",
			},
			Env: []WorkloadEnvVar{
				{Name: "LOG_LEVEL", Value: "debug"},
				{Name: "REGION", Value: "test"},
			},
			Ports: []WorkloadPort{
				{Name: "http", ContainerPort: 8080, Protocol: WorkloadPortProtocolHTTP},
			},
			Replicas: 2,
		},
	}
}

func expectedLabels() map[string]string {
	return map[string]string{
		LabelTenantID:     testTenantID,
		LabelNetworkID:    testNetworkID,
		LabelSubnetID:     testSubnetID,
		LabelAppServiceID: testAppServiceID,
		LabelVisibility:   "public",
	}
}
