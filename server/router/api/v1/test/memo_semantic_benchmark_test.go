package test

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	apiv1 "github.com/usememos/memos/server/router/api/v1"
	"github.com/usememos/memos/store"
)

const (
	benchmarkSemanticMemoCount  = 10000
	benchmarkSemanticVectorSize = 64
	benchmarkSemanticModel      = "benchmark-embedding-model"
	benchmarkSemanticQueryText  = "semantic benchmark query"
)

func BenchmarkSearchMemosSemanticPostgres10k(b *testing.B) {
	if os.Getenv("DRIVER") != "postgres" {
		b.Skip("benchmark requires DRIVER=postgres")
	}

	ctx := context.Background()
	ts := NewTestService(b)
	b.Cleanup(ts.Cleanup)

	user, err := ts.CreateRegularUser(ctx, "semantic-bench-user")
	if err != nil {
		b.Fatalf("failed to create benchmark user: %v", err)
	}

	// Build a deterministic 10k corpus with embeddings to mirror target scale.
	for i := range benchmarkSemanticMemoCount {
		memo, createErr := ts.Store.CreateMemo(ctx, &store.Memo{
			UID:        fmt.Sprintf("semantic-bench-%d", i),
			CreatorID:  user.ID,
			RowStatus:  store.Normal,
			Content:    fmt.Sprintf("semantic benchmark memo %d", i),
			Visibility: store.Private,
		})
		if createErr != nil {
			b.Fatalf("failed to create benchmark memo %d: %v", i, createErr)
		}

		upsertErr := ts.Store.UpsertMemoEmbedding(ctx, &store.MemoEmbedding{
			MemoID:      memo.ID,
			Model:       benchmarkSemanticModel,
			Dimension:   benchmarkSemanticVectorSize,
			Embedding:   buildBenchmarkVector(i),
			ContentHash: fmt.Sprintf("semantic-bench-hash-%d", i),
		})
		if upsertErr != nil {
			b.Fatalf("failed to upsert benchmark embedding %d: %v", i, upsertErr)
		}
	}

	queryVector := buildBenchmarkVector(benchmarkSemanticMemoCount + 7)
	ts.Service.EmbeddingClientFactory = func(context.Context) (apiv1.SemanticEmbeddingClient, error) {
		return &fakeSemanticEmbeddingClient{
			model: benchmarkSemanticModel,
			vectors: map[string][]float64{
				benchmarkSemanticQueryText: queryVector,
			},
		}, nil
	}

	userCtx := ts.CreateUserContext(ctx, user.ID)
	request := &v1pb.SearchMemosSemanticRequest{
		Query:    benchmarkSemanticQueryText,
		PageSize: 20,
	}

	// Warm up one query before timing to reduce one-time initialization noise.
	if _, err := ts.Service.SearchMemosSemantic(userCtx, request); err != nil {
		b.Fatalf("failed warm-up semantic query: %v", err)
	}

	durations := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for range b.N {
		begin := time.Now()
		_, searchErr := ts.Service.SearchMemosSemantic(userCtx, request)
		if searchErr != nil {
			b.Fatalf("failed semantic search during benchmark: %v", searchErr)
		}
		durations = append(durations, time.Since(begin))
	}
	b.StopTimer()

	if len(durations) == 0 {
		return
	}
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})
	b.ReportMetric(durationMetricMS(percentileDuration(durations, 0.50)), "p50_ms")
	b.ReportMetric(durationMetricMS(percentileDuration(durations, 0.95)), "p95_ms")
	b.ReportMetric(durationMetricMS(percentileDuration(durations, 0.99)), "p99_ms")
}

func buildBenchmarkVector(seed int) []float64 {
	vector := make([]float64, benchmarkSemanticVectorSize)
	for i := range benchmarkSemanticVectorSize {
		phase := float64(seed*benchmarkSemanticVectorSize+i) * 0.017
		vector[i] = math.Sin(phase) + 0.5*math.Cos(phase*0.7)
	}
	return vector
}

func percentileDuration(sorted []time.Duration, percentile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if percentile <= 0 {
		return sorted[0]
	}
	if percentile >= 1 {
		return sorted[len(sorted)-1]
	}
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func durationMetricMS(value time.Duration) float64 {
	return float64(value.Nanoseconds()) / float64(time.Millisecond)
}
