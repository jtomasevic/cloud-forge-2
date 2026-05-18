module github.com/jtomasevic/cloud-forge-2/services/cf-router

go 1.26.0

require (
	github.com/getkin/kin-openapi v0.138.0
	github.com/gocql/gocql v1.7.0
	github.com/google/uuid v1.6.0
	github.com/jtomasevic/cloud-forge-2/libs/clients/cf-accounts v0.0.0-20260517020151-3de2d2080534
	github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core v0.0.0-20260517020151-3de2d2080534
	github.com/jtomasevic/cloud-forge-2/libs/scylladb v0.0.0-00010101000000-000000000000
	github.com/oapi-codegen/runtime v1.4.0
	golang.org/x/crypto v0.48.0
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/hailocab/go-hostpool v0.0.0-20160125115350-e80d13ce29ed // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/oasdiff/yaml v0.0.9 // indirect
	github.com/oasdiff/yaml3 v0.0.12 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/woodsbury/decimal128 v1.3.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/jtomasevic/cloud-forge-2/libs/clients/cf-accounts => ../../libs/clients/cf-accounts
	github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core => ../../libs/cloudforge-core
	github.com/jtomasevic/cloud-forge-2/libs/scylladb => ../../libs/scylladb
)
