package test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/store"
)

const (
	liveSemanticSmokeEnv         = "MEMOS_SEMANTIC_LIVE_SMOKE"
	liveSemanticExtendedSmokeEnv = "MEMOS_SEMANTIC_LIVE_EXTENDED"
	liveSemanticExtendedMemoSize = 48
)

type liveSemanticMemoSeed struct {
	key     string
	content string
}

type liveSemanticQueryScenario struct {
	name         string
	query        string
	expectedKeys []string
	topN         int
	pageSize     int
}

func TestSearchMemosSemanticPostgresLiveOpenAI(t *testing.T) {
	runLiveSemanticOpenAISmoke(t, false)
}

func TestSearchMemosSemanticPostgresLiveOpenAIExtended(t *testing.T) {
	if os.Getenv(liveSemanticExtendedSmokeEnv) != "1" {
		t.Skipf("set %s=1 to enable extended live semantic smoke test", liveSemanticExtendedSmokeEnv)
	}
	runLiveSemanticOpenAISmoke(t, true)
}

func runLiveSemanticOpenAISmoke(t *testing.T, extended bool) {
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

	memoSeeds := buildLiveSemanticMemoSeeds(extended)
	t.Logf("[live-semantic] mode=%s corpus=%d", liveSemanticModeLabel(extended), len(memoSeeds))

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

	scenarios := buildLiveSemanticQueryScenarios(extended)
	for _, scenario := range scenarios {
		startedAt := time.Now()
		response, searchErr := ts.Service.SearchMemosSemantic(userCtx, &v1pb.SearchMemosSemanticRequest{
			Query:    scenario.query,
			PageSize: int32(scenario.pageSize),
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
			"[live-semantic] mode=%s scenario=%q query=%q latency=%s top%d=%s",
			liveSemanticModeLabel(extended),
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

func buildLiveSemanticMemoSeeds(extended bool) []liveSemanticMemoSeed {
	seeds := []liveSemanticMemoSeed{
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
	if !extended {
		return seeds
	}
	return appendGeneratedLiveSemanticMemoSeeds(seeds, liveSemanticExtendedMemoSize)
}

func buildLiveSemanticQueryScenarios(extended bool) []liveSemanticQueryScenario {
	topN, pageSize := 5, 10
	if extended {
		topN, pageSize = 8, 16
	}
	return []liveSemanticQueryScenario{
		{
			name:         "backend reliability query",
			query:        "grpc connect backend reliability observability runbook",
			expectedKeys: []string{"backend_postmortem", "backend_observability", "backend_runbook"},
			topN:         topN,
			pageSize:     pageSize,
		},
		{
			name:         "recipe query",
			query:        "sourdough starter fermentation oven steam recipe",
			expectedKeys: []string{"recipe_sourdough"},
			topN:         topN,
			pageSize:     pageSize,
		},
		{
			name:         "hiking query",
			query:        "mountain hiking trail altitude backpack checklist",
			expectedKeys: []string{"hiking_plan", "hiking_gear"},
			topN:         topN,
			pageSize:     pageSize,
		},
		{
			name:         "distributed systems query",
			query:        "distributed systems consensus protocol failure-domain isolation notes",
			expectedKeys: []string{"reading_notes"},
			topN:         topN,
			pageSize:     pageSize,
		},
	}
}

func appendGeneratedLiveSemanticMemoSeeds(base []liveSemanticMemoSeed, targetCount int) []liveSemanticMemoSeed {
	if len(base) >= targetCount {
		return base
	}

	generatedTopics := []struct {
		keyPrefix string
		content   string
	}{
		{
			keyPrefix: "tea_brewing",
			content:   "Tea brewing journal entry %d with gongfu steep timeline, water mineral profile, and aroma notes.",
		},
		{
			keyPrefix: "pottery_class",
			content:   "Pottery class checklist %d covering wheel centering drills, clay trimming, and glaze firing schedule.",
		},
		{
			keyPrefix: "astronomy_log",
			content:   "Astronomy observation log %d with telescope alignment, moon phase, and star chart references.",
		},
		{
			keyPrefix: "language_drill",
			content:   "Language practice session %d with shadowing exercises, vocabulary recall, and listening comprehension.",
		},
		{
			keyPrefix: "cleaning_plan",
			content:   "Home cleaning rotation %d for kitchen degreasing, bathroom descaling, and storage reorganization.",
		},
		{
			keyPrefix: "piano_routine",
			content:   "Piano routine %d focused on scales, arpeggio tempo control, and phrasing refinement.",
		},
		{
			keyPrefix: "photo_walk",
			content:   "Film photography walk %d with aperture settings, metering practice, and composition notes.",
		},
		{
			keyPrefix: "bird_watch",
			content:   "Birdwatch checklist %d documenting migration timing, binocular setup, and habitat notes.",
		},
	}

	seeds := append([]liveSemanticMemoSeed(nil), base...)
	for index := len(base); index < targetCount; index++ {
		topic := generatedTopics[index%len(generatedTopics)]
		seedNumber := index - len(base) + 1
		seeds = append(seeds, liveSemanticMemoSeed{
			key:     fmt.Sprintf("%s_%02d", topic.keyPrefix, seedNumber),
			content: fmt.Sprintf(topic.content, seedNumber),
		})
	}
	return seeds
}

func liveSemanticModeLabel(extended bool) string {
	if extended {
		return "extended"
	}
	return "smoke"
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
			return errors.Errorf("timed out waiting memo embeddings, got %d/%d", len(embeddingMap), len(memoIDs))
		}
		time.Sleep(500 * time.Millisecond)
	}
}
