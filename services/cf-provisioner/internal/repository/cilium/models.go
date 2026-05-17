package cilium

type IngressPolicyParams struct {
	VClusterNamespace  string
	NetworkID          string
	PublicEndpointPort int
}

type PolicyInfo struct {
	Name      string
	Namespace string
	Exists    bool
}
