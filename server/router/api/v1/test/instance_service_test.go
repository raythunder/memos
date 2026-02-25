package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	apiv1 "github.com/usememos/memos/server/router/api/v1"
)

func TestGetInstanceProfile(t *testing.T) {
	ctx := context.Background()

	t.Run("GetInstanceProfile returns instance profile", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Call GetInstanceProfile directly
		req := &v1pb.GetInstanceProfileRequest{}
		resp, err := ts.Service.GetInstanceProfile(ctx, req)

		// Verify response
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify the response contains expected data
		require.Equal(t, "test-1.0.0", resp.Version)
		require.True(t, resp.Demo)
		require.Equal(t, "http://localhost:8080", resp.InstanceUrl)

		// Instance should not be initialized since no admin users are created
		require.Nil(t, resp.Admin)
	})

	t.Run("GetInstanceProfile with initialized instance", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a host user in the store
		hostUser, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)
		require.NotNil(t, hostUser)

		// Call GetInstanceProfile directly
		req := &v1pb.GetInstanceProfileRequest{}
		resp, err := ts.Service.GetInstanceProfile(ctx, req)

		// Verify response
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify the response contains expected data with initialized flag
		require.Equal(t, "test-1.0.0", resp.Version)
		require.True(t, resp.Demo)
		require.Equal(t, "http://localhost:8080", resp.InstanceUrl)

		// Instance should be initialized since an admin user exists
		require.NotNil(t, resp.Admin)
		require.Equal(t, hostUser.Username, resp.Admin.Username)
	})
}

func TestGetInstanceProfile_Concurrency(t *testing.T) {
	ctx := context.Background()

	t.Run("Concurrent access to service", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a host user
		_, err := ts.CreateHostUser(ctx, "admin")
		require.NoError(t, err)

		// Make concurrent requests
		numGoroutines := 10
		results := make(chan *v1pb.InstanceProfile, numGoroutines)
		errors := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				req := &v1pb.GetInstanceProfileRequest{}
				resp, err := ts.Service.GetInstanceProfile(ctx, req)
				if err != nil {
					errors <- err
					return
				}
				results <- resp
			}()
		}

		// Collect all results
		for i := 0; i < numGoroutines; i++ {
			select {
			case err := <-errors:
				t.Fatalf("Goroutine returned error: %v", err)
			case resp := <-results:
				require.NotNil(t, resp)
				require.Equal(t, "test-1.0.0", resp.Version)
				require.True(t, resp.Demo)
				require.Equal(t, "http://localhost:8080", resp.InstanceUrl)
				require.NotNil(t, resp.Admin)
			}
		}
	})
}

