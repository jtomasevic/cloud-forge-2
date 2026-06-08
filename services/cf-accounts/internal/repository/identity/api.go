package identity

import "context"

// Provider creates and removes users in the configured identity provider.
type Provider interface {
	CreateUser(ctx context.Context, params CreateUserParams) (User, error)
	DeleteUser(ctx context.Context, id string) error
	AuthenticatePassword(ctx context.Context, params AuthenticatePasswordParams) (TokenSet, error)
}

type CreateUserParams struct {
	ID        string
	AccountID string
	Email     string
	Password  string
}

type User struct {
	ID    string
	Email string
}

type AuthenticatePasswordParams struct {
	Email    string
	Password string
}

type TokenSet struct {
	AccessToken      string
	RefreshToken     string
	IDToken          string
	TokenType        string
	Scope            string
	ExpiresIn        int
	RefreshExpiresIn int
	AccountID        string
	Subject          string
	Email            string
}
