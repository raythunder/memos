package v1

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"golang.org/x/sync/semaphore"

	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/plugin/markdown"
	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

const (
	semanticEmbeddingConcurrencyEnv    = "MEMOS_SEMANTIC_EMBEDDING_CONCURRENCY"
	defaultEmbeddingRefreshConcurrency = int64(8)
)

type APIV1Service struct {
	v1pb.UnimplementedInstanceServiceServer
	v1pb.UnimplementedAuthServiceServer
	v1pb.UnimplementedUserServiceServer
	v1pb.UnimplementedMemoServiceServer
	v1pb.UnimplementedAttachmentServiceServer
	v1pb.UnimplementedShortcutServiceServer
	v1pb.UnimplementedActivityServiceServer
	v1pb.UnimplementedIdentityProviderServiceServer

	Secret          string
	Profile         *profile.Profile
	Store           *store.Store
	MarkdownService markdown.Service
	// EmbeddingClientFactory overrides semantic embedding client creation.
	// Used by tests to avoid external API dependency.
	EmbeddingClientFactory func(ctx context.Context) (SemanticEmbeddingClient, error)

	// thumbnailSemaphore limits concurrent thumbnail generation to prevent memory exhaustion
	thumbnailSemaphore *semaphore.Weighted
	// embeddingSemaphore limits concurrent embedding refresh jobs to avoid unbounded goroutines.
	embeddingSemaphore *semaphore.Weighted
}

func NewAPIV1Service(secret string, profile *profile.Profile, store *store.Store) *APIV1Service {
	markdownService := markdown.NewService(
		markdown.WithTagExtension(),
	)
	embeddingConcurrency := resolveEmbeddingRefreshConcurrency(context.Background(), store)
	return &APIV1Service{
		Secret:             secret,
		Profile:            profile,
		Store:              store,
		MarkdownService:    markdownService,
		thumbnailSemaphore: semaphore.NewWeighted(3),                    // Limit to 3 concurrent thumbnail generations
		embeddingSemaphore: semaphore.NewWeighted(embeddingConcurrency), // Limit embedding refresh concurrency
	}
}

func resolveEmbeddingRefreshConcurrency(ctx context.Context, stores *store.Store) int64 {
	if stores != nil {
		aiSetting, err := stores.GetInstanceAISetting(ctx)
		if err != nil {
			slog.Warn("failed to load AI setting for embedding concurrency, fallback to env/default", "error", err)
		} else if aiSetting != nil && aiSetting.GetSemanticEmbeddingConcurrency() > 0 {
			return int64(aiSetting.GetSemanticEmbeddingConcurrency())
		}
	}
	return parseEmbeddingRefreshConcurrencyFromEnv(strings.TrimSpace(os.Getenv(semanticEmbeddingConcurrencyEnv)))
}

func parseEmbeddingRefreshConcurrencyFromEnv(raw string) int64 {
	if raw == "" {
		return defaultEmbeddingRefreshConcurrency
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		slog.Warn("invalid semantic embedding concurrency, fallback to default", "env", semanticEmbeddingConcurrencyEnv, "value", raw)
		return defaultEmbeddingRefreshConcurrency
	}
	return value
}

// RegisterGateway registers the gRPC-Gateway and Connect handlers with the given Echo instance.
func (s *APIV1Service) RegisterGateway(ctx context.Context, echoServer *echo.Echo) error {
	// Auth middleware for gRPC-Gateway - runs after routing, has access to method name.
	// Uses the same PublicMethods config as the Connect AuthInterceptor.
	authenticator := auth.NewAuthenticator(s.Store, s.Secret)
	gatewayAuthMiddleware := func(next runtime.HandlerFunc) runtime.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
			ctx := r.Context()

			// Get the RPC method name from context (set by grpc-gateway after routing)
			rpcMethod, ok := runtime.RPCMethod(ctx)

			// Extract credentials from HTTP headers
			authHeader := r.Header.Get("Authorization")

			result := authenticator.Authenticate(ctx, authHeader)

			// Enforce authentication for non-public methods
			// If rpcMethod cannot be determined, allow through, service layer will handle visibility checks
			if result == nil && ok && !IsPublicMethod(rpcMethod) {
				http.Error(w, `{"code": 16, "message": "authentication required"}`, http.StatusUnauthorized)
				return
			}

			// Set context based on auth result (may be nil for public endpoints)
			if result != nil {
				if result.Claims != nil {
					// Access Token V2 - stateless, use claims
					ctx = auth.SetUserClaimsInContext(ctx, result.Claims)
					ctx = context.WithValue(ctx, auth.UserIDContextKey, result.Claims.UserID)
				} else if result.User != nil {
					// PAT - have full user
					ctx = auth.SetUserInContext(ctx, result.User, result.AccessToken)
				}
				r = r.WithContext(ctx)
			}

			next(w, r, pathParams)
		}
	}

	// Create gRPC-Gateway mux with auth middleware.
	gwMux := runtime.NewServeMux(
		runtime.WithMiddlewares(gatewayAuthMiddleware),
	)
	if err := v1pb.RegisterInstanceServiceHandlerServer(ctx, gwMux, s); err != nil {
		return err
	}
	if err := v1pb.RegisterAuthServiceHandlerServer(ctx, gwMux, s); err != nil {
		return err
	}
	if err := v1pb.RegisterUserServiceHandlerServer(ctx, gwMux, s); err != nil {
		return err
	}
	if err := v1pb.RegisterMemoServiceHandlerServer(ctx, gwMux, s); err != nil {
		return err
	}
	if err := v1pb.RegisterAttachmentServiceHandlerServer(ctx, gwMux, s); err != nil {
		return err
	}
	if err := v1pb.RegisterShortcutServiceHandlerServer(ctx, gwMux, s); err != nil {
		return err
	}
	if err := v1pb.RegisterActivityServiceHandlerServer(ctx, gwMux, s); err != nil {
		return err
	}
	if err := v1pb.RegisterIdentityProviderServiceHandlerServer(ctx, gwMux, s); err != nil {
		return err
	}
	gwGroup := echoServer.Group("")
	gwGroup.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
	}))
	handler := echo.WrapHandler(gwMux)

	gwGroup.Any("/api/v1/*", handler)
	gwGroup.Any("/file/*", handler)

	// Connect handlers for browser clients (replaces grpc-web).
	logStacktraces := s.Profile.Demo
	connectInterceptors := connect.WithInterceptors(
		NewMetadataInterceptor(), // Convert HTTP headers to gRPC metadata first
		NewLoggingInterceptor(logStacktraces),
		NewRecoveryInterceptor(logStacktraces),
		NewAuthInterceptor(s.Store, s.Secret),
	)
	connectMux := http.NewServeMux()
	connectHandler := NewConnectServiceHandler(s)
	connectHandler.RegisterConnectHandlers(connectMux, connectInterceptors)

	// Wrap with CORS for browser access
	corsHandler := middleware.CORSWithConfig(middleware.CORSConfig{
		UnsafeAllowOriginFunc: func(_ *echo.Context, origin string) (string, bool, error) {
			return origin, true, nil
		},
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders:     []string{"*"},
		AllowCredentials: true,
	})
	connectGroup := echoServer.Group("", corsHandler)
	connectGroup.Any("/memos.api.v1.*", echo.WrapHandler(connectMux))

	return nil
}
