package service

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/gocql/gocql"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/blake2b"

	cferrors "github.com/jtomasevic/cloud-forge-2/libs/cloudforge-core/pkg/errors"
	accountsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/accounts"
	credentialsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/credentials"
	identityrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/identity"
	networksrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/networks"
	tenantsrepo "github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/repository/tenants"
	"github.com/jtomasevic/cloud-forge-2/services/cf-accounts/internal/service/mocks"
)

type fakeIdentityProvider struct {
	createParams       []identityrepo.CreateUserParams
	createErr          error
	createID           string
	deleteIDs          []string
	authenticateParams []identityrepo.AuthenticatePasswordParams
	authenticateResult identityrepo.TokenSet
	authenticateErr    error
}

func (f *fakeIdentityProvider) CreateUser(ctx context.Context, params identityrepo.CreateUserParams) (identityrepo.User, error) {
	_ = ctx
	f.createParams = append(f.createParams, params)
	if f.createErr != nil {
		return identityrepo.User{}, f.createErr
	}
	id := f.createID
	if id == "" {
		id = params.ID
	}
	return identityrepo.User{ID: id, Email: params.Email}, nil
}

func (f *fakeIdentityProvider) DeleteUser(ctx context.Context, id string) error {
	_ = ctx
	f.deleteIDs = append(f.deleteIDs, id)
	return nil
}

func (f *fakeIdentityProvider) AuthenticatePassword(ctx context.Context, params identityrepo.AuthenticatePasswordParams) (identityrepo.TokenSet, error) {
	_ = ctx
	f.authenticateParams = append(f.authenticateParams, params)
	if f.authenticateErr != nil {
		return identityrepo.TokenSet{}, f.authenticateErr
	}
	return f.authenticateResult, nil
}

func TestCreateAccount_ErrAccountEmailTaken(t *testing.T) {
	ctrl := gomock.NewController(t)
	ma := mocks.NewMockAccountsRepository(ctrl)
	ma.EXPECT().
		GetByEmail(gomock.Any(), "taken@example.com").
		Return(accountsrepo.AccountRow{Email: "taken@example.com"}, nil)

	svc := New(Deps{Accounts: ma})
	_, err := svc.CreateAccount(context.Background(), CreateAccountParams{Email: "taken@example.com", Password: "longpassword1"})
	if !errors.Is(err, ErrAccountEmailTaken) {
		t.Fatalf("expected ErrAccountEmailTaken, got %v", err)
	}
}

func TestCreateAccount_CreatesDefaultTenantSlugFromEmail(t *testing.T) {
	ctrl := gomock.NewController(t)

	ma := mocks.NewMockAccountsRepository(ctrl)
	ma.EXPECT().
		GetByEmail(gomock.Any(), "Alice.User+tag@Example.com").
		Return(accountsrepo.AccountRow{}, accountsrepo.ErrAccountNotFound)
	ma.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, row accountsrepo.AccountRow) {
			if row.PasswordHash == "" || !strings.HasPrefix(row.PasswordHash, "$2") {
				t.Fatalf("expected bcrypt hash prefix, got %q", row.PasswordHash)
			}
		}).
		Return(nil)

	mt := mocks.NewMockTenantsRepository(ctrl)
	mt.EXPECT().
		GetBySlug(gomock.Any(), "alice-user-tag").
		Return(tenantsrepo.TenantRow{}, tenantsrepo.ErrTenantNotFound)
	mt.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Return(nil)

	svc := New(Deps{Accounts: ma, Tenants: mt})
	out, err := svc.CreateAccount(context.Background(), CreateAccountParams{Email: "Alice.User+tag@Example.com", Password: "longpassword1"})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	wantSlug := "alice-user-tag"
	if out.DefaultTenant.Slug != wantSlug {
		t.Fatalf("tenant slug: want %q, got %q", wantSlug, out.DefaultTenant.Slug)
	}
	if out.DefaultTenant.Status != "active" {
		t.Fatalf("tenant status: want active, got %q", out.DefaultTenant.Status)
	}
	if out.Account.Email != "Alice.User+tag@Example.com" {
		t.Fatalf("account email: got %q", out.Account.Email)
	}
}

