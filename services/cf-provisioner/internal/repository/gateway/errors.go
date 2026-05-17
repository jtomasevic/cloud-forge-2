package gateway

import cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"

var (
	ErrHTTPRouteNotFound     = cferrors.New(cferrors.CodeNotFound, "httproute not found")
	ErrHTTPRouteCreateFailed = cferrors.New(cferrors.CodeInternal, "failed to create httproute")
)
