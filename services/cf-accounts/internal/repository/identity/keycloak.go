package identity

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

type KeycloakConfig struct {
	BaseURL      string
	Realm        string
	AdminRealm   string
	AdminClient  string
	AdminSecret  string
	AdminUser    string
	AdminPass    string
	LoginClient  string
	LoginSecret  string
	HTTPClient   *http.Client
	RequestLimit int64
}

type KeycloakProvider struct {
	baseURL      string
	realm        string
	adminRealm   string
	adminClient  string
	adminSecret  string
	adminUser    string
	adminPass    string
	loginClient  string
	loginSecret  string
	httpClient   *http.Client
	requestLimit int64
}

func NewKeycloakProvider(cfg KeycloakConfig) (*KeycloakProvider, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	realm := strings.TrimSpace(cfg.Realm)
	if baseURL == "" || realm == "" {
		return nil, cferrors.Wrap(cferrors.CodeInternal, "KEYCLOAK_ADMIN_URL and KEYCLOAK_REALM are required", ErrClientConfig)
	}
	adminRealm := strings.TrimSpace(cfg.AdminRealm)
	if adminRealm == "" {
		adminRealm = "master"
	}
	adminClient := strings.TrimSpace(cfg.AdminClient)
	if adminClient == "" {
		adminClient = "admin-cli"
	}
	loginClient := strings.TrimSpace(cfg.LoginClient)
	if loginClient == "" {
		loginClient = "cf-console"
	}
	requestLimit := cfg.RequestLimit
	if requestLimit <= 0 {
		requestLimit = 1 << 20
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &KeycloakProvider{
		baseURL:      baseURL,
		realm:        realm,
		adminRealm:   adminRealm,
		adminClient:  adminClient,
		adminSecret:  strings.TrimSpace(cfg.AdminSecret),
		adminUser:    strings.TrimSpace(cfg.AdminUser),
		adminPass:    cfg.AdminPass,
		loginClient:  loginClient,
		loginSecret:  strings.TrimSpace(cfg.LoginSecret),
		httpClient:   client,
		requestLimit: requestLimit,
	}, nil
}

func closeResponseBody(resp *http.Response) {
	_ = resp.Body.Close()
}

func (p *KeycloakProvider) CreateUser(ctx context.Context, params CreateUserParams) (User, error) {
	id := strings.TrimSpace(params.ID)
	accountID := strings.TrimSpace(params.AccountID)
	email := strings.TrimSpace(params.Email)
	if id == "" || accountID == "" || email == "" || params.Password == "" {
		return User{}, cferrors.Wrap(cferrors.CodeInvalidInput, "identity id, accountID, email, and password are required", cferrors.ErrInvalidInput)
	}

	token, err := p.adminToken(ctx)
	if err != nil {
		return User{}, err
	}

	body := keycloakUser{
		ID:              id,
		Username:        email,
		Email:           email,
		Enabled:         true,
		EmailVerified:   true,
		FirstName:       firstNameFromEmail(email),
		LastName:        "CloudForge",
		RequiredActions: []string{},
		Attributes: map[string][]string{
			"cf_account_id": {accountID},
		},
		Credentials: []keycloakCredential{
			{
				Type:      "password",
				Value:     params.Password,
				Temporary: false,
			},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return User{}, cferrors.Wrap(cferrors.CodeInternal, "marshal identity user", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.adminUsersURL(), bytes.NewReader(raw))
	if err != nil {
		return User{}, cferrors.Wrap(cferrors.CodeInternal, "build identity create request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return User{}, cferrors.Wrap(cferrors.CodeUnavailable, "create identity user", err)
	}
	defer closeResponseBody(resp)

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusNoContent:
		userID := id
		if loc := resp.Header.Get("Location"); loc != "" {
			if parsed := userIDFromLocation(loc); parsed != "" {
				userID = parsed
			}
		}
		if userID == "" {
			var err error
			userID, err = p.findUserIDByUsername(ctx, token, email)
			if err != nil {
				return User{}, err
			}
		}
		if err := p.updateUser(ctx, token, userID, body); err != nil {
			if delErr := p.deleteUserWithToken(context.WithoutCancel(ctx), token, userID); delErr != nil {
				return User{}, cferrors.Wrapf(ErrIdentityService, "update identity user %q failed and delete failed: %v: %v", userID, err, delErr)
			}
			return User{}, err
		}
		return User{ID: userID, Email: email}, nil
	case http.StatusConflict:
		return User{}, ErrUserExists
	default:
		return User{}, p.statusError(resp, "create identity user")
	}
}

func (p *KeycloakProvider) DeleteUser(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	token, err := p.adminToken(ctx)
	if err != nil {
		return err
	}
	return p.deleteUserWithToken(ctx, token, id)
}

func (p *KeycloakProvider) AuthenticatePassword(ctx context.Context, params AuthenticatePasswordParams) (TokenSet, error) {
	email := strings.TrimSpace(params.Email)
	if email == "" || params.Password == "" {
		return TokenSet{}, ErrAuthenticationFailed
	}

	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", p.loginClient)
	form.Set("username", email)
	form.Set("password", params.Password)
	if p.loginSecret != "" {
		form.Set("client_secret", p.loginSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.realmTokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return TokenSet{}, cferrors.Wrap(cferrors.CodeInternal, "build identity password token request", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return TokenSet{}, cferrors.Wrap(cferrors.CodeUnavailable, "request identity password token", err)
	}
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return TokenSet{}, p.passwordTokenStatusError(resp)
	}

	var out keycloakTokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, p.requestLimit)).Decode(&out); err != nil {
		return TokenSet{}, cferrors.Wrap(cferrors.CodeUnavailable, "decode identity password token", err)
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		return TokenSet{}, cferrors.Wrap(cferrors.CodeUnavailable, "identity password token response missing access_token", ErrIdentityService)
	}
	claims, err := decodeAccessTokenClaims(out.AccessToken)
	if err != nil {
		return TokenSet{}, cferrors.Wrap(cferrors.CodeUnavailable, "decode identity access token claims", err)
	}
	accountID := strings.TrimSpace(claims.CFAccountID)
	if accountID == "" {
		return TokenSet{}, ErrAuthenticationFailed
	}
	tokenType := strings.TrimSpace(out.TokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}
	return TokenSet{
		AccessToken:      out.AccessToken,
		RefreshToken:     out.RefreshToken,
		IDToken:          out.IDToken,
		TokenType:        tokenType,
		Scope:            out.Scope,
		ExpiresIn:        out.ExpiresIn,
		RefreshExpiresIn: out.RefreshExpiresIn,
		AccountID:        accountID,
		Subject:          strings.TrimSpace(claims.Subject),
		Email:            strings.TrimSpace(claims.Email),
	}, nil
}

func (p *KeycloakProvider) deleteUserWithToken(ctx context.Context, token, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, p.adminUsersURL()+"/"+url.PathEscape(id), nil)
	if err != nil {
		return cferrors.Wrap(cferrors.CodeInternal, "build identity delete request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return cferrors.Wrap(cferrors.CodeUnavailable, "delete identity user", err)
	}
	defer closeResponseBody(resp)

	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		return p.statusError(resp, "delete identity user")
	}
}

func (p *KeycloakProvider) updateUser(ctx context.Context, token, userID string, user keycloakUser) error {
	user.ID = ""
	user.Credentials = nil
	raw, err := json.Marshal(user)
	if err != nil {
		return cferrors.Wrap(cferrors.CodeInternal, "marshal identity user update", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.adminUsersURL()+"/"+url.PathEscape(userID), bytes.NewReader(raw))
	if err != nil {
		return cferrors.Wrap(cferrors.CodeInternal, "build identity user update request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return cferrors.Wrap(cferrors.CodeUnavailable, "update identity user", err)
	}
	defer closeResponseBody(resp)

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return ErrUserNotFound
	default:
		return p.statusError(resp, "update identity user")
	}
}

func (p *KeycloakProvider) findUserIDByUsername(ctx context.Context, token, username string) (string, error) {
	u, err := url.Parse(p.adminUsersURL())
	if err != nil {
		return "", cferrors.Wrap(cferrors.CodeInternal, "parse identity users URL", err)
	}
	q := u.Query()
	q.Set("username", username)
	q.Set("exact", "true")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", cferrors.Wrap(cferrors.CodeInternal, "build identity user lookup request", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", cferrors.Wrap(cferrors.CodeUnavailable, "lookup identity user", err)
	}
	defer closeResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		return "", p.statusError(resp, "lookup identity user")
	}

	var out []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, p.requestLimit)).Decode(&out); err != nil {
		return "", cferrors.Wrap(cferrors.CodeUnavailable, "decode identity user lookup", err)
	}
	if len(out) == 0 || strings.TrimSpace(out[0].ID) == "" {
		return "", ErrUserNotFound
	}
	return strings.TrimSpace(out[0].ID), nil
}

