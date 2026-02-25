package v1

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

const (
	semanticReindexProgressFlushStep = 10
	semanticReindexTaskTimeout       = 12 * time.Hour
)

func (s *APIV1Service) startSemanticReindexTask(ctx context.Context) error {
	if !s.semanticStorageEnabled() {
		return status.Errorf(codes.FailedPrecondition, "semantic search only supports postgres driver")
	}
	if _, err := s.getSemanticEmbeddingClient(ctx); err != nil {
		return status.Errorf(codes.FailedPrecondition, "semantic search is not configured: %v", err)
	}

	s.semanticReindexMu.Lock()
	if s.semanticReindexRunning {
		s.semanticReindexMu.Unlock()
		return status.Errorf(codes.AlreadyExists, "semantic reindex is already running")
	}
	s.semanticReindexRunning = true
	s.semanticReindexMu.Unlock()

	go s.runSemanticReindexTask()
	return nil
}

func (s *APIV1Service) resetStaleSemanticReindexState(ctx context.Context) {
	aiSetting, err := s.Store.GetInstanceAISetting(ctx)
	if err != nil || aiSetting == nil || !aiSetting.SemanticReindexRunning {
		return
	}

	if updateErr := s.updateSemanticReindexState(ctx, func(setting *storepb.InstanceAISetting) {
		setting.SemanticReindexRunning = false
		setting.SemanticReindexUpdatedTs = time.Now().Unix()
	}); updateErr != nil {
		slog.Warn("failed to reset stale semantic reindex state", "error", updateErr)
	}
}

func (s *APIV1Service) runSemanticReindexTask() {
	defer func() {
		s.semanticReindexMu.Lock()
		s.semanticReindexRunning = false
		s.semanticReindexMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), semanticReindexTaskTimeout)
	defer cancel()

	embeddingClient, err := s.getSemanticEmbeddingClient(ctx)
	if err != nil {
		slog.Warn("failed to initialize semantic reindex", "error", err)
		return
	}

	model := embeddingClient.Model()
	startedTs := time.Now().Unix()
	_ = s.updateSemanticReindexState(ctx, func(setting *storepb.InstanceAISetting) {
		setting.SemanticReindexRunning = true
		setting.SemanticReindexTotal = 0
		setting.SemanticReindexProcessed = 0
		setting.SemanticReindexFailed = 0
		setting.SemanticReindexStartedTs = startedTs
		setting.SemanticReindexUpdatedTs = startedTs
		setting.SemanticReindexModel = model
	})

	memos, err := s.listMemosForSemanticReindex(ctx)
	if err != nil {
		slog.Warn("failed to list memos for semantic reindex", "error", err)
		_ = s.updateSemanticReindexState(ctx, func(setting *storepb.InstanceAISetting) {
			setting.SemanticReindexRunning = false
			setting.SemanticReindexUpdatedTs = time.Now().Unix()
			setting.SemanticReindexModel = model
		})
		return
	}

	total := len(memos)
	_ = s.updateSemanticReindexState(ctx, func(setting *storepb.InstanceAISetting) {
		setting.SemanticReindexRunning = true
		setting.SemanticReindexTotal = int32(total)
		setting.SemanticReindexProcessed = 0
		setting.SemanticReindexFailed = 0
		setting.SemanticReindexStartedTs = startedTs
		setting.SemanticReindexUpdatedTs = time.Now().Unix()
		setting.SemanticReindexModel = model
	})

	processed := 0
	failed := 0
	for _, memo := range memos {
		content := strings.TrimSpace(memo.Content)
		if content != "" {
			embedCtx := withEmbeddingTask(ctx, embeddingTaskPassage)
			if err := s.refreshMemoEmbeddingWithOptions(embedCtx, memo.ID, memo.Content, true); err != nil {
				failed++
				slog.Warn("semantic reindex failed for memo", "memoID", memo.ID, "error", err)
			}
		}

		processed++
		if processed%semanticReindexProgressFlushStep == 0 || processed == total {
			processedSnapshot := processed
			failedSnapshot := failed
			_ = s.updateSemanticReindexState(ctx, func(setting *storepb.InstanceAISetting) {
				setting.SemanticReindexRunning = true
				setting.SemanticReindexTotal = int32(total)
				setting.SemanticReindexProcessed = int32(processedSnapshot)
				setting.SemanticReindexFailed = int32(failedSnapshot)
				setting.SemanticReindexStartedTs = startedTs
				setting.SemanticReindexUpdatedTs = time.Now().Unix()
				setting.SemanticReindexModel = model
			})
		}
	}

	_ = s.updateSemanticReindexState(ctx, func(setting *storepb.InstanceAISetting) {
		setting.SemanticReindexRunning = false
		setting.SemanticReindexTotal = int32(total)
		setting.SemanticReindexProcessed = int32(processed)
		setting.SemanticReindexFailed = int32(failed)
		setting.SemanticReindexStartedTs = startedTs
		setting.SemanticReindexUpdatedTs = time.Now().Unix()
		setting.SemanticReindexModel = model
	})
}

func (s *APIV1Service) listMemosForSemanticReindex(ctx context.Context) ([]*store.Memo, error) {
	result := make([]*store.Memo, 0, semanticSearchBatchSize)

	statuses := []store.RowStatus{store.Normal, store.Archived}
	for _, rowStatus := range statuses {
		rowStatusCopy := rowStatus
		memos, err := s.listMemosForSemanticSearch(ctx, &store.FindMemo{
			ExcludeComments: true,
			RowStatus:       &rowStatusCopy,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, memos...)
	}

	return result, nil
}

func (s *APIV1Service) updateSemanticReindexState(ctx context.Context, mutate func(*storepb.InstanceAISetting)) error {
	aiSetting, err := s.Store.GetInstanceAISetting(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get current ai setting")
	}
	if aiSetting == nil {
		aiSetting = &storepb.InstanceAISetting{}
	}

	next := proto.Clone(aiSetting).(*storepb.InstanceAISetting)
	mutate(next)

	_, err = s.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key:   storepb.InstanceSettingKey_AI,
		Value: &storepb.InstanceSetting_AiSetting{AiSetting: next},
	})
	if err != nil {
		return errors.Wrap(err, "failed to persist semantic reindex state")
	}
	return nil
}
