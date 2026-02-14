package v1

import (
	"context"
	"testing"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
	teststore "github.com/usememos/memos/store/test"
)

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
			actual := parseEmbeddingRefreshConcurrencyFromEnv(testCase.envValue)
			if actual != testCase.expected {
				t.Fatalf("parseEmbeddingRefreshConcurrencyFromEnv()=%d, expected=%d", actual, testCase.expected)
			}
		})
	}
}

func TestResolveEmbeddingRefreshConcurrencyFromAISetting(t *testing.T) {
	ctx := context.Background()
	stores := teststore.NewTestingStore(ctx, t)
	defer stores.Close()

	_, err := stores.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_AI,
		Value: &storepb.InstanceSetting_AiSetting{
			AiSetting: &storepb.InstanceAISetting{
				SemanticEmbeddingConcurrency: 15,
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to upsert ai setting: %v", err)
	}

	t.Setenv(semanticEmbeddingConcurrencyEnv, "3")
	actual := resolveEmbeddingRefreshConcurrency(ctx, stores)
	if actual != 15 {
		t.Fatalf("resolveEmbeddingRefreshConcurrency()=%d, expected=15", actual)
	}
}

func TestResolveEmbeddingRefreshConcurrencyFallbackToEnvWhenStoreMissing(t *testing.T) {
	ctx := context.Background()
	t.Setenv(semanticEmbeddingConcurrencyEnv, "11")
	actual := resolveEmbeddingRefreshConcurrency(ctx, (*store.Store)(nil))
	if actual != 11 {
		t.Fatalf("resolveEmbeddingRefreshConcurrency()=%d, expected=11", actual)
	}
}
