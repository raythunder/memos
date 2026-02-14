package test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	apiv1 "github.com/usememos/memos/server/router/api/v1"
	"github.com/usememos/memos/store"
)

type fakeSemanticEmbeddingClient struct {
	model   string
	vectors map[string][]float64
}

func (c *fakeSemanticEmbeddingClient) Embed(_ context.Context, text string) ([]float64, error) {
	vector, ok := c.vectors[text]
	if !ok {
		return nil, errors.New("vector not found")
	}
	return vector, nil
}

func (c *fakeSemanticEmbeddingClient) Model() string {
	return c.model
}

func TestSearchMemosSemanticPostgresRankingAndVisibility(t *testing.T) {
	if os.Getenv("DRIVER") != "postgres" {
		t.Skip("postgres only")
	}

	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	userOne, err := ts.CreateRegularUser(ctx, "semantic-user-1")
	require.NoError(t, err)
	userTwo, err := ts.CreateRegularUser(ctx, "semantic-user-2")
	require.NoError(t, err)

	memoOne, err := ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "semantic-owned-private",
		CreatorID:  userOne.ID,
		RowStatus:  store.Normal,
		Content:    "owned private memo",
		Visibility: store.Private,
	})
	require.NoError(t, err)
	memoTwo, err := ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "semantic-other-private",
		CreatorID:  userTwo.ID,
		RowStatus:  store.Normal,
		Content:    "other private memo",
		Visibility: store.Private,
	})
	require.NoError(t, err)
	memoThree, err := ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "semantic-owned-public",
		CreatorID:  userOne.ID,
		RowStatus:  store.Normal,
		Content:    "owned public memo",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	require.NoError(t, ts.Store.UpsertMemoEmbedding(ctx, &store.MemoEmbedding{
		MemoID:      memoOne.ID,
		Model:       "fake-embedding-model",
		Dimension:   2,
		Embedding:   []float64{1, 0},
		ContentHash: "hash-m1",
	}))
	require.NoError(t, ts.Store.UpsertMemoEmbedding(ctx, &store.MemoEmbedding{
		MemoID:      memoTwo.ID,
		Model:       "fake-embedding-model",
		Dimension:   2,
		Embedding:   []float64{0.99, 0.01},
		ContentHash: "hash-m2",
	}))
	require.NoError(t, ts.Store.UpsertMemoEmbedding(ctx, &store.MemoEmbedding{
		MemoID:      memoThree.ID,
		Model:       "fake-embedding-model",
		Dimension:   2,
		Embedding:   []float64{0.4, 0.6},
		ContentHash: "hash-m3",
	}))

	ts.Service.EmbeddingClientFactory = func(context.Context) (apiv1.SemanticEmbeddingClient, error) {
		return &fakeSemanticEmbeddingClient{
			model: "fake-embedding-model",
			vectors: map[string][]float64{
				"semantic query": {1, 0},
			},
		}, nil
	}

	userCtx := ts.CreateUserContext(ctx, userOne.ID)
	response, err := ts.Service.SearchMemosSemantic(userCtx, &v1pb.SearchMemosSemanticRequest{
		Query:    "semantic query",
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Len(t, response.Memos, 2)
	require.Equal(t, "memos/"+memoOne.UID, response.Memos[0].Name)
	require.Equal(t, "memos/"+memoThree.UID, response.Memos[1].Name)
	for _, memo := range response.Memos {
		require.NotEqual(t, "memos/"+memoTwo.UID, memo.Name)
	}
}

func TestSearchMemosSemanticPostgresEmbeddingConfigError(t *testing.T) {
	if os.Getenv("DRIVER") != "postgres" {
		t.Skip("postgres only")
	}

	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "semantic-user")
	require.NoError(t, err)

	ts.Service.EmbeddingClientFactory = func(context.Context) (apiv1.SemanticEmbeddingClient, error) {
		return nil, errors.New("mock embedding unavailable")
	}

	userCtx := ts.CreateUserContext(ctx, user.ID)
	_, err = ts.Service.SearchMemosSemantic(userCtx, &v1pb.SearchMemosSemanticRequest{
		Query: "semantic query",
	})
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.FailedPrecondition, st.Code())
}
