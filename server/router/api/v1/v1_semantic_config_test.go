package v1

import "testing"

func TestResolveEmbeddingRefreshConcurrency(t *testing.T) {
	testCases := []struct {
		name     string
		envValue string
		expected int64
	}{
		{
			name:     "empty uses default",
			envValue: "",
			expected: defaultEmbeddingRefreshConcurrency,
		},
		{
			name:     "valid positive value",
			envValue: "12",
			expected: 12,
		},
		{
			name:     "invalid value falls back",
			envValue: "abc",
			expected: defaultEmbeddingRefreshConcurrency,
		},
		{
			name:     "zero falls back",
			envValue: "0",
			expected: defaultEmbeddingRefreshConcurrency,
		},
		{
			name:     "negative falls back",
			envValue: "-3",
			expected: defaultEmbeddingRefreshConcurrency,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv(semanticEmbeddingConcurrencyEnv, testCase.envValue)
			actual := resolveEmbeddingRefreshConcurrency()
			if actual != testCase.expected {
				t.Fatalf("resolveEmbeddingRefreshConcurrency()=%d, expected=%d", actual, testCase.expected)
			}
		})
	}
}
