package vcluster

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

type stubExec struct {
	out []byte
	err error
}

func (s stubExec) CombinedOutput(ctx context.Context, name string, arg ...string) ([]byte, error) {
	return s.out, s.err
}

func fakeVClusterSTS(name, namespace string) *appsv1.StatefulSet {
	replicas := int32(1)
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": "vcluster"},
			Annotations: map[string]string{
				"cf.cloudforge.io/pod-cidr": "10.1.0.0/16",
				"cf.cloudforge.io/svc-cidr": "10.2.0.0/16",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "vcluster"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "vcluster"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "syncer", Image: "test"}},
				},
			},
			ServiceName: "svc",
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:      1,
			ReadyReplicas: 1,
		},
	}
}

func TestCreate_MapsAlreadyExistsCLI(t *testing.T) {
	kc := fake.NewSimpleClientset(fakeVClusterSTS("vc1", "ns1"))
	c := newCfVClusterClientForTest(kc, stubExec{
		out: []byte("Error: vcluster already exists in namespace ns1"),
		err: errors.New("exit status 1"),
	})
	_, err := c.Create(context.Background(), CreateVClusterParams{Name: "vc1", Namespace: "ns1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrVClusterExists) {
		t.Fatalf("expected ErrVClusterExists, got %v", err)
	}
}

func TestGet_NotFound(t *testing.T) {
	kc := fake.NewSimpleClientset()
	c := newCfVClusterClientForTest(kc, stubExec{})
	_, err := c.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrVClusterNotFound) {
		t.Fatalf("expected ErrVClusterNotFound, got %v", err)
	}
}

func TestGet_Success(t *testing.T) {
	kc := fake.NewSimpleClientset(fakeVClusterSTS("vc1", "ns1"))
	c := newCfVClusterClientForTest(kc, stubExec{})
	info, err := c.Get(context.Background(), "vc1")
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != VClusterStatusRunning || info.Namespace != "ns1" {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info.PodCIDR != "10.1.0.0/16" {
		t.Fatalf("pod CIDR: %q", info.PodCIDR)
	}
}

func TestCreate_InvalidParams(t *testing.T) {
	c := newCfVClusterClientForTest(fake.NewSimpleClientset(), stubExec{})
	_, err := c.Create(context.Background(), CreateVClusterParams{Name: "", Namespace: "ns"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, cferrors.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestNew_EmptyKubeconfig(t *testing.T) {
	_, err := newVClusterClient(nil, stubExec{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, cferrors.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}