func TestGetInstanceSetting(t *testing.T) {
	ctx := context.Background()

	t.Run("GetInstanceSetting - general setting", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Call GetInstanceSetting for general setting
		req := &v1pb.GetInstanceSettingRequest{
			Name: "instance/settings/GENERAL",
		}
		resp, err := ts.Service.GetInstanceSetting(ctx, req)

		// Verify response
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "instance/settings/GENERAL", resp.Name)

		// The general setting should have a general_setting field
		generalSetting := resp.GetGeneralSetting()
		require.NotNil(t, generalSetting)

		// General setting should have default values
		require.False(t, generalSetting.DisallowUserRegistration)
		require.False(t, generalSetting.DisallowPasswordAuth)
		require.Empty(t, generalSetting.AdditionalScript)
	})

	t.Run("GetInstanceSetting - storage setting", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a host user for storage setting access
		hostUser, err := ts.CreateHostUser(ctx, "testhost")
		require.NoError(t, err)

		// Add user to context
		userCtx := ts.CreateUserContext(ctx, hostUser.ID)

		// Call GetInstanceSetting for storage setting
		req := &v1pb.GetInstanceSettingRequest{
			Name: "instance/settings/STORAGE",
		}
		resp, err := ts.Service.GetInstanceSetting(userCtx, req)

		// Verify response
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "instance/settings/STORAGE", resp.Name)

		// The storage setting should have a storage_setting field
		storageSetting := resp.GetStorageSetting()
		require.NotNil(t, storageSetting)
	})

	t.Run("GetInstanceSetting - memo related setting", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Call GetInstanceSetting for memo related setting
		req := &v1pb.GetInstanceSettingRequest{
			Name: "instance/settings/MEMO_RELATED",
		}
		resp, err := ts.Service.GetInstanceSetting(ctx, req)

		// Verify response
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "instance/settings/MEMO_RELATED", resp.Name)

		// The memo related setting should have a memo_related_setting field
		memoRelatedSetting := resp.GetMemoRelatedSetting()
		require.NotNil(t, memoRelatedSetting)
	})

	t.Run("GetInstanceSetting - invalid setting name", func(t *testing.T) {
		// Create test service for this specific test
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Call GetInstanceSetting with invalid name
		req := &v1pb.GetInstanceSettingRequest{
			Name: "invalid/setting/name",
		}
		_, err := ts.Service.GetInstanceSetting(ctx, req)

		// Should return an error
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid instance setting name")
	})

	t.Run("GetInstanceSetting - ai setting requires admin", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		req := &v1pb.GetInstanceSettingRequest{
			Name: "instance/settings/AI",
		}
		_, err := ts.Service.GetInstanceSetting(ctx, req)

		require.Error(t, err)
		require.Contains(t, err.Error(), "user not authenticated")
	})

	t.Run("GetInstanceSetting - ai setting for admin hides api key", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		hostUser, err := ts.CreateHostUser(ctx, "ai-admin")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, hostUser.ID)

		req := &v1pb.GetInstanceSettingRequest{
			Name: "instance/settings/AI",
		}
		resp, err := ts.Service.GetInstanceSetting(userCtx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "instance/settings/AI", resp.Name)
		require.NotNil(t, resp.GetAiSetting())
		require.False(t, resp.GetAiSetting().GetOpenaiApiKeySet())
		require.Empty(t, resp.GetAiSetting().GetOpenaiApiKey())
	})
}

