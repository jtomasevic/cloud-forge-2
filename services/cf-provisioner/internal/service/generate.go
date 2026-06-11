package service

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/vcluster_client_mock.go -package=mocks github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/vcluster VClusterClient
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/cilium_client_mock.go -package=mocks github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/cilium CiliumClient
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/gateway_client_mock.go -package=mocks github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/gateway GatewayClient
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/kubeconfig_repository_mock.go -package=mocks github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/kubeconfig KubeconfigRepository
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/cidr_repository_mock.go -package=mocks github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/cidr CIDRRepository
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/jobs_repository_mock.go -package=mocks github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/jobs JobsRepository
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/subnets_repository_mock.go -package=mocks github.com/jtomasevic/cloud-forge-2/services/cf-provisioner/internal/repository/subnets SubnetsRepository