func TestCreateAccount_CreatesIdentityUser(t *testing.T) {
	ctrl := gomock.NewController(t)

	ma := mocks.NewMockAccountsRepository(ctrl)
	ma.EXPECT().
		GetByEmail(gomock.Any(), "new@example.com").
		Return(accountsrepo.AccountRow{}, accountsrepo.ErrAccountNotFound)
	ma.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Return(nil)

	mt := mocks.NewMockTenantsRepository(ctrl)
	mt.EXPECT().
		GetBySlug(gomock.Any(), "new").
		Return(tenantsrepo.TenantRow{}, tenantsrepo.ErrTenantNotFound)
	mt.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Return(nil)

	idp := &fakeIdentityProvider{createID: "keycloak-user-id"}
	svc := New(Deps{Accounts: ma, Tenants: mt, Identity: idp})

	out, err := svc.CreateAccount(context.Background(), CreateAccountParams{Email: "new@example.com", Password: "longpassword1"})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if len(idp.createParams) != 1 {
		t.Fatalf("identity CreateUser calls: got %d want 1", len(idp.createParams))
	}
	got := idp.createParams[0]
	if got.ID != out.Account.ID || got.AccountID != out.Account.ID {
		t.Fatalf("identity ids not linked to account: params=%+v account=%+v", got, out.Account)
	}
	if got.Email != "new@example.com" || got.Password != "longpassword1" {
		t.Fatalf("identity user params mismatch: %+v", got)
	}
	if out.DefaultTenant.Status != "active" {
		t.Fatalf("tenant status: want active, got %q", out.DefaultTenant.Status)
	}
}

func TestCreateAccount_IdentityConflictReturnsEmailTaken(t *testing.T) {
	ctrl := gomock.NewController(t)

	ma := mocks.NewMockAccountsRepository(ctrl)
	ma.EXPECT().
		GetByEmail(gomock.Any(), "new@example.com").
		Return(accountsrepo.AccountRow{}, accountsrepo.ErrAccountNotFound)

	mt := mocks.NewMockTenantsRepository(ctrl)
	mt.EXPECT().
		GetBySlug(gomock.Any(), "new").
		Return(tenantsrepo.TenantRow{}, tenantsrepo.ErrTenantNotFound)

	idp := &fakeIdentityProvider{createErr: identityrepo.ErrUserExists}
	svc := New(Deps{Accounts: ma, Tenants: mt, Identity: idp})

	_, err := svc.CreateAccount(context.Background(), CreateAccountParams{Email: "new@example.com", Password: "longpassword1"})
	if !errors.Is(err, ErrAccountEmailTaken) {
		t.Fatalf("expected ErrAccountEmailTaken, got %v", err)
	}
}

func TestCreateAccount_RollsBackIdentityWhenAccountInsertFails(t *testing.T) {
	ctrl := gomock.NewController(t)

	ma := mocks.NewMockAccountsRepository(ctrl)
	ma.EXPECT().
		GetByEmail(gomock.Any(), "new@example.com").
		Return(accountsrepo.AccountRow{}, accountsrepo.ErrAccountNotFound)
	ma.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Return(cferrors.ErrInternal)

	mt := mocks.NewMockTenantsRepository(ctrl)
	mt.EXPECT().
		GetBySlug(gomock.Any(), "new").
		Return(tenantsrepo.TenantRow{}, tenantsrepo.ErrTenantNotFound)

	idp := &fakeIdentityProvider{createID: "keycloak-user-id"}
	svc := New(Deps{Accounts: ma, Tenants: mt, Identity: idp})

	_, err := svc.CreateAccount(context.Background(), CreateAccountParams{Email: "new@example.com", Password: "longpassword1"})
	if !errors.Is(err, cferrors.ErrInternal) {
		t.Fatalf("expected ErrInternal, got %v", err)
	}
	if len(idp.createParams) != 1 {
		t.Fatalf("identity CreateUser calls: got %d want 1", len(idp.createParams))
	}
	if len(idp.deleteIDs) != 1 || idp.deleteIDs[0] != "keycloak-user-id" {
		t.Fatalf("identity rollback delete IDs: got %#v create=%#v", idp.deleteIDs, idp.createParams)
	}
}

