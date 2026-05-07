package client

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

// ErrConnectionFailed is returned when a ScyllaDB session cannot be established.
var ErrConnectionFailed = cferrors.New(cferrors.CodeUnavailable, "scylladb connection failed")
