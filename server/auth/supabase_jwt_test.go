package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
	teststore "github.com/usememos/memos/store/test"
)

func TestSupabaseJWTValidator_ValidToken(t *testing.T) {
	t.Parallel()

	issuer, validator, signer := newSupabaseValidatorFixture(t)
	token := signer.signToken(t, tokenSignInput{
		issuer:   issuer,
		audience: "authenticated",
		subject:  "supabase-sub-1",
		email:    "alice@example.com",
		kid:      signer.currentKID(),
	})

	claims, err := validator.ValidateToken(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, "supabase-sub-1", claims.Subject)
	require.Equal(t, "alice@example.com", claims.Email)
}

func TestSupabaseJWTValidator_RejectsWrongAudience(t *testing.T) {
	t.Parallel()

	issuer, validator, signer := newSupabaseValidatorFixture(t)
	token := signer.signToken(t, tokenSignInput{
		issuer:   issuer,
		audience: "wrong-audience",
		subject:  "supabase-sub-2",
		email:    "bob@example.com",
		kid:      signer.currentKID(),
	})

	_, err := validator.ValidateToken(context.Background(), token)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid supabase token")
}

func TestSupabaseJWTValidator_RefreshesOnKIDMiss(t *testing.T) {
	t.Parallel()

	issuer, validator, signer := newSupabaseValidatorFixture(t)
	firstToken := signer.signToken(t, tokenSignInput{
		issuer:   issuer,
		audience: "authenticated",
		subject:  "supabase-sub-3",
		email:    "first@example.com",
		kid:      signer.currentKID(),
	})
	_, err := validator.ValidateToken(context.Background(), firstToken)
	require.NoError(t, err)

	newKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer.rotate("kid-rotated", newKey)

	secondToken := signer.signToken(t, tokenSignInput{
		issuer:   issuer,
		audience: "authenticated",
		subject:  "supabase-sub-4",
		email:    "second@example.com",
		kid:      "kid-rotated",
	})
	claims, err := validator.ValidateToken(context.Background(), secondToken)
	require.NoError(t, err)
	require.Equal(t, "supabase-sub-4", claims.Subject)
}

func TestAuthenticator_AuthenticateBySupabaseToken_AutoCreatesAndBinds(t *testing.T) {
	ctx := context.Background()
	stores := teststore.NewTestingStore(ctx, t)
	t.Cleanup(func() {
		_ = stores.Close()
	})
	ensureExternalIdentityTable(t, stores)

	issuer, validator, signer := newSupabaseValidatorFixture(t)
	authenticator := NewAuthenticator(stores, "test-secret")
	authenticator.supabaseJWTVerifier = validator

	token := signer.signToken(t, tokenSignInput{
		issuer:   issuer,
		audience: "authenticated",
		subject:  "supabase-sub-auto-create",
		email:    "first-admin@example.com",
		kid:      signer.currentKID(),
	})

	user, err := authenticator.AuthenticateBySupabaseToken(ctx, token)
	require.NoError(t, err)
	require.Equal(t, store.RoleAdmin, user.Role)

	provider := store.UserIdentityProviderSupabase
	subject := "supabase-sub-auto-create"
	mapping, err := stores.GetUserExternalIdentity(ctx, &store.FindUserExternalIdentity{
		Provider: &provider,
		Subject:  &subject,
	})
	require.NoError(t, err)
	require.NotNil(t, mapping)
	require.Equal(t, user.ID, mapping.UserID)

	userAgain, err := authenticator.AuthenticateBySupabaseToken(ctx, token)
	require.NoError(t, err)
	require.Equal(t, user.ID, userAgain.ID)

	users, err := stores.ListUsers(ctx, &store.FindUser{})
	require.NoError(t, err)
	require.Len(t, users, 1)
}

