package main

//go:generate bash -c "cd ../../api/cf-accounts/v1 && oapi-codegen -config oapi-server.cfg.yaml openapi.yaml"
//go:generate bash -c "cd ../../api/cf-accounts/v1 && oapi-codegen -config oapi-client.cfg.yaml openapi.yaml"
