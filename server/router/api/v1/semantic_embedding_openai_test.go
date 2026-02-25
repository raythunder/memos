package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIBaseURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty uses default",
			input:    "",
			expected: defaultOpenAIBaseURL,
		},
		{
			name:     "trims surrounding spaces",
			input:    "  https://api.openai.com/v1  ",
			expected: "https://api.openai.com/v1",
		},
		{
			name:     "adds https scheme for host path input",
			input:    "api.v3.cm/v1",
			expected: "https://api.v3.cm/v1",
		},
		{
			name:     "keeps explicit http scheme",
			input:    "http://localhost:11434/v1/",
			expected: "http://localhost:11434/v1",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual := normalizeOpenAIBaseURL(testCase.input)
			require.Equal(t, testCase.expected, actual)
		})
	}
}

func TestNewOpenAIEmbeddingClientBaseURLNormalization(t *testing.T) {
	t.Parallel()

	client, err := newOpenAIEmbeddingClient(&openAIEmbeddingConfig{
		baseURL: "api.v3.cm/v1",
		apiKey:  "sk-test",
		model:   "",
	})
	require.NoError(t, err)
	require.Equal(t, "https://api.v3.cm/v1", client.baseURL)
	require.Equal(t, defaultEmbeddingModel, client.model)
	require.Equal(t, openAIEmbeddingMaxRetry, client.maxRetry)
	require.Equal(t, openAIEmbeddingBackoff, client.backoff)
}

func TestNewOpenAIEmbeddingClientRequireAPIKey(t *testing.T) {
	t.Parallel()

	_, err := newOpenAIEmbeddingClient(&openAIEmbeddingConfig{
		baseURL: "https://api.openai.com/v1",
		apiKey:  " ",
		model:   defaultEmbeddingModel,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "openai api key is not configured")
}

func TestNewOpenAIEmbeddingClientRetryConfigFromSetting(t *testing.T) {
	t.Parallel()

	client, err := newOpenAIEmbeddingClient(&openAIEmbeddingConfig{
		baseURL:   "https://api.openai.com/v1",
		apiKey:    "sk-test",
		model:     "text-embedding-3-small",
		maxRetry:  4,
		backoffMs: 250,
	})
	require.NoError(t, err)
	require.Equal(t, 4, client.maxRetry)
	require.Equal(t, 250*time.Millisecond, client.backoff)
}

func TestOpenAIEmbeddingClientRetryOnServerError(t *testing.T) {
	t.Parallel()

	var attemptCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := atomic.AddInt32(&attemptCount, 1)
		w.Header().Set("Content-Type", "application/json")
		if attempt < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"temporary upstream error"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"embedding": []float64{0.1, 0.2, 0.3},
				},
			},
		})
	}))
	defer server.Close()

	client, err := newOpenAIEmbeddingClient(&openAIEmbeddingConfig{
		baseURL: server.URL,
		apiKey:  "sk-test",
		model:   "test-model",
	})
	require.NoError(t, err)

	embedding, err := client.Embed(context.Background(), "hello world")
	require.NoError(t, err)
	require.Equal(t, []float64{0.1, 0.2, 0.3}, embedding)
	require.Equal(t, int32(3), atomic.LoadInt32(&attemptCount))
}

func TestOpenAIEmbeddingClientNoRetryOnUnauthorized(t *testing.T) {
	t.Parallel()

	var attemptCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attemptCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	client, err := newOpenAIEmbeddingClient(&openAIEmbeddingConfig{
		baseURL: server.URL,
		apiKey:  "sk-test",
		model:   "test-model",
	})
	require.NoError(t, err)

	_, err = client.Embed(context.Background(), "hello world")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid api key")
	require.Equal(t, int32(1), atomic.LoadInt32(&attemptCount))
}

func TestOpenAIEmbeddingClientJinaRequestBody(t *testing.T) {
	t.Parallel()

	type embeddingRequest struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
		Task  string   `json:"task"`
	}
	requestCh := make(chan embeddingRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request embeddingRequest
		err := json.NewDecoder(r.Body).Decode(&request)
		require.NoError(t, err)
		requestCh <- request

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"embedding": []float64{0.1, 0.2, 0.3},
				},
			},
		})
	}))
	defer server.Close()

	client, err := newOpenAIEmbeddingClient(&openAIEmbeddingConfig{
		baseURL: server.URL,
		apiKey:  "jina-test-key",
		model:   "jina-embeddings-v4",
	})
	require.NoError(t, err)

	_, err = client.Embed(withEmbeddingTask(context.Background(), embeddingTaskQuery), "hello world")
	require.NoError(t, err)

	request := <-requestCh
	require.Equal(t, "jina-embeddings-v4", request.Model)
	require.Equal(t, []string{"hello world"}, request.Input)
	require.Equal(t, embeddingTaskQuery, request.Task)
}

