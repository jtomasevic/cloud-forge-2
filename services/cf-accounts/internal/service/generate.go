package service

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/accounts_repository_mock.go -package=mocks github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/accounts AccountsRepository
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/tenants_repository_mock.go -package=mocks github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/tenants TenantsRepository
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/networks_repository_mock.go -package=mocks github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/networks NetworksRepository
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/credentials_repository_mock.go -package=mocks github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/credentials CredentialsRepository
