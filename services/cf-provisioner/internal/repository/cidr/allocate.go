package cidr

import (
	"fmt"
	"net"
	"strings"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

// Allocation strategy (v1):
//   - Pod supernet: 10.0.0.0/8 split into canonical /16 blocks 10.{i}.0.0/16 for i in [0,255].
//   - Service supernet: 172.16.0.0/12 split into /20 blocks using indexToSvcCIDR(i) for the same i.
//   - Auto mode picks the lowest integer i > max(i) already present in existing canonical pod CIDRs.
//   - This is sequential (not gap-filling). Concurrent writers may race; callers should serialize
//     high-volume creates or tolerate rare overlaps until a stronger allocator (LWT / slot table) exists.

const maxAutoIndex = 255

var (
	superPod *net.IPNet
	superSvc *net.IPNet
)

func init() {
	var err error
	_, superPod, err = net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		panic(err)
	}
	_, superSvc, err = net.ParseCIDR("172.16.0.0/12")
	if err != nil {
		panic(err)
	}
}

func indexToPodCIDR(i int) string {
	return fmt.Sprintf("10.%d.0.0/16", i)
}

// indexToSvcCIDR maps the same index i used for pod blocks to the i-th /20 inside 172.16.0.0/12.
func indexToSvcCIDR(i int) string {
	if i < 0 || i > maxAutoIndex {
		return ""
	}
	second := 16 + i/16
	third := (i % 16) * 16
	return fmt.Sprintf("172.%d.%d.0/20", second, third)
}

func parseAutoPodIndex(pod string) (int, bool) {
	pod = strings.TrimSpace(pod)
	var i int
	n, err := fmt.Sscanf(pod, "10.%d.0.0/16", &i)
	if err != nil || n != 1 {
		return 0, false
	}
	if i < 0 || i > maxAutoIndex {
		return 0, false
	}
	return i, true
}

func parseAutoSvcIndex(svc string) (int, bool) {
	svc = strings.TrimSpace(svc)
	var second, third int
	n, err := fmt.Sscanf(svc, "172.%d.%d.0/20", &second, &third)
	if err != nil || n != 2 {
		return 0, false
	}
	if second < 16 || second > 31 || third%16 != 0 || third > 240 {
		return 0, false
	}
	i := (second-16)*16 + third/16
	if i < 0 || i > maxAutoIndex {
		return 0, false
	}
	return i, true
}

func maxUsedAutoIndex(allocations []CIDRAllocation) int {
	max := -1
	for _, a := range allocations {
		if i, ok := parseAutoPodIndex(a.PodCIDR); ok {
			if i > max {
				max = i
			}
		}
	}
	return max
}

func nextAutoIndex(allocations []CIDRAllocation) (int, error) {
	next := maxUsedAutoIndex(allocations) + 1
	if next > maxAutoIndex {
		return 0, ErrCIDRExhausted
	}
	return next, nil
}

func mustParseCIDR(s string) (*net.IPNet, error) {
	_, ipnet, err := net.ParseCIDR(strings.TrimSpace(s))
	if err != nil {
		return nil, cferrors.Wrap(cferrors.CodeInvalidInput, "invalid CIDR", cferrors.ErrInvalidInput)
	}
	if ipnet.IP.To4() == nil {
		return nil, cferrors.Wrap(cferrors.CodeInvalidInput, "IPv6 CIDR not supported", cferrors.ErrInvalidInput)
	}
	return ipnet, nil
}

func cidrInSupernet(ipnet *net.IPNet, super *net.IPNet) bool {
	if !super.Contains(ipnet.IP) {
		return false
	}
	// First IP of CIDR must be inside supernet; mask must be at least as specific as super's prefix... simplified:
	last := lastIP4(ipnet)
	return super.Contains(last)
}

func firstIP4(n *net.IPNet) net.IP {
	return n.IP.Mask(n.Mask).To4()
}

func lastIP4(n *net.IPNet) net.IP {
	ip := n.IP.To4()
	m := net.IPMask(n.Mask)
	if len(ip) != 4 || len(m) != 4 {
		return nil
	}
	out := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		out[i] = ip[i] | ^m[i]
	}
	return out
}

func ip4LessOrEqual(a, b net.IP) bool {
	aa := a.To4()
	bb := b.To4()
	if aa == nil || bb == nil {
		return false
	}
	for i := 0; i < 4; i++ {
		if aa[i] != bb[i] {
			return aa[i] < bb[i]
		}
	}
	return true
}

func cidrOverlap(a, b *net.IPNet) bool {
	a0, a1 := firstIP4(a), lastIP4(a)
	b0, b1 := firstIP4(b), lastIP4(b)
	if a0 == nil || a1 == nil || b0 == nil || b1 == nil {
		return false
	}
	return ip4LessOrEqual(a0, b1) && ip4LessOrEqual(b0, a1)
}

