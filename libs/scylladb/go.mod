module github.com/jtomasevic/cloud-forge-2/libs/scylladb

go 1.26

require (
	github.com/gocql/gocql v1.7.0
	github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core v0.0.0-00010101000000-000000000000
)

require (
	github.com/golang/snappy v0.0.3 // indirect
	github.com/hailocab/go-hostpool v0.0.0-20160125115350-e80d13ce29ed // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
)

replace github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core => ../cloudforge-core
