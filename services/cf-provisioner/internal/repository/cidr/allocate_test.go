package cidr

import (
	"errors"
	"testing"
)

func TestNextAutoIndex_EmptyPool(t *testing.T) {
	pod, svc, err := resolveAllocateCID(AllocateParams{NetworkID: "n1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pod != "10.0.0.0/16" || svc != "172.16.0.0/20" {
		t.Fatalf("got %s %s", pod, svc)
	}
}

func TestNextAutoIndex_Sequential(t *testing.T) {
	all := []CIDRAllocation{
		{NetworkID: "a", PodCIDR: "10.0.0.0/16", SvcCIDR: "172.16.0.0/20"},
	}
	pod, svc, err := resolveAllocateCID(AllocateParams{NetworkID: "b"}, all)
	if err != nil {
		t.Fatal(err)
	}
	if pod != "10.1.0.0/16" || svc != "172.16.16.0/20" {
		t.Fatalf("got %s %s", pod, svc)
	}
}

func TestAllocate_ConflictOnRequestedPod(t *testing.T) {
	all := []CIDRAllocation{
		{NetworkID: "a", PodCIDR: "10.5.0.0/16", SvcCIDR: "172.16.80.0/20"},
	}
	_, _, err := resolveAllocateCID(AllocateParams{
		NetworkID:    "b",
		RequestedPod: "10.5.0.0/16",
		RequestedSvc: "172.16.96.0/20",
	}, all)
	if err == nil {
		t.Fatal("expected conflict")
	}
	if !errors.Is(err, ErrCIDRConflict) {
		t.Fatalf("expected ErrCIDRConflict, got %v", err)
	}
}

func TestIndexRoundTrip(t *testing.T) {
	for _, idx := range []int{0, 1, 15, 16, 255} {
		p := indexToPodCIDR(idx)
		s := indexToSvcCIDR(idx)
		ip, ok := parseAutoPodIndex(p)
		if !ok || ip != idx {
			t.Fatalf("pod idx %d: got %d ok=%v", idx, ip, ok)
		}
		is, ok := parseAutoSvcIndex(s)
		if !ok || is != idx {
			t.Fatalf("svc idx %d: got %d ok=%v", idx, is, ok)
		}
	}
}

func TestErrCIDRExhausted(t *testing.T) {
	all := []CIDRAllocation{{NetworkID: "x", PodCIDR: indexToPodCIDR(255), SvcCIDR: indexToSvcCIDR(255)}}
	_, _, err := resolveAllocateCID(AllocateParams{NetworkID: "y"}, all)
	if !errors.Is(err, ErrCIDRExhausted) {
		t.Fatalf("expected exhausted, got %v", err)
	}
}
