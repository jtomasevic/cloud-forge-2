package vcluster

import (
	"context"
	"os/exec"
	"strings"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// execRunner abstracts CLI invocation for tests. Production uses the vCluster
// binary via [os/exec]; replace with the vCluster operator / Helm API when the
// platform standardises on in-cluster reconciliation.
type execRunner interface {
	CombinedOutput(ctx context.Context, name string, arg ...string) ([]byte, error)
}

type osExecRunner struct{}

func (osExecRunner) CombinedOutput(ctx context.Context, name string, arg ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, arg...)
	return cmd.CombinedOutput()
}

type cfVClusterClient struct {
	kube kubernetes.Interface
	exec execRunner
}

func newVClusterClient(hostKubeconfig []byte, r execRunner) (VClusterClient, error) {
	if len(hostKubeconfig) == 0 {
		return nil, cferrors.Wrap(cferrors.CodeInvalidInput, "host kubeconfig is required", cferrors.ErrInvalidInput)
	}
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(hostKubeconfig)
	if err != nil {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "invalid host kubeconfig", cferrors.ErrInternal)
	}
	kube, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "kubernetes client init failed", cferrors.ErrInternal)
	}
	if r == nil {
		r = osExecRunner{}
	}
	return &cfVClusterClient{kube: kube, exec: r}, nil
}

// newCfVClusterClientForTest wires a fake host API and/or stub exec runner (package tests only).
func newCfVClusterClientForTest(kube kubernetes.Interface, r execRunner) VClusterClient {
	if r == nil {
		r = osExecRunner{}
	}
	return &cfVClusterClient{kube: kube, exec: r}
}

func (c *cfVClusterClient) Create(ctx context.Context, params CreateVClusterParams) (VClusterInfo, error) {
	if err := validateCreateParams(params); err != nil {
		return VClusterInfo{}, err
	}
	args := []string{"create", params.Name, "-n", params.Namespace, "--connect=false"}
	if params.PodCIDR != "" {
		args = append(args, "--pod-cidr="+params.PodCIDR)
	}
	if params.SvcCIDR != "" {
		args = append(args, "--service-cidr="+params.SvcCIDR)
	}
	out, err := c.exec.CombinedOutput(ctx, "vcluster", args...)
	if mapErr := mapVClusterCLIError(out, err); mapErr != nil {
		return VClusterInfo{}, mapErr
	}
	return c.Get(ctx, params.Name)
}

func validateCreateParams(p CreateVClusterParams) error {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Namespace) == "" {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "name and namespace are required", cferrors.ErrInvalidInput)
	}
	return nil
}

func (c *cfVClusterClient) Get(ctx context.Context, name string) (VClusterInfo, error) {
	if strings.TrimSpace(name) == "" {
		return VClusterInfo{}, cferrors.Wrap(cferrors.CodeInvalidInput, "name is required", cferrors.ErrInvalidInput)
	}
	ns, err := c.findVClusterNamespace(ctx, name)
	if err != nil {
		return VClusterInfo{}, err
	}
	info := VClusterInfo{
		Name:      name,
		Namespace: ns,
		Status:    VClusterStatusRunning,
	}
	sts, err := c.kube.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			info.Status = VClusterStatusCreating
			return info, nil
		}
		return VClusterInfo{}, cferrors.Wrap(cferrors.CodeInternal, "read vcluster statefulset", err)
	}
	info.PodCIDR = sts.Annotations["cf.cloudforge.io/pod-cidr"]
	info.SvcCIDR = sts.Annotations["cf.cloudforge.io/svc-cidr"]
	if sts.DeletionTimestamp != nil {
		info.Status = VClusterStatusDeleting
	} else if sts.Status.ReadyReplicas < 1 || sts.Status.Replicas < 1 {
		info.Status = VClusterStatusCreating
	}
	return info, nil
}

func (c *cfVClusterClient) findVClusterNamespace(ctx context.Context, name string) (string, error) {
	list, err := c.kube.AppsV1().StatefulSets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{"app": "vcluster"}.AsSelector().String(),
	})
	if err != nil {
		return "", cferrors.Wrap(cferrors.CodeInternal, "list vcluster workloads", err)
	}
	var hits []string
	for i := range list.Items {
		sts := &list.Items[i]
		if sts.Name == name {
			hits = append(hits, sts.Namespace)
		}
	}
	switch len(hits) {
	case 0:
		return "", cferrors.Wrapf(ErrVClusterNotFound, "vcluster %q", name)
	case 1:
		return hits[0], nil
	default:
		return "", cferrors.Wrap(cferrors.CodeConflict, "multiple vcluster workloads match name", cferrors.ErrConflict)
	}
}

func (c *cfVClusterClient) Delete(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "name is required", cferrors.ErrInvalidInput)
	}
	ns, err := c.findVClusterNamespace(ctx, name)
	if err != nil {
		return err
	}
	args := append([]string{"delete", name, "-n", ns}, "--force")
	out, err := c.exec.CombinedOutput(ctx, "vcluster", args...)
	return mapVClusterCLIError(out, err)
}

func (c *cfVClusterClient) GetKubeconfig(ctx context.Context, name string) ([]byte, error) {
	if strings.TrimSpace(name) == "" {
		return nil, cferrors.Wrap(cferrors.CodeInvalidInput, "name is required", cferrors.ErrInvalidInput)
	}
	ns, err := c.findVClusterNamespace(ctx, name)
	if err != nil {
		return nil, err
	}
	out, err := c.exec.CombinedOutput(ctx, "vcluster", "connect", name, "-n", ns, "--print")
	if err != nil {
		lower := strings.ToLower(string(out))
		if strings.Contains(lower, "not ready") || strings.Contains(lower, "waiting") {
			return nil, cferrors.Wrapf(ErrKubeconfigNotReady, "vcluster %q", name)
		}
		if mapErr := mapVClusterCLIError(out, err); mapErr != nil {
			return nil, mapErr
		}
	}
	return out, nil
}

func mapVClusterCLIError(out []byte, execErr error) error {
	if execErr == nil {
		return nil
	}
	msg := strings.ToLower(string(out))
	if strings.Contains(msg, "already exists") || strings.Contains(msg, "already exist") {
		return cferrors.Wrapf(ErrVClusterExists, "vcluster cli: %s", firstLine(string(out)))
	}
	if strings.Contains(msg, "not found") {
		return cferrors.Wrapf(ErrVClusterNotFound, "vcluster cli: %s", firstLine(string(out)))
	}
	return cferrors.Wrap(cferrors.CodeInternal, "vcluster cli failed", cferrors.ErrInternal)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
