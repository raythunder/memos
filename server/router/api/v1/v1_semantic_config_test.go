package v1

import (
	"context"
	"testing"
	"time"

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

func TestResolveEmbeddingRefreshConcurrencyWithSettingFallback(t *testing.T) {
	t.Setenv(semanticEmbeddingConcurrencyEnv, "9")
	actual := resolveEmbeddingRefreshConcurrencyWithSetting(0)
	if actual != 9 {
		t.Fatalf("resolveEmbeddingRefreshConcurrencyWithSetting()=%d, expected=9", actual)
	}
}

func TestSetEmbeddingSemaphoreLimit(t *testing.T) {
	service := &APIV1Service{}
	service.setEmbeddingSemaphoreLimit(1)
	semaphoreOne := service.getEmbeddingSemaphore()
	if semaphoreOne == nil {
		t.Fatal("embedding semaphore should not be nil")
	}

	ctx := context.Background()
	if err := semaphoreOne.Acquire(ctx, 1); err != nil {
		t.Fatalf("failed to acquire first semaphore: %v", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := semaphoreOne.Acquire(timeoutCtx, 1); err == nil {
		t.Fatal("expected second acquire to fail due limit")
	}
	semaphoreOne.Release(1)

	service.setEmbeddingSemaphoreLimit(2)
	semaphoreTwo := service.getEmbeddingSemaphore()
	if semaphoreTwo == nil {
		t.Fatal("updated embedding semaphore should not be nil")
	}
	if semaphoreTwo == semaphoreOne {
		t.Fatal("expected semaphore to be replaced on limit update")
	}
	if err := semaphoreTwo.Acquire(ctx, 2); err != nil {
		t.Fatalf("failed to acquire updated semaphore with limit=2: %v", err)
	}
	semaphoreTwo.Release(2)
}
