package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/usememos/memos/internal/util"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

// Authenticator provides shared authentication and authorization logic.
// Used by gRPC interceptor, Connect interceptor, and file server to ensure
// consistent authentication behavior across all API endpoints.
//
// Authentication methods:
// - JWT access tokens: Short-lived tokens (15 minutes) for API access
// - Personal Access Tokens (PAT): Long-lived tokens for programmatic access
//
// This struct is safe for concurrent use.
type Authenticator struct {
	store               *store.Store
	secret              string
	supabaseJWTVerifier *SupabaseJWTValidator
	supabaseProvisionMu sync.Mutex
}

// NewAuthenticator creates a new Authenticator instance.
func NewAuthenticator(stores *store.Store, secret string) *Authenticator {
	authenticator := &Authenticator{
		store:  stores,
		secret: secret,
	}

	// Supabase JWT support is only enabled for PostgreSQL runtime.
	if stores != nil && strings.EqualFold(stores.DriverName(), "postgres") {
		config, err := LoadSupabaseConfigFromEnv()
		if err != nil {
			slog.Warn("invalid supabase auth config, fallback to memos auth only", "error", err)
		} else if config != nil {
			authenticator.supabaseJWTVerifier = NewSupabaseJWTValidator(config)
		}
	}

	return authenticator
}

// AuthenticateByAccessTokenV2 validates a short-lived access token.
// Returns claims without database query (stateless validation).
func (a *Authenticator) AuthenticateByAccessTokenV2(accessToken string) (*UserClaims, error) {
	claims, err := ParseAccessTokenV2(accessToken, []byte(a.secret))
	if err != nil {
		return nil, errors.Wrap(err, "invalid access token")
	}

	userID, err := util.ConvertStringToInt32(claims.Subject)
	if err != nil {
		return nil, errors.Wrap(err, "invalid user ID in token")
	}

	return &UserClaims{
		UserID:   userID,
		Username: claims.Username,
		Role:     claims.Role,
		Status:   claims.Status,
	}, nil
}

// AuthenticateByRefreshToken validates a refresh token against the database.
func (a *Authenticator) AuthenticateByRefreshToken(ctx context.Context, refreshToken string) (*store.User, string, error) {
	claims, err := ParseRefreshToken(refreshToken, []byte(a.secret))
	if err != nil {
		return nil, "", errors.Wrap(err, "invalid refresh token")
	}

	userID, err := util.ConvertStringToInt32(claims.Subject)
	if err != nil {
		return nil, "", errors.Wrap(err, "invalid user ID in token")
	}

	// Check token exists in database (revocation check)
	token, err := a.store.GetUserRefreshTokenByID(ctx, userID, claims.TokenID)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to get refresh token")
	}
	if token == nil {
		return nil, "", errors.New("refresh token revoked")
	}

	// Check token not expired
	if token.ExpiresAt != nil && token.ExpiresAt.AsTime().Before(time.Now()) {
		return nil, "", errors.New("refresh token expired")
	}

	// Get user
	user, err := a.store.GetUser(ctx, &store.FindUser{ID: &userID})
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to get user")
	}
	if user == nil {
		return nil, "", errors.New("user not found")
	}
	if user.RowStatus == store.Archived {
		return nil, "", errors.New("user is archived")
	}

	return user, claims.TokenID, nil
}

// AuthenticateByPAT validates a Personal Access Token.
func (a *Authenticator) AuthenticateByPAT(ctx context.Context, token string) (*store.User, *storepb.PersonalAccessTokensUserSetting_PersonalAccessToken, error) {
	if !strings.HasPrefix(token, PersonalAccessTokenPrefix) {
		return nil, nil, errors.New("invalid PAT format")
	}

	tokenHash := HashPersonalAccessToken(token)
	result, err := a.store.GetUserByPATHash(ctx, tokenHash)
	if err != nil {
		return nil, nil, errors.Wrap(err, "invalid PAT")
	}

	// Check expiry
	if result.PAT.ExpiresAt != nil && result.PAT.ExpiresAt.AsTime().Before(time.Now()) {
		return nil, nil, errors.New("PAT expired")
	}

	// Check user status
	if result.User.RowStatus == store.Archived {
		return nil, nil, errors.New("user is archived")
	}

	return result.User, result.PAT, nil
}

