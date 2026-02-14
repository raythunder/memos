package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestMemoEmbeddingStore_UnsupportedDriver(t *testing.T) {
	t.Parallel()
	if getDriverFromEnv() == "postgres" {
		t.Skip("this case only validates non-postgres behavior")
	}

	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	err := ts.UpsertMemoEmbedding(ctx, &store.MemoEmbedding{
		MemoID:      1,
		Model:       "test-model",
		Dimension:   2,
		Embedding:   []float64{0.1, 0.2},
		ContentHash: "hash-a",
	})
	require.ErrorContains(t, err, "only supported for postgres driver")

	_, err = ts.GetMemoEmbeddingContentHash(ctx, 1)
	require.ErrorContains(t, err, "only supported for postgres driver")

	_, err = ts.ListMemoEmbeddingsByMemoIDs(ctx, []int32{1})
	require.ErrorContains(t, err, "only supported for postgres driver")

	err = ts.DeleteMemoEmbeddingByMemoID(ctx, 1)
	require.ErrorContains(t, err, "only supported for postgres driver")
}

func TestMemoEmbeddingStore_PostgresCRUD(t *testing.T) {
	t.Parallel()
	if getDriverFromEnv() != "postgres" {
		t.Skip("postgres only")
	}

	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	memo, err := ts.CreateMemo(ctx, &store.Memo{
		UID:        "memo-embedding-test",
		CreatorID:  user.ID,
		Content:    "memo for embedding",
		Visibility: store.Private,
	})
	require.NoError(t, err)

	// Empty state.
	contentHash, err := ts.GetMemoEmbeddingContentHash(ctx, memo.ID)
	require.NoError(t, err)
	require.Empty(t, contentHash)

	embeddingA := []float64{0.12, -0.34, 0.56}
	err = ts.UpsertMemoEmbedding(ctx, &store.MemoEmbedding{
		MemoID:      memo.ID,
		Model:       "text-embedding-3-small",
		Dimension:   int32(len(embeddingA)),
		Embedding:   embeddingA,
		ContentHash: "hash-a",
	})
	require.NoError(t, err)

	contentHash, err = ts.GetMemoEmbeddingContentHash(ctx, memo.ID)
	require.NoError(t, err)
	require.Equal(t, "hash-a", contentHash)

	embeddingMap, err := ts.ListMemoEmbeddingsByMemoIDs(ctx, []int32{memo.ID, memo.ID + 100})
	require.NoError(t, err)
	require.Len(t, embeddingMap, 1)
	require.Equal(t, embeddingA, embeddingMap[memo.ID])

	// Upsert should overwrite by memo_id conflict key.
	embeddingB := []float64{0.91, 0.82, 0.73}
	err = ts.UpsertMemoEmbedding(ctx, &store.MemoEmbedding{
		MemoID:      memo.ID,
		Model:       "text-embedding-3-large",
		Dimension:   int32(len(embeddingB)),
		Embedding:   embeddingB,
		ContentHash: "hash-b",
	})
	require.NoError(t, err)

	contentHash, err = ts.GetMemoEmbeddingContentHash(ctx, memo.ID)
	require.NoError(t, err)
	require.Equal(t, "hash-b", contentHash)

	embeddingMap, err = ts.ListMemoEmbeddingsByMemoIDs(ctx, []int32{memo.ID})
	require.NoError(t, err)
	require.Len(t, embeddingMap, 1)
	require.Equal(t, embeddingB, embeddingMap[memo.ID])

	err = ts.DeleteMemoEmbeddingByMemoID(ctx, memo.ID)
	require.NoError(t, err)

	contentHash, err = ts.GetMemoEmbeddingContentHash(ctx, memo.ID)
	require.NoError(t, err)
	require.Empty(t, contentHash)

	embeddingMap, err = ts.ListMemoEmbeddingsByMemoIDs(ctx, []int32{memo.ID})
	require.NoError(t, err)
	require.Len(t, embeddingMap, 0)
}

func TestMemoEmbeddingStore_PostgresValidation(t *testing.T) {
	t.Parallel()
	if getDriverFromEnv() != "postgres" {
		t.Skip("postgres only")
	}

	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	err := ts.UpsertMemoEmbedding(ctx, nil)
	require.ErrorContains(t, err, "embedding cannot be nil")

	err = ts.UpsertMemoEmbedding(ctx, &store.MemoEmbedding{
		MemoID:      1,
		Model:       "test-model",
		Dimension:   0,
		Embedding:   nil,
		ContentHash: "hash-empty",
	})
	require.ErrorContains(t, err, "embedding vector cannot be empty")
}
