package rest

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/rest/generated"
	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/service"
)

func TestToServiceCreateAccountParams_RequiresNonEmptyPassword(t *testing.T) {
	_, err := ToServiceCreateAccountParams(&generated.CreateAccountRequest{
		Email:    openapi_types.Email("u@example.com"),
		Password: "   ",
	})
	if err == nil {
		t.Fatal("expected error for blank password")
	}
}

func TestToServiceCreateAccountParams_OK(t *testing.T) {
	p, err := ToServiceCreateAccountParams(&generated.CreateAccountRequest{
		Email:    openapi_types.Email("u@example.com"),
		Password: "longpassword1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Email != "u@example.com" || p.Password != "longpassword1" {
		t.Fatalf("unexpected params: %+v", p)
	}
}

func TestToServiceLoginWithPasswordParams_RequiresNonEmptyPassword(t *testing.T) {
	_, err := ToServiceLoginWithPasswordParams(&generated.LoginRequest{
		Email:    openapi_types.Email("u@example.com"),
		Password: "",
	})
	if err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestToServiceLoginWithPasswordParams_OK(t *testing.T) {
	p, err := ToServiceLoginWithPasswordParams(&generated.LoginRequest{
		Email:    openapi_types.Email("u@example.com"),
		Password: "password1!",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Email != "u@example.com" || p.Password != "password1!" {
		t.Fatalf("unexpected params: %+v", p)
	}
}

func TestToLoginResponseFromService(t *testing.T) {
	created := time.Date(2026, 6, 8, 20, 49, 3, 0, time.UTC)
	out := ToLoginResponseFromService(service.LoginResult{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		IDToken:          "id-token",
		TokenType:        "Bearer",
		Scope:            "openid email",
		ExpiresIn:        300,
		RefreshExpiresIn: 1800,
		Account: service.Account{
			ID:        "222e9117-3fed-4915-b591-5f524f1fe158",
			Email:     "u@example.com",
			Status:    "active",
			CreatedAt: created,
			UpdatedAt: created,
		},
	})
	if out.AccessToken != "access-token" || out.TokenType != "Bearer" || out.ExpiresIn != 300 {
		t.Fatalf("token fields mismatch: %+v", out)
	}
	if out.RefreshToken == nil || *out.RefreshToken != "refresh-token" {
		t.Fatalf("refresh token: %+v", out.RefreshToken)
	}
	if out.RefreshExpiresIn == nil || *out.RefreshExpiresIn != 1800 {
		t.Fatalf("refresh expiry: %+v", out.RefreshExpiresIn)
	}
	if out.IdToken == nil || *out.IdToken != "id-token" {
		t.Fatalf("id token: %+v", out.IdToken)
	}
	if out.Scope == nil || *out.Scope != "openid email" {
		t.Fatalf("scope: %+v", out.Scope)
	}
	if out.Account.Email != openapi_types.Email("u@example.com") || out.Account.Id.String() != "222e9117-3fed-4915-b591-5f524f1fe158" {
		t.Fatalf("account mismatch: %+v", out.Account)
	}
}