// AuthenticateBySupabaseToken validates a Supabase JWT and resolves it to a local Memos user.
func (a *Authenticator) AuthenticateBySupabaseToken(ctx context.Context, token string) (*store.User, error) {
	if a.supabaseJWTVerifier == nil {
		return nil, errors.New("supabase auth is not configured")
	}

	claims, err := a.supabaseJWTVerifier.ValidateToken(ctx, token)
	if err != nil {
		return nil, err
	}

	return a.resolveLocalUserFromSupabaseClaims(ctx, claims)
}

// AuthResult contains the result of an authentication attempt.
type AuthResult struct {
	User        *store.User // Set for PAT authentication
	Claims      *UserClaims // Set for Access Token V2 (stateless)
	AccessToken string      // Non-empty if authenticated via JWT
}

// Authenticate tries to authenticate using the provided credentials.
// Priority: 1. Access Token V2, 2. PAT, 3. Supabase JWT
// Returns nil if no valid credentials are provided.
func (a *Authenticator) Authenticate(ctx context.Context, authHeader string) *AuthResult {
	token := ExtractBearerToken(authHeader)

	// Try Access Token V2 (stateless)
	if token != "" && !strings.HasPrefix(token, PersonalAccessTokenPrefix) {
		claims, err := a.AuthenticateByAccessTokenV2(token)
		if err == nil && claims != nil {
			return &AuthResult{
				Claims:      claims,
				AccessToken: token,
			}
		}
	}

	// Try PAT
	if token != "" && strings.HasPrefix(token, PersonalAccessTokenPrefix) {
		user, pat, err := a.AuthenticateByPAT(ctx, token)
		if err == nil && user != nil {
			// Update last used (fire-and-forget with logging)
			go func() {
				if err := a.store.UpdatePATLastUsed(context.Background(), user.ID, pat.TokenId, timestamppb.Now()); err != nil {
					slog.Warn("failed to update PAT last used time", "error", err, "userID", user.ID)
				}
			}()
			return &AuthResult{User: user, AccessToken: token}
		}
	}

	// Try Supabase JWT.
	if token != "" && !strings.HasPrefix(token, PersonalAccessTokenPrefix) {
		user, err := a.AuthenticateBySupabaseToken(ctx, token)
		if err == nil && user != nil {
			return &AuthResult{
				Claims: &UserClaims{
					UserID:   user.ID,
					Username: user.Username,
					Role:     string(user.Role),
					Status:   string(user.RowStatus),
				},
				AccessToken: token,
			}
		}
	}

	return nil
}