func TestUpdateInstanceAISetting(t *testing.T) {
	ctx := context.Background()

	t.Run("Update ai setting encrypts key and hides plaintext", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		hostUser, err := ts.CreateHostUser(ctx, "ai-admin")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, hostUser.ID)

		updateReq := &v1pb.UpdateInstanceSettingRequest{
			Setting: &v1pb.InstanceSetting{
				Name: "instance/settings/AI",
				Value: &v1pb.InstanceSetting_AiSetting{
					AiSetting: &v1pb.InstanceSetting_AISetting{
						OpenaiBaseUrl:                 "https://api.openai.com/v1",
						OpenaiEmbeddingModel:          "text-embedding-3-small",
						OpenaiEmbeddingModels:         []string{"text-embedding-3-small", "jina-embeddings-v4"},
						OpenaiApiKey:                  "sk-test-secret",
						OpenaiEmbeddingMaxRetry:       3,
						OpenaiEmbeddingRetryBackoffMs: 150,
						SemanticEmbeddingConcurrency:  10,
					},
				},
			},
		}
		resp, err := ts.Service.UpdateInstanceSetting(userCtx, updateReq)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.GetAiSetting())
		require.True(t, resp.GetAiSetting().GetOpenaiApiKeySet())
		require.Empty(t, resp.GetAiSetting().GetOpenaiApiKey())

		aiSetting, err := ts.Store.GetInstanceAISetting(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, aiSetting.GetOpenaiApiKeyEncrypted())
		require.NotEqual(t, "sk-test-secret", aiSetting.GetOpenaiApiKeyEncrypted())
		require.Equal(t, int32(3), aiSetting.GetOpenaiEmbeddingMaxRetry())
		require.Equal(t, int32(150), aiSetting.GetOpenaiEmbeddingRetryBackoffMs())
		require.Equal(t, int32(10), aiSetting.GetSemanticEmbeddingConcurrency())
		require.Equal(t, []string{"text-embedding-3-small", "jina-embeddings-v4"}, aiSetting.GetOpenaiEmbeddingModels())

		// Updating base/model without api key should keep stored ciphertext.
		prevCipherText := aiSetting.GetOpenaiApiKeyEncrypted()
		updateReq.Setting.GetAiSetting().OpenaiApiKey = ""
		updateReq.Setting.GetAiSetting().OpenaiEmbeddingModel = "text-embedding-3-large"
		_, err = ts.Service.UpdateInstanceSetting(userCtx, updateReq)
		require.NoError(t, err)

		aiSetting, err = ts.Store.GetInstanceAISetting(ctx)
		require.NoError(t, err)
		require.Equal(t, prevCipherText, aiSetting.GetOpenaiApiKeyEncrypted())
		require.Equal(t, "text-embedding-3-large", aiSetting.GetOpenaiEmbeddingModel())
		require.Equal(t, int32(3), aiSetting.GetOpenaiEmbeddingMaxRetry())
		require.Equal(t, int32(150), aiSetting.GetOpenaiEmbeddingRetryBackoffMs())
		require.Equal(t, int32(10), aiSetting.GetSemanticEmbeddingConcurrency())
		require.Equal(t, []string{"text-embedding-3-large", "text-embedding-3-small", "jina-embeddings-v4"}, aiSetting.GetOpenaiEmbeddingModels())

		// Clearing api key should remove ciphertext.
		updateReq.Setting.GetAiSetting().ClearOpenaiApiKey = true
		_, err = ts.Service.UpdateInstanceSetting(userCtx, updateReq)
		require.NoError(t, err)

		aiSetting, err = ts.Store.GetInstanceAISetting(ctx)
		require.NoError(t, err)
		require.Empty(t, aiSetting.GetOpenaiApiKeyEncrypted())

		// Negative numeric values should be rejected with invalid argument.
		updateReq.Setting.GetAiSetting().OpenaiEmbeddingMaxRetry = -1
		updateReq.Setting.GetAiSetting().OpenaiEmbeddingRetryBackoffMs = -200
		updateReq.Setting.GetAiSetting().SemanticEmbeddingConcurrency = -8
		_, err = ts.Service.UpdateInstanceSetting(userCtx, updateReq)
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.InvalidArgument, st.Code())

		aiSetting, err = ts.Store.GetInstanceAISetting(ctx)
		require.NoError(t, err)
		require.Equal(t, int32(3), aiSetting.GetOpenaiEmbeddingMaxRetry())
		require.Equal(t, int32(150), aiSetting.GetOpenaiEmbeddingRetryBackoffMs())
		require.Equal(t, int32(10), aiSetting.GetSemanticEmbeddingConcurrency())
	})

	t.Run("Trigger semantic reindex persists progress state", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		hostUser, err := ts.CreateHostUser(ctx, "reindex-admin")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, hostUser.ID)

		// Semantic reindex requires postgres profile. Use a fake embedding client
		// to avoid external dependencies during tests.
		ts.Service.Profile.Driver = "postgres"
		ts.Service.EmbeddingClientFactory = func(context.Context) (apiv1.SemanticEmbeddingClient, error) {
			return &fakeSemanticEmbeddingClient{
				model: "jina-embeddings-v4",
			}, nil
		}

		updateReq := &v1pb.UpdateInstanceSettingRequest{
			Setting: &v1pb.InstanceSetting{
				Name: "instance/settings/AI",
				Value: &v1pb.InstanceSetting_AiSetting{
					AiSetting: &v1pb.InstanceSetting_AISetting{
						OpenaiBaseUrl:            "https://api.jina.ai/v1",
						OpenaiEmbeddingModel:     "jina-embeddings-v4",
						TriggerSemanticReindex:   true,
						OpenaiEmbeddingModels:    []string{"jina-embeddings-v4"},
						SemanticReindexRunning:   false, // client writes should not control runtime state
						SemanticReindexTotal:     0,
						SemanticReindexProcessed: 0,
					},
				},
			},
		}
		_, err = ts.Service.UpdateInstanceSetting(userCtx, updateReq)
		require.NoError(t, err)

		// Poll until background reindex updates state and finishes.
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			aiSetting, getErr := ts.Store.GetInstanceAISetting(ctx)
			require.NoError(t, getErr)
			if aiSetting.GetSemanticReindexStartedTs() > 0 && !aiSetting.GetSemanticReindexRunning() {
				require.Equal(t, "jina-embeddings-v4", aiSetting.GetSemanticReindexModel())
				require.Greater(t, aiSetting.GetSemanticReindexUpdatedTs(), int64(0))
				return
			}
			time.Sleep(50 * time.Millisecond)
		}

		t.Fatal("semantic reindex state was not updated in time")
	})
}
