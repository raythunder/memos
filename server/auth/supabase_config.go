package auth

import (
	"net/url"
	"os"
	"strings"

	"github.com/pkg/errors"
)

const (
	supabaseProjectURLEnv   = "MEMOS_SUPABASE_PROJECT_URL"
	supabaseJWTAudienceEnv  = "MEMOS_SUPABASE_JWT_AUDIENCE"
	defaultSupabaseAudience = "authenticated"
)

// SupabaseConfig contains runtime configuration for Supabase JWT verification.
type SupabaseConfig struct {
	ProjectURL string
	Issuer     string
	JWKSURL    string
	Audience   string
}

// LoadSupabaseConfigFromEnv loads Supabase Auth settings from environment variables.
// Returns nil when Supabase integration is not configured.
func LoadSupabaseConfigFromEnv() (*SupabaseConfig, error) {
	projectURL := strings.TrimSpace(os.Getenv(supabaseProjectURLEnv))
	if projectURL == "" {
		return nil, nil
	}

	parsedURL, err := url.Parse(projectURL)
	if err != nil {
		return nil, errors.Wrap(err, "invalid Supabase project URL")
	}
	if parsedURL.Scheme != "https" {
		return nil, errors.New("supabase project URL must use https")
	}
	if parsedURL.Host == "" {
		return nil, errors.New("supabase project URL host is required")
	}

	projectURL = strings.TrimRight(parsedURL.String(), "/")
	issuer := projectURL + "/auth/v1"
	audience := strings.TrimSpace(os.Getenv(supabaseJWTAudienceEnv))
	if audience == "" {
		audience = defaultSupabaseAudience
	}

	return &SupabaseConfig{
		ProjectURL: projectURL,
		Issuer:     issuer,
		JWKSURL:    issuer + "/.well-known/jwks.json",
		Audience:   audience,
	}, nil
}
