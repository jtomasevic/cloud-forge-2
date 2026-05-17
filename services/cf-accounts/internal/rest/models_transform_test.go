package rest

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/rest/generated"
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