func overlapsExisting(pod, svc *net.IPNet, allocations []CIDRAllocation, skipNetworkID string) error {
	for _, a := range allocations {
		if a.NetworkID == skipNetworkID {
			continue
		}
		if pod != nil && a.PodCIDR != "" {
			_, o, err := net.ParseCIDR(a.PodCIDR)
			if err == nil && cidrOverlap(pod, o) {
				return cferrors.Wrapf(ErrCIDRConflict, "pod CIDR overlaps %s", a.PodCIDR)
			}
		}
		if svc != nil && a.SvcCIDR != "" {
			_, o, err := net.ParseCIDR(a.SvcCIDR)
			if err == nil && cidrOverlap(svc, o) {
				return cferrors.Wrapf(ErrCIDRConflict, "service CIDR overlaps %s", a.SvcCIDR)
			}
		}
	}
	return nil
}

// resolveAllocateCID returns pod and svc CIDR strings for an Allocate call.
func resolveAllocateCID(params AllocateParams, allocations []CIDRAllocation) (pod, svc string, err error) {
	reqP := strings.TrimSpace(params.RequestedPod)
	reqS := strings.TrimSpace(params.RequestedSvc)

	switch {
	case reqP == "" && reqS == "":
		idx, err := nextAutoIndex(allocations)
		if err != nil {
			return "", "", err
		}
		return indexToPodCIDR(idx), indexToSvcCIDR(idx), nil

	case reqP != "" && reqS != "":
		pn, err := mustParseCIDR(reqP)
		if err != nil {
			return "", "", err
		}
		sn, err := mustParseCIDR(reqS)
		if err != nil {
			return "", "", err
		}
		if !cidrInSupernet(pn, superPod) {
			return "", "", cferrors.Wrap(cferrors.CodeInvalidInput, "pod CIDR outside 10.0.0.0/8", cferrors.ErrInvalidInput)
		}
		if !cidrInSupernet(sn, superSvc) {
			return "", "", cferrors.Wrap(cferrors.CodeInvalidInput, "service CIDR outside 172.16.0.0/12", cferrors.ErrInvalidInput)
		}
		if err := overlapsExisting(pn, sn, allocations, params.NetworkID); err != nil {
			return "", "", err
		}
		return reqP, reqS, nil

	case reqP != "" && reqS == "":
		pn, err := mustParseCIDR(reqP)
		if err != nil {
			return "", "", err
		}
		if !cidrInSupernet(pn, superPod) {
			return "", "", cferrors.Wrap(cferrors.CodeInvalidInput, "pod CIDR outside 10.0.0.0/8", cferrors.ErrInvalidInput)
		}
		idx, ok := parseAutoPodIndex(reqP)
		if !ok {
			return "", "", cferrors.Wrap(cferrors.CodeInvalidInput, "auto service allocation requires canonical pod CIDR 10.{i}.0.0/16", cferrors.ErrInvalidInput)
		}
		s := indexToSvcCIDR(idx)
		_, sn, parseErr := net.ParseCIDR(s)
		if parseErr != nil {
			return "", "", cferrors.Wrap(cferrors.CodeInternal, "parse generated service CIDR", cferrors.ErrInternal)
		}
		if err := overlapsExisting(pn, sn, allocations, params.NetworkID); err != nil {
			return "", "", err
		}
		return reqP, s, nil

	case reqP == "" && reqS != "":
		sn, err := mustParseCIDR(reqS)
		if err != nil {
			return "", "", err
		}
		if !cidrInSupernet(sn, superSvc) {
			return "", "", cferrors.Wrap(cferrors.CodeInvalidInput, "service CIDR outside 172.16.0.0/12", cferrors.ErrInvalidInput)
		}
		idx, ok := parseAutoSvcIndex(reqS)
		if !ok {
			return "", "", cferrors.Wrap(cferrors.CodeInvalidInput, "auto pod allocation requires canonical service CIDR from allocator range", cferrors.ErrInvalidInput)
		}
		p := indexToPodCIDR(idx)
		_, pn, parseErr := net.ParseCIDR(p)
		if parseErr != nil {
			return "", "", cferrors.Wrap(cferrors.CodeInternal, "parse generated pod CIDR", cferrors.ErrInternal)
		}
		if err := overlapsExisting(pn, sn, allocations, params.NetworkID); err != nil {
			return "", "", err
		}
		return p, reqS, nil
	}
	return "", "", cferrors.Wrap(cferrors.CodeInternal, "unreachable CIDR resolution branch", cferrors.ErrInternal)
}