func TestParseOpenAIEmbeddingMaxRetry(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "empty uses default",
			input:    "",
			expected: openAIEmbeddingMaxRetry,
		},
		{
			name:     "valid value",
			input:    "5",
			expected: 5,
		},
		{
			name:     "zero falls back",
			input:    "0",
			expected: openAIEmbeddingMaxRetry,
		},
		{
			name:     "negative falls back",
			input:    "-1",
			expected: openAIEmbeddingMaxRetry,
		},
		{
			name:     "invalid falls back",
			input:    "abc",
			expected: openAIEmbeddingMaxRetry,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual := parseOpenAIEmbeddingMaxRetry(testCase.input)
			require.Equal(t, testCase.expected, actual)
		})
	}
}

func TestResolveOpenAIEmbeddingRetryConfig(t *testing.T) {
	testCases := []struct {
		name             string
		settingMaxRetry  int32
		settingBackoffMS int32
		envMaxRetry      string
		envBackoffMS     string
		expectedMaxRetry int
		expectedBackoff  time.Duration
	}{
		{
			name:             "setting values override env",
			settingMaxRetry:  4,
			settingBackoffMS: 250,
			envMaxRetry:      "6",
			envBackoffMS:     "300",
			expectedMaxRetry: 4,
			expectedBackoff:  250 * time.Millisecond,
		},
		{
			name:             "fallback to env when setting missing",
			settingMaxRetry:  0,
			settingBackoffMS: 0,
			envMaxRetry:      "5",
			envBackoffMS:     "220",
			expectedMaxRetry: 5,
			expectedBackoff:  220 * time.Millisecond,
		},
		{
			name:             "fallback to default when both missing",
			settingMaxRetry:  0,
			settingBackoffMS: 0,
			envMaxRetry:      "",
			envBackoffMS:     "",
			expectedMaxRetry: openAIEmbeddingMaxRetry,
			expectedBackoff:  openAIEmbeddingBackoff,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv(openAIEmbeddingMaxRetryEnv, testCase.envMaxRetry)
			t.Setenv(openAIEmbeddingBackoffMSEnv, testCase.envBackoffMS)
			actualMaxRetry, actualBackoff := resolveOpenAIEmbeddingRetryConfig(testCase.settingMaxRetry, testCase.settingBackoffMS)
			require.Equal(t, testCase.expectedMaxRetry, actualMaxRetry)
			require.Equal(t, testCase.expectedBackoff, actualBackoff)
		})
	}
}

func TestParseOpenAIEmbeddingMaxRetryFromSetting(t *testing.T) {
	t.Parallel()

	require.Equal(t, 0, parseOpenAIEmbeddingMaxRetryFromSetting(0))
	require.Equal(t, 0, parseOpenAIEmbeddingMaxRetryFromSetting(-1))
	require.Equal(t, 3, parseOpenAIEmbeddingMaxRetryFromSetting(3))
}

func TestParseOpenAIEmbeddingRetryBackoffFromSetting(t *testing.T) {
	t.Parallel()

	require.Equal(t, time.Duration(0), parseOpenAIEmbeddingRetryBackoffFromSetting(0))
	require.Equal(t, time.Duration(0), parseOpenAIEmbeddingRetryBackoffFromSetting(-1))
	require.Equal(t, 150*time.Millisecond, parseOpenAIEmbeddingRetryBackoffFromSetting(150))
}

func TestParseOpenAIEmbeddingRetryBackoff(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{
			name:     "empty uses default",
			input:    "",
			expected: openAIEmbeddingBackoff,
		},
		{
			name:     "valid value in ms",
			input:    "250",
			expected: 250 * time.Millisecond,
		},
		{
			name:     "zero falls back",
			input:    "0",
			expected: openAIEmbeddingBackoff,
		},
		{
			name:     "negative falls back",
			input:    "-10",
			expected: openAIEmbeddingBackoff,
		},
		{
			name:     "invalid falls back",
			input:    "abc",
			expected: openAIEmbeddingBackoff,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			actual := parseOpenAIEmbeddingRetryBackoff(testCase.input)
			require.Equal(t, testCase.expected, actual)
		})
	}
}