func TestCreateAccount_ShortPassword(t *testing.T) {
	ctrl := gomock.NewController(t)
	ma := mocks.NewMockAccountsRepository(ctrl)
	svc := New(Deps{Accounts: ma})
	_, err := svc.CreateAccount(context.Background(), CreateAccountParams{Email: "new@example.com", Password: "short"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, cferrors.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestLoginWithPassword_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	accountID := uuid.New().String()
	uid, err := gocql.ParseUUID(accountID)
	if err != nil {
		t.Fatal(err)
	}

	ma := mocks.NewMockAccountsRepository(ctrl)
	ma.EXPECT().
		GetByID(gomock.Any(), accountID).
		Return(accountsrepo.AccountRow{
			ID:     uid,
			Email:  "ok@example.com",
			Status: "active",
		}, nil)

	idp := &fakeIdentityProvider{authenticateResult: identityrepo.TokenSet{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		IDToken:          "id-token",
		TokenType:        "Bearer",
		Scope:            "openid email",
		ExpiresIn:        300,
		RefreshExpiresIn: 1800,
		AccountID:        accountID,
		Subject:          "keycloak-user-id",
		Email:            "OK@example.com",
	}}
	svc := New(Deps{Accounts: ma, Identity: idp})
	out, err := svc.LoginWithPassword(context.Background(), LoginWithPasswordParams{
		Email:    "ok@example.com",
		Password: "right-password1",
	})
	if err != nil {
		t.Fatalf("LoginWithPassword: %v", err)
	}
	if len(idp.authenticateParams) != 1 || idp.authenticateParams[0].Email != "ok@example.com" || idp.authenticateParams[0].Password != "right-password1" {
		t.Fatalf("identity authenticate params: %+v", idp.authenticateParams)
	}
	if out.AccessToken != "access-token" || out.RefreshToken != "refresh-token" || out.IDToken != "id-token" || out.TokenType != "Bearer" || out.ExpiresIn != 300 || out.RefreshExpiresIn != 1800 {
		t.Fatalf("unexpected token result: %+v", out)
	}
	if out.Account.ID != uid.String() || out.Account.Email != "ok@example.com" {
		t.Fatalf("unexpected account: %+v", out.Account)
	}
}

func TestLoginWithPassword_WrongPassword(t *testing.T) {
	idp := &fakeIdentityProvider{authenticateErr: identityrepo.ErrAuthenticationFailed}
	svc := New(Deps{Identity: idp})
	_, err := svc.LoginWithPassword(context.Background(), LoginWithPasswordParams{
		Email:    "ok@example.com",
		Password: "wrong-password1",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginWithPassword_InactiveAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	accountID := uuid.New().String()
	uid, err := gocql.ParseUUID(accountID)
	if err != nil {
		t.Fatal(err)
	}

	ma := mocks.NewMockAccountsRepository(ctrl)
	ma.EXPECT().
		GetByID(gomock.Any(), accountID).
		Return(accountsrepo.AccountRow{
			ID:     uid,
			Email:  "ok@example.com",
			Status: "suspended",
		}, nil)

	idp := &fakeIdentityProvider{authenticateResult: identityrepo.TokenSet{
		AccessToken: "access-token",
		TokenType:   "Bearer",
		AccountID:   accountID,
		Email:       "ok@example.com",
	}}
	svc := New(Deps{Accounts: ma, Identity: idp})
	_, err = svc.LoginWithPassword(context.Background(), LoginWithPasswordParams{
		Email:    "ok@example.com",
		Password: "right-password1",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginWithPassword_TokenEmailMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	accountID := uuid.New().String()
	uid, err := gocql.ParseUUID(accountID)
	if err != nil {
		t.Fatal(err)
	}

	ma := mocks.NewMockAccountsRepository(ctrl)
	ma.EXPECT().
		GetByID(gomock.Any(), accountID).
		Return(accountsrepo.AccountRow{
			ID:     uid,
			Email:  "ok@example.com",
			Status: "active",
		}, nil)

	idp := &fakeIdentityProvider{authenticateResult: identityrepo.TokenSet{
		AccessToken: "access-token",
		TokenType:   "Bearer",
		AccountID:   accountID,
		Email:       "other@example.com",
	}}
	svc := New(Deps{Accounts: ma, Identity: idp})
	_, err = svc.LoginWithPassword(context.Background(), LoginWithPasswordParams{
		Email:    "ok@example.com",
		Password: "right-password1",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginWithPassword_TokenAccountMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	tokenAccountID := uuid.New().String()
	otherAccountID := uuid.New().String()
	otherUID, err := gocql.ParseUUID(otherAccountID)
	if err != nil {
		t.Fatal(err)
	}

	ma := mocks.NewMockAccountsRepository(ctrl)
	ma.EXPECT().
		GetByID(gomock.Any(), tokenAccountID).
		Return(accountsrepo.AccountRow{
			ID:     otherUID,
			Email:  "ok@example.com",
			Status: "active",
		}, nil)

	idp := &fakeIdentityProvider{authenticateResult: identityrepo.TokenSet{
		AccessToken: "access-token",
		TokenType:   "Bearer",
		AccountID:   tokenAccountID,
		Email:       "ok@example.com",
	}}
	svc := New(Deps{Accounts: ma, Identity: idp})
	_, err = svc.LoginWithPassword(context.Background(), LoginWithPasswordParams{
		Email:    "ok@example.com",
		Password: "right-password1",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginWithPassword_MissingIdentityProvider(t *testing.T) {
	svc := New(Deps{})
	_, err := svc.LoginWithPassword(context.Background(), LoginWithPasswordParams{
		Email:    "ok@example.com",
		Password: "right-password1",
	})
	if !errors.Is(err, ErrIdentityProviderNotConfigured) {
		t.Fatalf("expected ErrIdentityProviderNotConfigured, got %v", err)
	}
}

func TestLoginWithPassword_ShortPassword(t *testing.T) {
	svc := New(Deps{})
	_, err := svc.LoginWithPassword(context.Background(), LoginWithPasswordParams{
		Email:    "any@example.com",
		Password: "short",
	})
	if !errors.Is(err, cferrors.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateCredential_SecretAndHash(t *testing.T) {
	ctrl := gomock.NewController(t)
	aid := uuid.New().String()
	var captured credentialsrepo.APIKeyRow

	ma := mocks.NewMockAccountsRepository(ctrl)
	ma.EXPECT().GetByID(gomock.Any(), aid).Return(accountsrepo.AccountRow{}, nil)

	mc := mocks.NewMockCredentialsRepository(ctrl)
	mc.EXPECT().
		Insert(gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, row credentialsrepo.APIKeyRow) {
			captured = row
		}).
		Return(nil)

	svc := New(Deps{Accounts: ma, Credentials: mc})
	out, err := svc.CreateCredential(context.Background(), CreateCredentialParams{AccountID: aid})
	if err != nil {
		t.Fatalf("CreateCredential: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(out.Secret)
	if err != nil || len(raw) != 32 {
		t.Fatalf("secret must decode to 32 raw bytes, got len=%d err=%v", len(raw), err)
	}
	sum := blake2b.Sum256(raw)
	wantHash := hex.EncodeToString(sum[:])
	if captured.KeyHash != wantHash {
		t.Fatalf("stored hash mismatch: want %q got %q", wantHash, captured.KeyHash)
	}
	if len(out.Secret) < 8 || captured.KeyPrefix != out.Secret[:8] {
		t.Fatalf("prefix mismatch: prefix=%q secret[:8]=%q", captured.KeyPrefix, out.Secret[:8])
	}
}

func TestResolveTenantContext_APIKeyHashNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockCredentialsRepository(ctrl)
	mc.EXPECT().
		GetByHash(gomock.Any(), "deadbeef").
		Return(credentialsrepo.APIKeyRow{}, credentialsrepo.ErrCredentialNotFound)

	svc := New(Deps{Credentials: mc})
	_, err := svc.ResolveTenantContext(context.Background(), ResolveTenantParams{APIKeyHash: "deadbeef"})
	if !errors.Is(err, ErrResolutionFailed) {
		t.Fatalf("expected ErrResolutionFailed, got %v", err)
	}
}

func TestResolveTenantContext_APIKeyRevoked(t *testing.T) {
	ctrl := gomock.NewController(t)
	mc := mocks.NewMockCredentialsRepository(ctrl)
	mc.EXPECT().
		GetByHash(gomock.Any(), "any").
		Return(credentialsrepo.APIKeyRow{}, credentialsrepo.ErrCredentialRevoked)

	svc := New(Deps{Credentials: mc})
	_, err := svc.ResolveTenantContext(context.Background(), ResolveTenantParams{APIKeyHash: "any"})
	if !errors.Is(err, ErrResolutionFailed) {
		t.Fatalf("expected ErrResolutionFailed, got %v", err)
	}
}

func TestEmailLocalPartToSlug(t *testing.T) {
	if got := emailLocalPartToSlug("user@example.com"); got != "user" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveTenantContext_ByAccountID(t *testing.T) {
	ctrl := gomock.NewController(t)
	accID := uuid.New().String()
	tid := uuid.New()
	nid := uuid.New()

	ma := mocks.NewMockAccountsRepository(ctrl)
	uid, _ := gocql.ParseUUID(accID)
	ma.EXPECT().GetByID(gomock.Any(), accID).Return(accountsrepo.AccountRow{ID: uid}, nil)

	mt := mocks.NewMockTenantsRepository(ctrl)
	aid, _ := gocql.ParseUUID(accID)
	mt.EXPECT().ListByAccount(gomock.Any(), accID, 1, 0).Return([]tenantsrepo.TenantRow{{
		ID:        gocql.UUID(tid),
		AccountID: aid,
		Slug:      "t1",
		Region:    "us-west-2",
		Status:    "active",
	}}, nil)

	mn := mocks.NewMockNetworksRepository(ctrl)
	mn.EXPECT().ListByTenant(gomock.Any(), tid.String()).Return([]networksrepo.NetworkRow{{
		ID:       gocql.UUID(nid),
		Status:   "active",
		Region:   "us-west-2",
		TenantID: gocql.UUID(tid),
	}}, nil)

	svc := New(Deps{Accounts: ma, Tenants: mt, Networks: mn})
	tc, err := svc.ResolveTenantContext(context.Background(), ResolveTenantParams{AccountID: accID})
	if err != nil {
		t.Fatal(err)
	}
	if tc.TenantID != tid.String() || tc.NetworkID != nid.String() || tc.AccountID != accID {
		t.Fatalf("unexpected context: %+v", tc)
	}
}

func TestResolveTenantContext_ByAccountIDWithoutNetwork(t *testing.T) {
	ctrl := gomock.NewController(t)
	accID := uuid.New().String()
	tid := uuid.New()

	ma := mocks.NewMockAccountsRepository(ctrl)
	uid, _ := gocql.ParseUUID(accID)
	ma.EXPECT().GetByID(gomock.Any(), accID).Return(accountsrepo.AccountRow{ID: uid}, nil)

	mt := mocks.NewMockTenantsRepository(ctrl)
	aid, _ := gocql.ParseUUID(accID)
	mt.EXPECT().ListByAccount(gomock.Any(), accID, 1, 0).Return([]tenantsrepo.TenantRow{{
		ID:        gocql.UUID(tid),
		AccountID: aid,
		Slug:      "t1",
		Status:    "active",
	}}, nil)

	mn := mocks.NewMockNetworksRepository(ctrl)
	mn.EXPECT().ListByTenant(gomock.Any(), tid.String()).Return(nil, nil)

	svc := New(Deps{Accounts: ma, Tenants: mt, Networks: mn})
	tc, err := svc.ResolveTenantContext(context.Background(), ResolveTenantParams{AccountID: accID})
	if err != nil {
		t.Fatal(err)
	}
	if tc.TenantID != tid.String() || tc.AccountID != accID || tc.NetworkID != "" || tc.Status != "active" {
		t.Fatalf("unexpected context: %+v", tc)
	}
}