func TestAuthenticator_AuthenticateBySupabaseToken_RespectsRegistrationSetting(t *testing.T) {
	ctx := context.Background()
	stores := teststore.NewTestingStore(ctx, t)
	t.Cleanup(func() {
		_ = stores.Close()
	})
	ensureExternalIdentityTable(t, stores)

	_, err := stores.CreateUser(ctx, &store.User{
		Username:     "existing-admin",
		Role:         store.RoleAdmin,
		Email:        "existing-admin@example.com",
		Nickname:     "Existing Admin",
		PasswordHash: "dummy",
	})
	require.NoError(t, err)

	_, err = stores.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_GENERAL,
		Value: &storepb.InstanceSetting_GeneralSetting{
			GeneralSetting: &storepb.InstanceGeneralSetting{
				DisallowUserRegistration: true,
			},
		},
	})
	require.NoError(t, err)

	issuer, validator, signer := newSupabaseValidatorFixture(t)
	authenticator := NewAuthenticator(stores, "test-secret")
	authenticator.supabaseJWTVerifier = validator

	token := signer.signToken(t, tokenSignInput{
		issuer:   issuer,
		audience: "authenticated",
		subject:  "supabase-sub-new-user",
		email:    "new-user@example.com",
		kid:      signer.currentKID(),
	})

	_, err = authenticator.AuthenticateBySupabaseToken(ctx, token)
	require.Error(t, err)
	require.Contains(t, err.Error(), "user registration is not allowed")
}

func ensureExternalIdentityTable(t *testing.T, stores *store.Store) {
	t.Helper()
	_, err := stores.GetDriver().GetDB().Exec(`
CREATE TABLE IF NOT EXISTS user_external_identity (
  provider TEXT NOT NULL,
  subject TEXT NOT NULL,
  user_id INTEGER NOT NULL,
  email TEXT NOT NULL DEFAULT '',
  created_ts BIGINT NOT NULL DEFAULT 0,
  updated_ts BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (provider, subject)
)`)
	require.NoError(t, err)
}

type tokenSignInput struct {
	issuer   string
	audience string
	subject  string
	email    string
	kid      string
}

type jwksSigner struct {
	mu     sync.RWMutex
	kid    string
	signer *rsa.PrivateKey
}

func (s *jwksSigner) currentKID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.kid
}

func (s *jwksSigner) rotate(kid string, signer *rsa.PrivateKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kid = kid
	s.signer = signer
}

func (s *jwksSigner) signToken(t *testing.T, input tokenSignInput) string {
	t.Helper()

	s.mu.RLock()
	kid := s.kid
	privateKey := s.signer
	s.mu.RUnlock()
	if input.kid != "" {
		kid = input.kid
	}

	now := time.Now()
	claims := &SupabaseJWTClaims{
		Email: input.email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    input.issuer,
			Audience:  jwt.ClaimStrings{input.audience},
			Subject:   input.subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(privateKey)
	require.NoError(t, err)
	return signed
}

func (s *jwksSigner) jwksHandler(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	publicKey := s.signer.PublicKey
	jwks := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"kid": s.kid,
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(bigEndianBytes(publicKey.E)),
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jwks)
}

func newSupabaseValidatorFixture(t *testing.T) (string, *SupabaseJWTValidator, *jwksSigner) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer := &jwksSigner{
		kid:    "kid-initial",
		signer: privateKey,
	}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.HandleFunc("/auth/v1/.well-known/jwks.json", signer.jwksHandler)
	issuer := strings.TrimRight(server.URL, "/") + "/auth/v1"

	validator := NewSupabaseJWTValidator(&SupabaseConfig{
		ProjectURL: strings.TrimRight(server.URL, "/"),
		Issuer:     issuer,
		JWKSURL:    issuer + "/.well-known/jwks.json",
		Audience:   "authenticated",
	})
	require.NotNil(t, validator)
	return issuer, validator, signer
}

func bigEndianBytes(v int) []byte {
	if v == 0 {
		return []byte{0}
	}
	out := make([]byte, 0, 4)
	for v > 0 {
		out = append([]byte{byte(v & 0xff)}, out...)
		v >>= 8
	}
	return out
}

func TestLoadSupabaseConfigFromEnv(t *testing.T) {
	t.Setenv(supabaseProjectURLEnv, "https://example.supabase.co")
	t.Setenv(supabaseJWTAudienceEnv, "")

	cfg, err := LoadSupabaseConfigFromEnv()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, "https://example.supabase.co", cfg.ProjectURL)
	require.Equal(t, "https://example.supabase.co/auth/v1", cfg.Issuer)
	require.Equal(t, "authenticated", cfg.Audience)
	require.Equal(t, fmt.Sprintf("%s/.well-known/jwks.json", cfg.Issuer), cfg.JWKSURL)
}
