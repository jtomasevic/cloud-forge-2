package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKeycloakProviderCreateUserUpdatesAttributes(t *testing.T) {
	const (
		accountID = "acc-1"
		userID    = "kc-user-1"
		email     = "new@example.com"
		password  = "password1"
	)

	var updated keycloakUser
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/realms/master/protocol/openid-connect/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if got := r.Form.Get("grant_type"); got != "password" {
				t.Fatalf("grant_type: got %q", got)
			}
			_, _ = w.Write([]byte(`{"access_token":"admin-token"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/admin/realms/cloudforge/users":
			var created keycloakUser
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatalf("decode create user: %v", err)
			}
			if created.Username != email || created.Email != email || len(created.Credentials) != 1 || created.Credentials[0].Value != password {
				t.Fatalf("created user mismatch: %+v", created)
			}
			w.Header().Set("Location", serverURL(r)+"/admin/realms/cloudforge/users/"+userID)
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPut && r.URL.Path == "/admin/realms/cloudforge/users/"+userID:
			if got := r.Header.Get("Authorization"); got != "Bearer admin-token" {
				t.Fatalf("authorization header: got %q", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
				t.Fatalf("decode update user: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider, err := NewKeycloakProvider(KeycloakConfig{
		BaseURL:     server.URL,
		Realm:       "cloudforge",
		AdminUser:   "admin",
		AdminPass:   "admin",
		HTTPClient:  server.Client(),
		AdminClient: "admin-cli",
	})
	if err != nil {
		t.Fatalf("NewKeycloakProvider: %v", err)
	}

	user, err := provider.CreateUser(context.Background(), CreateUserParams{
		ID:        accountID,
		AccountID: accountID,
		Email:     email,
		Password:  password,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.ID != userID || user.Email != email {
		t.Fatalf("created identity user: %+v", user)
	}
	got := updated.Attributes["cf_account_id"]
	if len(got) != 1 || got[0] != accountID {
		t.Fatalf("cf_account_id attribute: got %#v", got)
	}
	if len(updated.Credentials) != 0 {
		t.Fatalf("update should not include credentials: %+v", updated.Credentials)
	}
}

func TestKeycloakProviderCreateUserDeletesUserWhenUpdateFails(t *testing.T) {
	const (
		accountID = "acc-1"
		userID    = "kc-user-1"
		email     = "new@example.com"
	)

	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/realms/master/protocol/openid-connect/token":
			_, _ = w.Write([]byte(`{"access_token":"admin-token"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/admin/realms/cloudforge/users":
			w.Header().Set("Location", serverURL(r)+"/admin/realms/cloudforge/users/"+userID)
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPut && r.URL.Path == "/admin/realms/cloudforge/users/"+userID:
			http.Error(w, "update failed", http.StatusInternalServerError)
		case r.Method == http.MethodDelete && r.URL.Path == "/admin/realms/cloudforge/users/"+userID:
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider, err := NewKeycloakProvider(KeycloakConfig{
		BaseURL:     server.URL,
		Realm:       "cloudforge",
		AdminUser:   "admin",
		AdminPass:   "admin",
		HTTPClient:  server.Client(),
		AdminClient: "admin-cli",
	})
	if err != nil {
		t.Fatalf("NewKeycloakProvider: %v", err)
	}

	_, err = provider.CreateUser(context.Background(), CreateUserParams{
		ID:        accountID,
		AccountID: accountID,
		Email:     email,
		Password:  "password1",
	})
	if err == nil || !strings.Contains(err.Error(), "update identity user") {
		t.Fatalf("expected update error, got %v", err)
	}
	if !deleted {
		t.Fatal("expected created identity user to be deleted after update failure")
	}
}

func TestKeycloakProviderAuthenticatePasswordSuccess(t *testing.T) {
	const (
		accountID = "acc-1"
		subject   = "kc-user-1"
		email     = "new@example.com"
		password  = "password1"
	)

	token := testJWT(map[string]string{
		"sub":           subject,
		"cf_account_id": accountID,
		"email":         email,
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/realms/cloudforge/protocol/openid-connect/token" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "password" {
			t.Fatalf("grant_type: got %q", got)
		}
		if got := r.Form.Get("client_id"); got != "cf-console" {
			t.Fatalf("client_id: got %q", got)
		}
		if got := r.Form.Get("username"); got != email {
			t.Fatalf("username: got %q", got)
		}
		if got := r.Form.Get("password"); got != password {
			t.Fatalf("password: got %q", got)
		}
		_ = json.NewEncoder(w).Encode(keycloakTokenResponse{
			AccessToken:      token,
			RefreshToken:     "refresh",
			IDToken:          "id-token",
			TokenType:        "Bearer",
			Scope:            "openid email",
			ExpiresIn:        300,
			RefreshExpiresIn: 1800,
		})
	}))
	defer server.Close()

	provider, err := NewKeycloakProvider(KeycloakConfig{
		BaseURL:    server.URL,
		Realm:      "cloudforge",
		AdminUser:  "admin",
		AdminPass:  "admin",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewKeycloakProvider: %v", err)
	}

	out, err := provider.AuthenticatePassword(context.Background(), AuthenticatePasswordParams{
		Email:    email,
		Password: password,
	})
	if err != nil {
		t.Fatalf("AuthenticatePassword: %v", err)
	}
	if out.AccessToken != token || out.AccountID != accountID || out.Subject != subject || out.Email != email {
		t.Fatalf("unexpected token set: %+v", out)
	}
	if out.RefreshToken != "refresh" || out.IDToken != "id-token" || out.TokenType != "Bearer" || out.ExpiresIn != 300 || out.RefreshExpiresIn != 1800 {
		t.Fatalf("token metadata mismatch: %+v", out)
	}
}

func TestKeycloakProviderAuthenticatePasswordInvalidGrant(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			}))
			defer server.Close()

			provider, err := NewKeycloakProvider(KeycloakConfig{
				BaseURL:    server.URL,
				Realm:      "cloudforge",
				AdminUser:  "admin",
				AdminPass:  "admin",
				HTTPClient: server.Client(),
			})
			if err != nil {
				t.Fatalf("NewKeycloakProvider: %v", err)
			}

			_, err = provider.AuthenticatePassword(context.Background(), AuthenticatePasswordParams{
				Email:    "new@example.com",
				Password: "wrong-password",
			})
			if !errors.Is(err, ErrAuthenticationFailed) {
				t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
			}
		})
	}
}

