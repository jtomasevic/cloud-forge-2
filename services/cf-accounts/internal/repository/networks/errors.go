package networks

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var ErrNetworkNotFound = cferrors.New(cferrors.CodeNotFound, "network not found")
