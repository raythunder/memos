package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	// UserIdentityProviderSupabase is the identity provider label for Supabase Auth.
	UserIdentityProviderSupabase = "supabase"
)

// UserExternalIdentity stores third-party identity mappings to local users.
type UserExternalIdentity struct {
	Provider  string
	Subject   string
	UserID    int32
	Email     string
	CreatedTs int64
	UpdatedTs int64
}

// FindUserExternalIdentity specifies filters for looking up external identities.
type FindUserExternalIdentity struct {
	Provider *string
	Subject  *string
	UserID   *int32
	Email    *string
}

func (s *Store) sqlPlaceholder(position int) string {
	if s.profile != nil && strings.EqualFold(s.profile.Driver, "postgres") {
		return fmt.Sprintf("$%d", position)
	}
	return "?"
}

// GetUserExternalIdentity returns one matching external identity mapping.
func (s *Store) GetUserExternalIdentity(ctx context.Context, find *FindUserExternalIdentity) (*UserExternalIdentity, error) {
	if find == nil {
		return nil, errors.New("find cannot be nil")
	}

	where, args := []string{"1 = 1"}, []any{}
	addFilter := func(column string, value any) {
		where = append(where, fmt.Sprintf("%s = %s", column, s.sqlPlaceholder(len(args)+1)))
		args = append(args, value)
	}

	if find.Provider != nil {
		addFilter("provider", *find.Provider)
	}
	if find.Subject != nil {
		addFilter("subject", *find.Subject)
	}
	if find.UserID != nil {
		addFilter("user_id", *find.UserID)
	}
	if find.Email != nil {
		addFilter("email", *find.Email)
	}

	query := `
SELECT provider, subject, user_id, email, created_ts, updated_ts
FROM user_external_identity
WHERE ` + strings.Join(where, " AND ") + `
LIMIT 1`

	identity := &UserExternalIdentity{}
	if err := s.driver.GetDB().QueryRowContext(ctx, query, args...).Scan(
		&identity.Provider,
		&identity.Subject,
		&identity.UserID,
		&identity.Email,
		&identity.CreatedTs,
		&identity.UpdatedTs,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "failed to get user external identity")
	}

	return identity, nil
}

// UpsertUserExternalIdentity creates or updates an external identity mapping.
func (s *Store) UpsertUserExternalIdentity(ctx context.Context, upsert *UserExternalIdentity) (*UserExternalIdentity, error) {
	if upsert == nil {
		return nil, errors.New("upsert cannot be nil")
	}
	upsert.Provider = strings.TrimSpace(upsert.Provider)
	upsert.Subject = strings.TrimSpace(upsert.Subject)
	upsert.Email = strings.TrimSpace(upsert.Email)
	if upsert.Provider == "" {
		return nil, errors.New("provider is required")
	}
	if upsert.Subject == "" {
		return nil, errors.New("subject is required")
	}
	if upsert.UserID <= 0 {
		return nil, errors.New("user_id is required")
	}

	now := time.Now().Unix()
	stmt := fmt.Sprintf(`
INSERT INTO user_external_identity (provider, subject, user_id, email, created_ts, updated_ts)
VALUES (%s, %s, %s, %s, %s, %s)
ON CONFLICT (provider, subject)
DO UPDATE SET
  user_id = EXCLUDED.user_id,
  email = EXCLUDED.email,
  updated_ts = EXCLUDED.updated_ts
RETURNING provider, subject, user_id, email, created_ts, updated_ts
`,
		s.sqlPlaceholder(1),
		s.sqlPlaceholder(2),
		s.sqlPlaceholder(3),
		s.sqlPlaceholder(4),
		s.sqlPlaceholder(5),
		s.sqlPlaceholder(6),
	)

	identity := &UserExternalIdentity{}
	if err := s.driver.GetDB().QueryRowContext(
		ctx,
		stmt,
		upsert.Provider,
		upsert.Subject,
		upsert.UserID,
		upsert.Email,
		now,
		now,
	).Scan(
		&identity.Provider,
		&identity.Subject,
		&identity.UserID,
		&identity.Email,
		&identity.CreatedTs,
		&identity.UpdatedTs,
	); err != nil {
		return nil, errors.Wrap(err, "failed to upsert user external identity")
	}

	return identity, nil
}