func TestKeycloakProviderAuthenticatePasswordMissingAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(keycloakTokenResponse{
			TokenType: "Bearer",
			ExpiresIn: 300,
		})
	}))
	defer server.Close()

	provider, err := NewKeycloakProvider(KeycloakConfig{
		BaseURL:    server.URL,
		Realm:      "cloudforge",
		AdminUser:  "admin",
		AdminPass:  "admin",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewKeycloakProvider: %v", err)
	}

	_, err = provider.AuthenticatePassword(context.Background(), AuthenticatePasswordParams{
		Email:    "new@example.com",
		Password: "password1",
	})
	if !errors.Is(err, ErrIdentityService) {
		t.Fatalf("expected ErrIdentityService, got %v", err)
	}
}

func TestKeycloakProviderAuthenticatePasswordMissingAccountClaim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(keycloakTokenResponse{
			AccessToken: testJWT(map[string]string{
				"sub":   "kc-user-1",
				"email": "new@example.com",
			}),
			TokenType: "Bearer",
			ExpiresIn: 300,
		})
	}))
	defer server.Close()

	provider, err := NewKeycloakProvider(KeycloakConfig{
		BaseURL:    server.URL,
		Realm:      "cloudforge",
		AdminUser:  "admin",
		AdminPass:  "admin",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewKeycloakProvider: %v", err)
	}

	_, err = provider.AuthenticatePassword(context.Background(), AuthenticatePasswordParams{
		Email:    "new@example.com",
		Password: "password1",
	})
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("expected ErrAuthenticationFailed, got %v", err)
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

func testJWT(claims map[string]string) string {
	header, _ := json.Marshal(map[string]string{"alg": "none"})
	payload, _ := json.Marshal(claims)
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
