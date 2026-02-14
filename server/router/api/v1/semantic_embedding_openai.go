package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const (
	openAIBaseURLEnv         = "MEMOS_OPENAI_BASE_URL"
	openAIAPIKeyEnv          = "MEMOS_OPENAI_API_KEY"
	openAIEmbeddingModelEnv  = "MEMOS_OPENAI_EMBEDDING_MODEL"
	openAIEmbeddingMaxRetryEnv = "MEMOS_OPENAI_EMBEDDING_MAX_RETRY"
	openAIEmbeddingBackoffMSEnv = "MEMOS_OPENAI_EMBEDDING_RETRY_BACKOFF_MS"
	defaultOpenAIBaseURL     = "https://api.openai.com/v1"
	defaultEmbeddingModel    = "text-embedding-3-small"
	openAIEmbeddingUserAgent = "memos-semantic-search/1.0"
	openAIEmbeddingMaxRetry  = 2
	openAIEmbeddingBackoff   = 100 * time.Millisecond
	openAIEmbeddingBodyLimit = 2 << 20
)

// SemanticEmbeddingClient abstracts semantic embedding generation.
// It allows test injection without external API dependency.
type SemanticEmbeddingClient interface {
	Embed(ctx context.Context, text string) ([]float64, error)
	Model() string
}

type openAIEmbeddingClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
	maxRetry   int
	backoff    time.Duration
}

type openAIEmbeddingConfig struct {
	baseURL string
	apiKey  string
	model   string
}

func newOpenAIEmbeddingClient(config *openAIEmbeddingConfig) (*openAIEmbeddingClient, error) {
	if config == nil {
		return nil, errors.New("openai config is required")
	}

	apiKey := strings.TrimSpace(config.apiKey)
	if apiKey == "" {
		return nil, errors.New("openai api key is not configured")
	}

	baseURL := normalizeOpenAIBaseURL(config.baseURL)

	model := strings.TrimSpace(config.model)
	if model == "" {
		model = defaultEmbeddingModel
	}
	maxRetry, backoff := resolveOpenAIEmbeddingRetryConfig()

	return &openAIEmbeddingClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxRetry: maxRetry,
		backoff:  backoff,
	}, nil
}

func normalizeOpenAIBaseURL(rawBaseURL string) string {
	baseURL := strings.TrimSpace(rawBaseURL)
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	if !strings.Contains(baseURL, "://") {
		baseURL = "https://" + baseURL
	}
	return strings.TrimRight(baseURL, "/")
}

func (s *APIV1Service) newOpenAIEmbeddingClient(ctx context.Context) (*openAIEmbeddingClient, error) {
	config, err := s.getOpenAIEmbeddingConfig(ctx)
	if err != nil {
		return nil, err
	}
	return newOpenAIEmbeddingClient(config)
}

func (s *APIV1Service) getSemanticEmbeddingClient(ctx context.Context) (SemanticEmbeddingClient, error) {
	if s.EmbeddingClientFactory != nil {
		return s.EmbeddingClientFactory(ctx)
	}
	return s.newOpenAIEmbeddingClient(ctx)
}

func (s *APIV1Service) getOpenAIEmbeddingConfig(ctx context.Context) (*openAIEmbeddingConfig, error) {
	aiSetting, err := s.Store.GetInstanceAISetting(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get instance ai setting")
	}

	config := &openAIEmbeddingConfig{
		baseURL: strings.TrimSpace(aiSetting.GetOpenaiBaseUrl()),
		model:   strings.TrimSpace(aiSetting.GetOpenaiEmbeddingModel()),
	}

	encryptedAPIKey := strings.TrimSpace(aiSetting.GetOpenaiApiKeyEncrypted())
	if encryptedAPIKey != "" {
		apiKey, err := decryptSensitiveValue(s.Secret, encryptedAPIKey)
		if err != nil {
			return nil, errors.Wrap(err, "failed to decrypt stored openai api key")
		}
		config.apiKey = strings.TrimSpace(apiKey)
	}

	// Backward compatibility: fallback to environment variables when UI setting is not configured.
	if config.baseURL == "" {
		config.baseURL = strings.TrimSpace(os.Getenv(openAIBaseURLEnv))
	}
	if config.model == "" {
		config.model = strings.TrimSpace(os.Getenv(openAIEmbeddingModelEnv))
	}
	if config.apiKey == "" {
		config.apiKey = strings.TrimSpace(os.Getenv(openAIAPIKeyEnv))
	}

	return config, nil
}

