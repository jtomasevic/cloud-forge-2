package service

import (
	"strings"

	"golang.org/x/crypto/bcrypt"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
)

const (
	minPasswordLen         = 8
	maxPasswordBcryptBytes = 72 // bcrypt limit in Go's implementation
)

// validateNewAccountPassword enforces signup rules: non-whitespace-only, length 8–72 bytes.
func validateNewAccountPassword(password string) error {
	if strings.TrimSpace(password) == "" {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "password is required", cferrors.ErrInvalidInput)
	}
	if len(password) < minPasswordLen {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "password must be at least 8 characters", cferrors.ErrInvalidInput)
	}
	if len(password) > maxPasswordBcryptBytes {
		return cferrors.Wrap(cferrors.CodeInvalidInput, "password must be at most 72 bytes", cferrors.ErrInvalidInput)
	}
	return nil
}

// hashPasswordBcrypt returns a bcrypt hash string suitable for the accounts.password_hash column.
func hashPasswordBcrypt(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", cferrors.Wrap(cferrors.CodeInternal, "failed to hash password", err)
	}
	return string(hashed), nil
}

// comparePasswordBcrypt returns nil if plain matches hashedPassword, otherwise bcrypt's compare error.
func comparePasswordBcrypt(hashedPassword, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plain))
}
