package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	cidrrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/cidr"
	jobsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/jobs"
	vclusterrepo "github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/vcluster"
	"github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/service/mocks"
)

const testNetworkID = "aaaaaaaa-bbbb-4ccc-dddd-eeeeeeeeeeee"

func TestProvisionNetwork_ReturnsPendingJob(t *testing.T) {
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockCIDRRepository(ctrl)
	mj := mocks.NewMockJobsRepository(ctrl)
	mv := mocks.NewMockVClusterClient(ctrl)
	mcl := mocks.NewMockCiliumClient(ctrl)
	mg := mocks.NewMockGatewayClient(ctrl)
	mk := mocks.NewMockKubeconfigRepository(ctrl)

	mc.EXPECT().
		Allocate(gomock.Any(), cidrrepo.AllocateParams{
			NetworkID:    testNetworkID,
			RequestedPod: "10.1.0.0/16",
			RequestedSvc: "10.2.0.0/16",
		}).
		Return(cidrrepo.CIDRAllocation{
			NetworkID: testNetworkID,
			PodCIDR:   "10.1.0.0/16",
			SvcCIDR:   "10.2.0.0/16",
		}, nil)

	mj.EXPECT().
		Create(gomock.Any(), jobsrepo.CreateJobParams{
			NetworkID: testNetworkID,
			Type:      jobsrepo.JobTypeProvisionNetwork,
		}).
		Return(jobsrepo.Job{
			ID:        "job-provision-1",
			NetworkID: testNetworkID,
			Type:      jobsrepo.JobTypeProvisionNetwork,
			Status:    jobsrepo.JobStatusPending,
		}, nil)

	vc := vclusterName(testNetworkID)
	var wg sync.WaitGroup
	wg.Add(1)
	mj.EXPECT().UpdateStatus(gomock.Any(), "job-provision-1", jobsrepo.JobStatusRunning, "").Return(nil).Times(1)
	mv.EXPECT().Create(gomock.Any(), vclusterrepo.CreateVClusterParams{
		Name:      vc,
		Namespace: vc,
		PodCIDR:   "10.1.0.0/16",
		SvcCIDR:   "10.2.0.0/16",
		Region:    "us-east-1",
	}).Return(vclusterrepo.VClusterInfo{Name: vc, Namespace: vc, Status: vclusterrepo.VClusterStatusRunning}, nil).Times(1)
	mv.EXPECT().Get(gomock.Any(), vc).Return(vclusterrepo.VClusterInfo{
		Name:      vc,
		Namespace: vc,
		Status:    vclusterrepo.VClusterStatusRunning,
	}, nil).AnyTimes()
	mcl.EXPECT().ApplyDefaultDenyPolicy(gomock.Any(), vc, testNetworkID).Return(nil).Times(1)
	mv.EXPECT().GetKubeconfig(gomock.Any(), vc).Return([]byte("kubeconfig"), nil).Times(1)
	mk.EXPECT().Store(gomock.Any(), "tenant-1", []byte("kubeconfig")).Return(nil).Times(1)
	mj.EXPECT().UpdateStatus(gomock.Any(), "job-provision-1", jobsrepo.JobStatusSucceeded, "").
		DoAndReturn(func(context.Context, string, jobsrepo.JobStatus, string) error {
			wg.Done()
			return nil
		}).Times(1)

	svc := New(Deps{
		CIDR:       mc,
		Jobs:       mj,
		VCluster:   mv,
		Cilium:     mcl,
		Gateway:    mg,
		Kubeconfig: mk,
	})

	job, err := svc.ProvisionNetwork(context.Background(), ProvisionNetworkParams{
		NetworkID:   testNetworkID,
		TenantID:    "tenant-1",
		Region:      "us-east-1",
		PodCIDRHint: "10.1.0.0/16",
		SvcCIDRHint: "10.2.0.0/16",
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != string(jobsrepo.JobStatusPending) {
		t.Fatalf("status: want pending, got %q", job.Status)
	}
	if job.ID != "job-provision-1" {
		t.Fatalf("job id: got %q", job.ID)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-waitCtx.Done():
		t.Fatal("timeout waiting for provision_network goroutine")
	}
}

func TestProvisionNetwork_CIDRExhausted(t *testing.T) {
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockCIDRRepository(ctrl)
	mj := mocks.NewMockJobsRepository(ctrl)
	mv := mocks.NewMockVClusterClient(ctrl)
	mcl := mocks.NewMockCiliumClient(ctrl)
	mg := mocks.NewMockGatewayClient(ctrl)
	mk := mocks.NewMockKubeconfigRepository(ctrl)

	mc.EXPECT().
		Allocate(gomock.Any(), gomock.Any()).
		Return(cidrrepo.CIDRAllocation{}, cidrrepo.ErrCIDRExhausted)

	svc := New(Deps{CIDR: mc, Jobs: mj, VCluster: mv, Cilium: mcl, Gateway: mg, Kubeconfig: mk})
	_, err := svc.ProvisionNetwork(context.Background(), ProvisionNetworkParams{
		NetworkID: testNetworkID,
		TenantID:  "tenant-1",
		Region:    "us-east-1",
	})
	if err == nil || !errors.Is(err, cidrrepo.ErrCIDRExhausted) {
		t.Fatalf("expected ErrCIDRExhausted, got %v", err)
	}
}

func TestProvisionNetwork_AsyncVClusterCreateFailsUpdatesJob(t *testing.T) {
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockCIDRRepository(ctrl)
	mj := mocks.NewMockJobsRepository(ctrl)
	mv := mocks.NewMockVClusterClient(ctrl)
	mcl := mocks.NewMockCiliumClient(ctrl)
	mg := mocks.NewMockGatewayClient(ctrl)
	mk := mocks.NewMockKubeconfigRepository(ctrl)

	mc.EXPECT().Allocate(gomock.Any(), gomock.Any()).Return(cidrrepo.CIDRAllocation{
		NetworkID: testNetworkID,
		PodCIDR:   "10.1.0.0/16",
		SvcCIDR:   "10.2.0.0/16",
	}, nil)

	mj.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(jobsrepo.Job{
			ID:        "job-async-fail",
			NetworkID: testNetworkID,
			Type:      jobsrepo.JobTypeProvisionNetwork,
			Status:    jobsrepo.JobStatusPending,
		}, nil)

	var wg sync.WaitGroup
	wg.Add(1)

	gomock.InOrder(
		mj.EXPECT().UpdateStatus(gomock.Any(), "job-async-fail", jobsrepo.JobStatusRunning, "").Return(nil),
		mv.EXPECT().Create(gomock.Any(), gomock.Any()).Return(vclusterrepo.VClusterInfo{}, errors.New("vcluster create failed")),
		mj.EXPECT().UpdateStatus(gomock.Any(), "job-async-fail", jobsrepo.JobStatusFailed, gomock.Any()).
			DoAndReturn(func(context.Context, string, jobsrepo.JobStatus, string) error {
				wg.Done()
				return nil
			}),
	)

	svc := New(Deps{
		CIDR:       mc,
		Jobs:       mj,
		VCluster:   mv,
		Cilium:     mcl,
		Gateway:    mg,
		Kubeconfig: mk,
	})
	if _, err := svc.ProvisionNetwork(context.Background(), ProvisionNetworkParams{
		NetworkID: testNetworkID,
		TenantID:  "tenant-1",
		Region:    "us-east-1",
	}); err != nil {
		t.Fatal(err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-waitCtx.Done():
		t.Fatal("timeout waiting for async job failure update")
	}
}

func TestDeprovisionNetwork_RevokeBeforeVClusterDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockCIDRRepository(ctrl)
	mj := mocks.NewMockJobsRepository(ctrl)
	mv := mocks.NewMockVClusterClient(ctrl)
	mcl := mocks.NewMockCiliumClient(ctrl)
	mg := mocks.NewMockGatewayClient(ctrl)
	mk := mocks.NewMockKubeconfigRepository(ctrl)

	mc.EXPECT().Get(gomock.Any(), testNetworkID).Return(cidrrepo.CIDRAllocation{
		NetworkID: testNetworkID,
		PodCIDR:   "10.1.0.0/16",
		SvcCIDR:   "10.2.0.0/16",
	}, nil)

	depJobID := "job-dep-1"
	mj.EXPECT().
		Create(gomock.Any(), jobsrepo.CreateJobParams{
			NetworkID: testNetworkID,
			Type:      jobsrepo.JobTypeDeprovisionNetwork,
		}).
		Return(jobsrepo.Job{
			ID:        depJobID,
			NetworkID: testNetworkID,
			Type:      jobsrepo.JobTypeDeprovisionNetwork,
			Status:    jobsrepo.JobStatusPending,
		}, nil)

	vcName := vclusterName(testNetworkID)
	ns := vcName
	deny := defaultDenyPolicyName(testNetworkID)
	ing := ingressPolicyName(testNetworkID)

	var wg sync.WaitGroup
	wg.Add(1)

	gomock.InOrder(
		mj.EXPECT().UpdateStatus(gomock.Any(), depJobID, jobsrepo.JobStatusRunning, "").Return(nil),
		mk.EXPECT().Revoke(gomock.Any(), "tenant-1").Return(nil),
		mcl.EXPECT().RemovePolicy(gomock.Any(), ns, deny).Return(nil),
		mcl.EXPECT().RemovePolicy(gomock.Any(), ns, ing).Return(nil),
		mv.EXPECT().Delete(gomock.Any(), vcName).Return(nil),
		mc.EXPECT().Release(gomock.Any(), testNetworkID).Return(nil),
		mj.EXPECT().UpdateStatus(gomock.Any(), depJobID, jobsrepo.JobStatusSucceeded, "").
			DoAndReturn(func(context.Context, string, jobsrepo.JobStatus, string) error {
				wg.Done()
				return nil
			}),
	)

	svc := New(Deps{
		CIDR:       mc,
		Jobs:       mj,
		VCluster:   mv,
		Cilium:     mcl,
		Gateway:    mg,
		Kubeconfig: mk,
	}).(*CFProvisionerService)
	svc.rememberTenant(testNetworkID, "tenant-1")

	if _, err := svc.DeprovisionNetwork(context.Background(), testNetworkID); err != nil {
		t.Fatal(err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-waitCtx.Done():
		t.Fatal("timeout waiting for deprovision goroutine")
	}
}
