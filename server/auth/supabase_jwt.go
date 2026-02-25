package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
)

const supabaseJWKSCacheTTL = 10 * time.Minute

type supabaseJWKSResponse struct {
	Keys []supabaseJWK `json:"keys"`
}

type supabaseJWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// SupabaseJWTClaims represents the subset of Supabase claims needed by Memos.
type SupabaseJWTClaims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// SupabaseJWTValidator verifies Supabase JWT tokens against the project's JWKS endpoint.
type SupabaseJWTValidator struct {
	config     *SupabaseConfig
	httpClient *http.Client

	mu        sync.RWMutex
	publicKey map[string]*rsa.PublicKey
	expiresAt time.Time
}

// NewSupabaseJWTValidator creates a Supabase JWT validator.
func NewSupabaseJWTValidator(config *SupabaseConfig) *SupabaseJWTValidator {
	if config == nil {
		return nil
	}
	return &SupabaseJWTValidator{
		config: config,
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
		publicKey: make(map[string]*rsa.PublicKey),
	}
}

// ValidateToken validates a Supabase JWT and returns parsed claims.
func (v *SupabaseJWTValidator) ValidateToken(ctx context.Context, tokenString string) (*SupabaseJWTClaims, error) {
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, errors.New("token is required")
	}

	claims := &SupabaseJWTClaims{}
	_, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		v.keyFunc(ctx),
		jwt.WithIssuer(v.config.Issuer),
		jwt.WithAudience(v.config.Audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "invalid supabase token")
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return nil, errors.New("supabase token subject is required")
	}
	return claims, nil
}

func (v *SupabaseJWTValidator) keyFunc(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, errors.Errorf("unexpected signing method: %s", token.Method.Alg())
		}

		kid, _ := token.Header["kid"].(string)
		if strings.TrimSpace(kid) == "" {
			return nil, errors.New("missing kid in token header")
		}

		key, err := v.getPublicKey(ctx, kid, false)
		if err != nil {
			return nil, err
		}
		if key != nil {
			return key, nil
		}

		// Force refresh once when kid is not found in cache.
		key, err = v.getPublicKey(ctx, kid, true)
		if err != nil {
			return nil, err
		}
		if key == nil {
			return nil, errors.Errorf("no matching signing key for kid: %s", kid)
		}

		return key, nil
	}
}

func (v *SupabaseJWTValidator) getPublicKey(ctx context.Context, kid string, forceRefresh bool) (*rsa.PublicKey, error) {
	if !forceRefresh {
		v.mu.RLock()
		cacheValid := time.Now().Before(v.expiresAt)
		if cacheValid {
			if key := v.publicKey[kid]; key != nil {
				v.mu.RUnlock()
				return key, nil
			}
		}
		v.mu.RUnlock()
	}

	if err := v.refreshJWKS(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.publicKey[kid], nil
}

func (v *SupabaseJWTValidator) refreshJWKS(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.config.JWKSURL, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create JWKS request")
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to fetch JWKS")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to fetch JWKS: status %d", resp.StatusCode)
	}

	var jwks supabaseJWKSResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return errors.Wrap(err, "failed to decode JWKS response")
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, key := range jwks.Keys {
		if strings.TrimSpace(key.Kid) == "" {
			continue
		}
		publicKey, err := parseRSAPublicKey(key)
		if err != nil {
			continue
		}
		keys[key.Kid] = publicKey
	}
	if len(keys) == 0 {
		return errors.New("no RSA public keys found in JWKS")
	}

	v.mu.Lock()
	v.publicKey = keys
	v.expiresAt = time.Now().Add(supabaseJWKSCacheTTL)
	v.mu.Unlock()
	return nil
}

func parseRSAPublicKey(jwk supabaseJWK) (*rsa.PublicKey, error) {
	if !strings.EqualFold(jwk.Kty, "RSA") {
		return nil, errors.New("unsupported jwk key type")
	}
	if strings.TrimSpace(jwk.N) == "" || strings.TrimSpace(jwk.E) == "" {
		return nil, errors.New("invalid RSA key parameters")
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, errors.Wrap(err, "invalid RSA modulus")
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, errors.Wrap(err, "invalid RSA exponent")
	}
	if len(eBytes) == 0 {
		return nil, errors.New("empty RSA exponent")
	}

	eBig := new(big.Int).SetBytes(eBytes)
	if !eBig.IsInt64() {
		return nil, errors.New("RSA exponent overflow")
	}
	exponent := int(eBig.Int64())
	if exponent <= 0 {
		return nil, errors.New("invalid RSA exponent")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: exponent,
	}, nil
}
