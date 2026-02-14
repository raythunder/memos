package test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/store"
)

const liveSemanticSmokeEnv = "MEMOS_SEMANTIC_LIVE_SMOKE"

func TestSearchMemosSemanticPostgresLiveOpenAI(t *testing.T) {
	if os.Getenv("DRIVER") != "postgres" {
		t.Skip("live semantic smoke test requires DRIVER=postgres")
	}
	if os.Getenv(liveSemanticSmokeEnv) != "1" {
		t.Skipf("set %s=1 to enable live semantic smoke test", liveSemanticSmokeEnv)
	}
	if strings.TrimSpace(os.Getenv("MEMOS_OPENAI_API_KEY")) == "" {
		t.Skip("live semantic smoke test requires MEMOS_OPENAI_API_KEY")
	}

	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "semantic-live-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	primaryMemo, err := ts.Service.CreateMemo(userCtx, &v1pb.CreateMemoRequest{
		Memo: &v1pb.Memo{
			Content:    "Memos semantic search smoke test about grpc connect interoperability and backend reliability.",
			Visibility: v1pb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	secondaryMemo, err := ts.Service.CreateMemo(userCtx, &v1pb.CreateMemoRequest{
		Memo: &v1pb.Memo{
			Content:    "Totally unrelated recipe note: apples, flour, cinnamon, oven timer.",
			Visibility: v1pb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	primaryUID := strings.TrimPrefix(primaryMemo.Name, "memos/")
	secondaryUID := strings.TrimPrefix(secondaryMemo.Name, "memos/")

	primaryStoreMemo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &primaryUID})
	require.NoError(t, err)
	require.NotNil(t, primaryStoreMemo)
	secondaryStoreMemo, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &secondaryUID})
	require.NoError(t, err)
	require.NotNil(t, secondaryStoreMemo)

	require.NoError(t, waitForMemoEmbeddingReady(ctx, ts.Store, []int32{primaryStoreMemo.ID, secondaryStoreMemo.ID}, 90*time.Second))

	response, err := ts.Service.SearchMemosSemantic(userCtx, &v1pb.SearchMemosSemanticRequest{
		Query:    "grpc connect backend search reliability",
		PageSize: 5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, response.Memos)

	resultNames := make(map[string]bool, len(response.Memos))
	for _, memo := range response.Memos {
		resultNames[memo.Name] = true
	}

	require.True(t, resultNames[primaryMemo.Name], "expected semantically related memo to appear in live search result")
}

func waitForMemoEmbeddingReady(ctx context.Context, st *store.Store, memoIDs []int32, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		embeddingMap, err := st.ListMemoEmbeddingsByMemoIDs(ctx, memoIDs)
		if err != nil {
			return err
		}
		if len(embeddingMap) == len(memoIDs) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting memo embeddings, got %d/%d", len(embeddingMap), len(memoIDs))
		}
		time.Sleep(500 * time.Millisecond)
	}
}