func (p *KeycloakProvider) adminToken(ctx context.Context) (string, error) {
	form := url.Values{}
	if p.adminSecret != "" {
		form.Set("grant_type", "client_credentials")
		form.Set("client_id", p.adminClient)
		form.Set("client_secret", p.adminSecret)
	} else {
		if p.adminUser == "" || p.adminPass == "" {
			return "", cferrors.Wrap(cferrors.CodeInternal, "KEYCLOAK_ADMIN_USERNAME and KEYCLOAK_ADMIN_PASSWORD are required without KEYCLOAK_ADMIN_CLIENT_SECRET", ErrClientConfig)
		}
		form.Set("grant_type", "password")
		form.Set("client_id", p.adminClient)
		form.Set("username", p.adminUser)
		form.Set("password", p.adminPass)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.adminTokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", cferrors.Wrap(cferrors.CodeInternal, "build identity token request", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", cferrors.Wrap(cferrors.CodeUnavailable, "request identity admin token", err)
	}
	defer closeResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		return "", p.statusError(resp, "request identity admin token")
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, p.requestLimit)).Decode(&out); err != nil {
		return "", cferrors.Wrap(cferrors.CodeUnavailable, "decode identity admin token", err)
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		return "", cferrors.Wrap(cferrors.CodeUnavailable, "identity admin token response missing access_token", ErrIdentityService)
	}
	return out.AccessToken, nil
}

