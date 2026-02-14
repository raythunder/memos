package v1

import (
	"testing"

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
