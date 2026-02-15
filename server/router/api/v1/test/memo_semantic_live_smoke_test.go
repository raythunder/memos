package test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/store"
)

const liveSemanticSmokeEnv = "MEMOS_SEMANTIC_LIVE_SMOKE"

type liveSemanticMemoSeed struct {
	key     string
	content string
}

type liveSemanticQueryScenario struct {
	name         string
	query        string
	expectedKeys []string
	topN         int
}

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

	memoSeeds := []liveSemanticMemoSeed{
		{
			key:     "backend_postmortem",
			content: "Postmortem notes for gRPC Connect backend reliability incident, timeout tuning, and retry budgets.",
		},
		{
			key:     "backend_observability",
			content: "Design doc: improve backend observability for connect-rpc handlers, structured logs, and API health probes.",
		},
		{
			key:     "backend_runbook",
			content: "Runbook for backend search reliability with circuit breaker policy and degraded-mode fallback.",
		},
		{
			key:     "recipe_sourdough",
			content: "Sourdough bread recipe with starter hydration ratio, long fermentation schedule, and oven steam setup.",
		},
		{
			key:     "recipe_roast",
			content: "Roasted chicken dinner plan with rosemary potatoes and lemon butter sauce.",
		},
		{
			key:     "hiking_plan",
			content: "Weekend hiking plan: mountain trail altitude profile, backpack checklist, and offline navigation map.",
		},
		{
			key:     "hiking_gear",
			content: "Alpine hiking gear list with trekking poles, hydration pack, waterproof shell, and emergency blanket.",
		},
		{
			key:     "finance_budget",
			content: "Monthly budget review covering savings ratio, ETF allocation, and emergency cash buffer.",
		},
		{
			key:     "reading_notes",
			content: "Book notes about distributed systems, consensus protocols, and failure-domain isolation.",
		},
		{
			key:     "garden_schedule",
			content: "Garden maintenance schedule: prune tomato vines, monitor soil moisture, and compost rotation.",
		},
		{
			key:     "movie_watchlist",
			content: "Weekend movie watchlist with sci-fi classics, noir detective stories, and animation picks.",
		},
		{
			key:     "fitness_cycle",
			content: "Cycling training plan with threshold intervals, cadence drills, and recovery day nutrition.",
		},
	}

	memoNameByKey, memoContentByName := make(map[string]string, len(memoSeeds)), make(map[string]string, len(memoSeeds))
	memoIDs := make([]int32, 0, len(memoSeeds))
	for _, seed := range memoSeeds {
		memo, createErr := ts.Service.CreateMemo(userCtx, &v1pb.CreateMemoRequest{
			Memo: &v1pb.Memo{
				Content:    seed.content,
				Visibility: v1pb.Visibility_PRIVATE,
			},
		})
		require.NoErrorf(t, createErr, "failed to create memo seed %q", seed.key)

		memoNameByKey[seed.key] = memo.Name
		memoContentByName[memo.Name] = seed.content

		uid := strings.TrimPrefix(memo.Name, "memos/")
		storeMemo, getErr := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &uid})
		require.NoErrorf(t, getErr, "failed to query store memo for seed %q", seed.key)
		require.NotNil(t, storeMemo)
		memoIDs = append(memoIDs, storeMemo.ID)
	}

	waitTimeout := 2*time.Minute + time.Duration(len(memoSeeds))*4*time.Second
	require.NoError(t, waitForMemoEmbeddingReady(ctx, ts.Store, memoIDs, waitTimeout))

	scenarios := []liveSemanticQueryScenario{
		{
			name:         "backend reliability query",
			query:        "grpc connect backend reliability observability runbook",
			expectedKeys: []string{"backend_postmortem", "backend_observability", "backend_runbook"},
			topN:         5,
		},
		{
			name:         "recipe query",
			query:        "sourdough starter fermentation oven steam recipe",
			expectedKeys: []string{"recipe_sourdough"},
			topN:         5,
		},
		{
			name:         "hiking query",
			query:        "mountain hiking trail altitude backpack checklist",
			expectedKeys: []string{"hiking_plan", "hiking_gear"},
			topN:         5,
		},
	}

	for _, scenario := range scenarios {
		startedAt := time.Now()
		response, searchErr := ts.Service.SearchMemosSemantic(userCtx, &v1pb.SearchMemosSemanticRequest{
			Query:    scenario.query,
			PageSize: 10,
		})
		require.NoErrorf(t, searchErr, "live semantic query failed for scenario %q", scenario.name)
		require.NotEmptyf(t, response.Memos, "empty semantic result for scenario %q", scenario.name)

		topN := scenario.topN
		if topN <= 0 || topN > len(response.Memos) {
			topN = len(response.Memos)
		}

		resultSummary := make([]string, 0, topN)
		topResultNames := make([]string, 0, topN)
		for index, memo := range response.Memos[:topN] {
			topResultNames = append(topResultNames, memo.Name)
			resultSummary = append(resultSummary, fmt.Sprintf("%d)%s => %s", index+1, memo.Name, memoContentByName[memo.Name]))
		}

		t.Logf(
			"[live-semantic] scenario=%q query=%q latency=%s top%d=%s",
			scenario.name,
			scenario.query,
			time.Since(startedAt).Round(time.Millisecond),
			topN,
			strings.Join(resultSummary, " | "),
		)

		matched := false
		for _, expectedKey := range scenario.expectedKeys {
			if slices.Contains(topResultNames, memoNameByKey[expectedKey]) {
				matched = true
				break
			}
		}
		require.Truef(
			t,
			matched,
			"expected one of %v to appear in top %d for scenario %q, got %v",
			scenario.expectedKeys,
			topN,
			scenario.name,
			topResultNames,
		)
	}
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