func (p *KeycloakProvider) adminTokenURL() string {
	return p.baseURL + "/realms/" + url.PathEscape(p.adminRealm) + "/protocol/openid-connect/token"
}

func (p *KeycloakProvider) realmTokenURL() string {
	return p.baseURL + "/realms/" + url.PathEscape(p.realm) + "/protocol/openid-connect/token"
}

func (p *KeycloakProvider) adminUsersURL() string {
	return p.baseURL + "/admin/realms/" + url.PathEscape(p.realm) + "/users"
}

func (p *KeycloakProvider) passwordTokenStatusError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, p.requestLimit))
	var body struct {
		Error string `json:"error"`
	}
	if resp.StatusCode == http.StatusBadRequest && json.Unmarshal(raw, &body) == nil && strings.EqualFold(strings.TrimSpace(body.Error), "invalid_grant") {
		return ErrAuthenticationFailed
	}
	if resp.StatusCode == http.StatusBadRequest && strings.Contains(string(raw), "invalid_grant") {
		return ErrAuthenticationFailed
	}
	msg := strings.TrimSpace(string(raw))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return cferrors.Wrapf(ErrIdentityService, "request identity password token: status %d: %s", resp.StatusCode, msg)
}

func (p *KeycloakProvider) statusError(resp *http.Response, op string) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, p.requestLimit))
	msg := strings.TrimSpace(string(raw))
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	return cferrors.Wrapf(ErrIdentityService, "%s: status %d: %s", op, resp.StatusCode, msg)
}

func userIDFromLocation(location string) string {
	u, err := url.Parse(location)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	id := parts[len(parts)-1]
	if id == "users" {
		return ""
	}
	return id
}

type keycloakUser struct {
	ID              string               `json:"id,omitempty"`
	Username        string               `json:"username"`
	Email           string               `json:"email"`
	Enabled         bool                 `json:"enabled"`
	EmailVerified   bool                 `json:"emailVerified"`
	FirstName       string               `json:"firstName,omitempty"`
	LastName        string               `json:"lastName,omitempty"`
	RequiredActions []string             `json:"requiredActions"`
	Attributes      map[string][]string  `json:"attributes,omitempty"`
	Credentials     []keycloakCredential `json:"credentials,omitempty"`
}

type keycloakCredential struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary"`
}

type keycloakTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

type accessTokenClaims struct {
	Subject     string `json:"sub"`
	CFAccountID string `json:"cf_account_id"`
	Email       string `json:"email"`
}

func decodeAccessTokenClaims(token string) (accessTokenClaims, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return accessTokenClaims{}, cferrors.Wrap(cferrors.CodeUnavailable, "identity access token is not a compact JWT", ErrIdentityService)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return accessTokenClaims{}, err
	}
	var claims accessTokenClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return accessTokenClaims{}, err
	}
	return claims, nil
}

var _ Provider = (*KeycloakProvider)(nil)

func firstNameFromEmail(email string) string {
	local, _, ok := strings.Cut(email, "@")
	if !ok || strings.TrimSpace(local) == "" {
		return "CloudForge"
	}
	local = strings.TrimSpace(local)
	if len(local) > 64 {
		return local[:64]
	}
	return local
}
