package jobs

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var ErrJobNotFound = cferrors.New(cferrors.CodeNotFound, "provisioning job not found")
