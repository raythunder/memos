package v1

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1pb "github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/store"
)

const semanticSearchBatchSize = 2000

func (s *APIV1Service) SearchMemosSemantic(ctx context.Context, request *v1pb.SearchMemosSemanticRequest) (*v1pb.ListMemosResponse, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, status.Errorf(codes.InvalidArgument, "query is required")
	}
	if !s.semanticStorageEnabled() {
		return nil, status.Errorf(codes.FailedPrecondition, "semantic search only supports postgres driver")
	}

	embeddingClient, err := s.newOpenAIEmbeddingClient(ctx)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "semantic search is not configured: %v", err)
	}
	queryEmbedding, err := embeddingClient.Embed(ctx, query)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate query embedding: %v", err)
	}

	memoFind := &store.FindMemo{
		ExcludeComments: true,
	}
	if request.State == v1pb.State_ARCHIVED {
		state := store.Archived
		memoFind.RowStatus = &state
	} else {
		state := store.Normal
		memoFind.RowStatus = &state
	}
	if request.Filter != "" {
		if err := s.validateFilter(ctx, request.Filter); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid filter: %v", err)
		}
		memoFind.Filters = append(memoFind.Filters, request.Filter)
	}
	if err := s.applyMemoVisibilityFilter(ctx, memoFind); err != nil {
		return nil, err
	}

	memos, err := s.listMemosForSemanticSearch(ctx, memoFind)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list semantic candidates: %v", err)
	}
	if len(memos) == 0 {
		return &v1pb.ListMemosResponse{
			Memos:         []*v1pb.Memo{},
			NextPageToken: "",
		}, nil
	}

	memoIDList := make([]int32, 0, len(memos))
	for _, memo := range memos {
		memoIDList = append(memoIDList, memo.ID)
	}
	embeddingMap, err := s.Store.ListMemoEmbeddingsByMemoIDs(ctx, memoIDList)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to load semantic embeddings: %v", err)
	}

	type scoredMemo struct {
		memo  *store.Memo
		score float64
	}
	scoredMemos := make([]scoredMemo, 0, len(memos))
	for _, memo := range memos {
		embedding, ok := embeddingMap[memo.ID]
		if !ok {
			continue
		}
		score, ok := cosineSimilarity(queryEmbedding, embedding)
		if !ok {
			continue
		}
		scoredMemos = append(scoredMemos, scoredMemo{
			memo:  memo,
			score: score,
		})
	}
	if len(scoredMemos) == 0 {
		return &v1pb.ListMemosResponse{
			Memos:         []*v1pb.Memo{},
			NextPageToken: "",
		}, nil
	}

	sort.Slice(scoredMemos, func(i, j int) bool {
		if scoredMemos[i].score == scoredMemos[j].score {
			if scoredMemos[i].memo.UpdatedTs == scoredMemos[j].memo.UpdatedTs {
				return scoredMemos[i].memo.ID > scoredMemos[j].memo.ID
			}
			return scoredMemos[i].memo.UpdatedTs > scoredMemos[j].memo.UpdatedTs
		}
		return scoredMemos[i].score > scoredMemos[j].score
	})

	limit, offset := int(request.PageSize), 0
	if request.PageToken != "" {
		var pageToken v1pb.PageToken
		if err := unmarshalPageToken(request.PageToken, &pageToken); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid page token: %v", err)
		}
		limit = int(pageToken.Limit)
		offset = int(pageToken.Offset)
	}
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	if offset >= len(scoredMemos) {
		return &v1pb.ListMemosResponse{
			Memos:         []*v1pb.Memo{},
			NextPageToken: "",
		}, nil
	}

	end := offset + limit
	if end > len(scoredMemos) {
		end = len(scoredMemos)
	}
	selectedMemos := make([]*store.Memo, 0, end-offset)
	for _, item := range scoredMemos[offset:end] {
		selectedMemos = append(selectedMemos, item.memo)
	}

	memoMessages, err := s.convertMemoListToMessages(ctx, selectedMemos)
	if err != nil {
		return nil, err
	}

	nextPageToken := ""
	if end < len(scoredMemos) {
		nextPageToken, err = getPageToken(limit, end)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get next page token: %v", err)
		}
	}
	return &v1pb.ListMemosResponse{
		Memos:         memoMessages,
		NextPageToken: nextPageToken,
	}, nil
}

func (s *APIV1Service) listMemosForSemanticSearch(ctx context.Context, base *store.FindMemo) ([]*store.Memo, error) {
	allMemos := make([]*store.Memo, 0, semanticSearchBatchSize)
	offset := 0
	for {
		limit := semanticSearchBatchSize
		find := *base
		find.Limit = &limit
		find.Offset = &offset

		memos, err := s.Store.ListMemos(ctx, &find)
		if err != nil {
			return nil, err
		}
		if len(memos) == 0 {
			break
		}
		allMemos = append(allMemos, memos...)
		if len(memos) < semanticSearchBatchSize {
			break
		}
		offset += len(memos)
	}
	return allMemos, nil
}

func cosineSimilarity(a, b []float64) (float64, bool) {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0, false
	}

	var dotProduct float64
	var normA float64
	var normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0, false
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)), true
}

func (s *APIV1Service) semanticStorageEnabled() bool {
	return s.Profile != nil && strings.EqualFold(s.Profile.Driver, "postgres")
}

func (s *APIV1Service) semanticIndexingEnabled() bool {
	if !s.semanticStorageEnabled() {
		return false
	}
	_, err := s.newOpenAIEmbeddingClient(context.Background())
	return err == nil
}

func (s *APIV1Service) scheduleMemoEmbeddingSync(memoID int32, content string) {
	if !s.semanticIndexingEnabled() {
		return
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if err := s.refreshMemoEmbedding(ctx, memoID, content); err != nil {
			slog.Warn("failed to refresh memo embedding", "memoID", memoID, "error", err)
		}
	}()
}

func (s *APIV1Service) scheduleMemoEmbeddingDelete(memoID int32) {
	if !s.semanticStorageEnabled() {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Store.DeleteMemoEmbeddingByMemoID(ctx, memoID); err != nil {
			slog.Warn("failed to delete memo embedding", "memoID", memoID, "error", err)
		}
	}()
}

func (s *APIV1Service) refreshMemoEmbedding(ctx context.Context, memoID int32, content string) error {
	embeddingClient, err := s.newOpenAIEmbeddingClient(ctx)
	if err != nil {
		return err
	}

	contentHashBytes := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(contentHashBytes[:])

	existingHash, err := s.Store.GetMemoEmbeddingContentHash(ctx, memoID)
	if err != nil {
		return err
	}
	if existingHash == contentHash {
		return nil
	}

	embedding, err := embeddingClient.Embed(ctx, content)
	if err != nil {
		return err
	}

	return s.Store.UpsertMemoEmbedding(ctx, &store.MemoEmbedding{
		MemoID:      memoID,
		Model:       embeddingClient.model,
		Dimension:   int32(len(embedding)),
		Embedding:   embedding,
		ContentHash: contentHash,
	})
}
