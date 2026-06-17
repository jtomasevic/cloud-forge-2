# Cilium Repository

`cilium` manages `cilium.io/v2` `CiliumNetworkPolicy` resources as unstructured objects. The module does not vendor Cilium API types, so tests inspect the generated object maps with the dynamic fake client.

## Existing Network Policies

The package keeps the original network-level policies:

- default-deny policy after vCluster creation
- internet ingress policy used by the existing network gateway flow

These methods remain compatible with the current service-layer gateway orchestration.

## App Service Ingress

`ApplyAppServiceIngressPolicy` creates or updates a public ingress policy for one CF App Service. Its `endpointSelector.matchLabels` includes tenant ID, network ID, subnet ID, app service ID, and `cloudforge.io/visibility=public`.

The selector is intentionally narrow. A namespace-wide selector would expose every workload in the tenant vCluster, including private-subnet services that must never receive public internet traffic.

The policy allows `world` only to the declared exposed port and always uses TCP at the Cilium layer. HTTP, gRPC, and TCP protocol-specific routing is handled by Gateway API objects.

## Cleanup

`RemoveAppServiceIngressPolicy` deletes the service-specific policy by namespace and app service ID. Missing policies are treated as success by `RemovePolicy`, which keeps exposure-removal jobs retry-safe.