func (c *openAIEmbeddingClient) Embed(ctx context.Context, text string) ([]float64, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("embedding text cannot be empty")
	}

	requestBody := struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}{
		Model: c.model,
		Input: text,
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal openai embedding request")
	}

	for attempt := 0; ; attempt++ {
		embedding, retryable, err := c.embedOnce(ctx, body)
		if err == nil {
			return embedding, nil
		}
		if !retryable || attempt >= c.maxRetry {
			return nil, err
		}

		backoffDuration := c.backoff * time.Duration(1<<attempt)
		timer := time.NewTimer(backoffDuration)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, errors.Wrap(ctx.Err(), "openai embedding request canceled")
		case <-timer.C:
		}
	}
}

func (c *openAIEmbeddingClient) Model() string {
	return c.model
}

func (c *openAIEmbeddingClient) embedOnce(ctx context.Context, body []byte) ([]float64, bool, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to create openai embedding request")
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("User-Agent", openAIEmbeddingUserAgent)

	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return nil, false, errors.Wrap(ctx.Err(), "openai embedding request canceled")
		}
		return nil, true, errors.Wrap(err, "failed to call openai embedding api")
	}
	defer httpResponse.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(httpResponse.Body, openAIEmbeddingBodyLimit))
	if err != nil {
		return nil, true, errors.Wrap(err, "failed to read openai embedding response")
	}

	response := struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}{}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, isRetryableOpenAIStatus(httpResponse.StatusCode), errors.Wrap(err, "failed to decode openai embedding response")
	}

	if httpResponse.StatusCode >= http.StatusBadRequest {
		if response.Error != nil && response.Error.Message != "" {
			return nil, isRetryableOpenAIStatus(httpResponse.StatusCode), errors.Errorf("openai embedding request failed: %s", response.Error.Message)
		}
		return nil, isRetryableOpenAIStatus(httpResponse.StatusCode), errors.Errorf("openai embedding request failed with status %d", httpResponse.StatusCode)
	}
	if len(response.Data) == 0 || len(response.Data[0].Embedding) == 0 {
		return nil, false, errors.New("openai embedding response is empty")
	}

	return response.Data[0].Embedding, false, nil
}

func isRetryableOpenAIStatus(statusCode int) bool {
	if statusCode == http.StatusTooManyRequests || statusCode == http.StatusRequestTimeout {
		return true
	}
	return statusCode >= http.StatusInternalServerError
}

func resolveOpenAIEmbeddingRetryConfig() (int, time.Duration) {
	maxRetry := parseOpenAIEmbeddingMaxRetry(strings.TrimSpace(os.Getenv(openAIEmbeddingMaxRetryEnv)))
	backoff := parseOpenAIEmbeddingRetryBackoff(strings.TrimSpace(os.Getenv(openAIEmbeddingBackoffMSEnv)))
	return maxRetry, backoff
}

func parseOpenAIEmbeddingMaxRetry(raw string) int {
	if raw == "" {
		return openAIEmbeddingMaxRetry
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		slog.Warn("invalid OpenAI embedding max retry, fallback to default", "env", openAIEmbeddingMaxRetryEnv, "value", raw)
		return openAIEmbeddingMaxRetry
	}
	return value
}

func parseOpenAIEmbeddingRetryBackoff(raw string) time.Duration {
	if raw == "" {
		return openAIEmbeddingBackoff
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		slog.Warn("invalid OpenAI embedding retry backoff, fallback to default", "env", openAIEmbeddingBackoffMSEnv, "value", raw)
		return openAIEmbeddingBackoff
	}
	return time.Duration(value) * time.Millisecond
}
