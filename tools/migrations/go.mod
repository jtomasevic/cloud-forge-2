module github.com/jtomasevic/cloud-forge-2/tools/migrations

go 1.26

require github.com/jtomasevic/cloud-forge-2/libs/scylladb v0.0.0-00010101000000-000000000000

require (
	github.com/gocql/gocql v1.7.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/hailocab/go-hostpool v0.0.0-20160125115350-e80d13ce29ed // indirect
	github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core v0.0.0-00010101000000-000000000000 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
)

replace (
	github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core => ../../libs/cloudforge-core
	github.com/jtomasevic/cloud-forge-2/libs/scylladb => ../../libs/scylladb
)
