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
			name:     "zero is valid",
			input:    "0",
			expected: 0,
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