func (a *Authenticator) resolveLocalUserFromSupabaseClaims(ctx context.Context, claims *SupabaseJWTClaims) (*store.User, error) {
	subject := strings.TrimSpace(claims.Subject)
	email := strings.TrimSpace(claims.Email)
	provider := store.UserIdentityProviderSupabase

	mapping, err := a.store.GetUserExternalIdentity(ctx, &store.FindUserExternalIdentity{
		Provider: &provider,
		Subject:  &subject,
	})
	if err != nil {
		return nil, err
	}
	if mapping != nil {
		userID := mapping.UserID
		user, err := a.store.GetUser(ctx, &store.FindUser{ID: &userID})
		if err != nil {
			return nil, err
		}
		if user == nil {
			return nil, errors.New("linked user does not exist")
		}
		if user.RowStatus == store.Archived {
			return nil, errors.New("user is archived")
		}
		if mapping.Email != email {
			_, _ = a.store.UpsertUserExternalIdentity(ctx, &store.UserExternalIdentity{
				Provider: provider,
				Subject:  subject,
				UserID:   user.ID,
				Email:    email,
			})
		}
		return user, nil
	}

	// Serialize first-time provisioning on this authenticator instance, then re-check.
	a.supabaseProvisionMu.Lock()
	defer a.supabaseProvisionMu.Unlock()

	mapping, err = a.store.GetUserExternalIdentity(ctx, &store.FindUserExternalIdentity{
		Provider: &provider,
		Subject:  &subject,
	})
	if err != nil {
		return nil, err
	}
	if mapping != nil {
		userID := mapping.UserID
		user, err := a.store.GetUser(ctx, &store.FindUser{ID: &userID})
		if err != nil {
			return nil, err
		}
		if user == nil {
			return nil, errors.New("linked user does not exist")
		}
		if user.RowStatus == store.Archived {
			return nil, errors.New("user is archived")
		}
		return user, nil
	}

	var user *store.User
	if email != "" {
		users, err := a.store.ListUsers(ctx, &store.FindUser{Email: &email})
		if err != nil {
			return nil, err
		}
		if len(users) == 1 {
			user = users[0]
			if user.RowStatus == store.Archived {
				return nil, errors.New("user is archived")
			}
		}
	}

	if user == nil {
		user, err = a.createSupabaseBackedUser(ctx, claims)
		if err != nil {
			return nil, err
		}
	}

	if _, err := a.store.UpsertUserExternalIdentity(ctx, &store.UserExternalIdentity{
		Provider: provider,
		Subject:  subject,
		UserID:   user.ID,
		Email:    email,
	}); err != nil {
		return nil, err
	}
	return user, nil
}

func (a *Authenticator) createSupabaseBackedUser(ctx context.Context, claims *SupabaseJWTClaims) (*store.User, error) {
	limitOne := 1
	userList, err := a.store.ListUsers(ctx, &store.FindUser{Limit: &limitOne})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list users")
	}

	role := store.RoleUser
	if len(userList) == 0 {
		role = store.RoleAdmin
	} else {
		setting, err := a.store.GetInstanceGeneralSetting(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get instance general setting")
		}
		if setting.DisallowUserRegistration {
			return nil, errors.New("user registration is not allowed")
		}
	}

	username, err := a.generateUniqueSupabaseUsername(ctx, claims.Subject)
	if err != nil {
		return nil, err
	}

	passwordSeed, err := util.RandomString(24)
	if err != nil {
		passwordSeed = util.GenUUID()
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(passwordSeed), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate password hash")
	}

	user, err := a.store.CreateUser(ctx, &store.User{
		Username:     username,
		Role:         role,
		Email:        strings.TrimSpace(claims.Email),
		Nickname:     deriveSupabaseNickname(claims.Email),
		PasswordHash: string(passwordHash),
	})
	if err != nil {
		// Handle race between multiple auth requests using same external identity.
		existingUser, lookupErr := a.store.GetUser(ctx, &store.FindUser{Username: &username})
		if lookupErr == nil && existingUser != nil {
			return existingUser, nil
		}
		return nil, errors.Wrap(err, "failed to create user")
	}

	return user, nil
}

func (a *Authenticator) generateUniqueSupabaseUsername(ctx context.Context, subject string) (string, error) {
	for i := 0; i < 8; i++ {
		seed := subject
		if i > 0 {
			seed = fmt.Sprintf("%s:%d", subject, i)
		}
		sum := sha256.Sum256([]byte(seed))
		candidate := hex.EncodeToString(sum[:])[:32]

		existing, err := a.store.GetUser(ctx, &store.FindUser{Username: &candidate})
		if err != nil {
			return "", err
		}
		if existing == nil {
			return candidate, nil
		}
	}
	return "", errors.New("failed to generate unique username for supabase user")
}

func deriveSupabaseNickname(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return "Supabase User"
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "Supabase User"
	}
	return strings.TrimSpace(parts[0])
}
