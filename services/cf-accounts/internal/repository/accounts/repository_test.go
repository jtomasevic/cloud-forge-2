package accounts

import (
	"errors"
	"testing"

	"github.com/gocql/gocql"
)

func TestParseUUID_Invalid(t *testing.T) {
	_, err := parseUUID("not-a-uuid")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMapScanErr_NotFound(t *testing.T) {
	err := mapScanErr(gocql.ErrNotFound, ErrAccountNotFound)
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestMapInsertErr_DuplicateMessage(t *testing.T) {
	dup := &fakeRequestError{code: gocql.ErrCodeInvalid, msg: "duplicate key exists"}
	err := mapInsertErr(dup, ErrAccountExists)
	if !errors.Is(err, ErrAccountExists) {
		t.Fatalf("expected ErrAccountExists, got %v", err)
	}
}

type fakeRequestError struct {
	code int
	msg  string
}

func (e *fakeRequestError) Code() int       { return e.code }
func (e *fakeRequestError) Message() string { return e.msg }
func (e *fakeRequestError) Error() string   { return e.msg }
