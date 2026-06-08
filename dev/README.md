## Get auth for swagger

For local dev, get a Bearer token through the CloudForge login API:

```
curl -sf -X POST http://api.cloudforge.local:18080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"dev-user@cloudforge.io","password":"devpassword"}' \
  | jq -r .accessToken
```

Then in http://api.cloudforge.local:18080/swagger/:

- Click Authorize.
- Under BearerAuth, paste only the JWT value.
- Do not prefix it with Bearer ; Swagger UI adds that.
- Click Authorize, then execute requests.

`POST /v1/accounts` is public signup and does not require Authorize.
After signup, use the same email and password with `POST /v1/auth/login`.
CF-Accounts authenticates through Keycloak and returns an access token with the
`cf_account_id` claim.
