package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/lib/pq"
	"github.com/pkg/errors"
)

// MemoEmbedding stores vector indexing data for a memo.
type MemoEmbedding struct {
	MemoID      int32
	Model       string
	Dimension   int32
	Embedding   []float64
	ContentHash string
}

func (s *Store) ensurePostgresEmbeddingSupport() error {
	if s.profile == nil || strings.ToLower(s.profile.Driver) != "postgres" {
		return errors.New("memo embedding is only supported for postgres driver")
	}
	return nil
}

func (s *Store) GetMemoEmbeddingContentHash(ctx context.Context, memoID int32) (string, error) {
	if err := s.ensurePostgresEmbeddingSupport(); err != nil {
		return "", err
	}

	const stmt = "SELECT content_hash FROM memo_embedding WHERE memo_id = $1"
	var contentHash string
	if err := s.driver.GetDB().QueryRowContext(ctx, stmt, memoID).Scan(&contentHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", errors.Wrap(err, "failed to query memo embedding content hash")
	}
	return contentHash, nil
}

func (s *Store) UpsertMemoEmbedding(ctx context.Context, embedding *MemoEmbedding) error {
	if err := s.ensurePostgresEmbeddingSupport(); err != nil {
		return err
	}
	if embedding == nil {
		return errors.New("embedding cannot be nil")
	}
	if len(embedding.Embedding) == 0 {
		return errors.New("embedding vector cannot be empty")
	}

	const stmt = `
INSERT INTO memo_embedding (memo_id, model, dimension, embedding, content_hash, updated_ts)
VALUES ($1, $2, $3, $4, $5, CAST(EXTRACT(EPOCH FROM NOW()) AS BIGINT))
ON CONFLICT (memo_id)
DO UPDATE SET
  model = EXCLUDED.model,
  dimension = EXCLUDED.dimension,
  embedding = EXCLUDED.embedding,
  content_hash = EXCLUDED.content_hash,
  updated_ts = EXCLUDED.updated_ts
`
	if _, err := s.driver.GetDB().ExecContext(
		ctx,
		stmt,
		embedding.MemoID,
		embedding.Model,
		embedding.Dimension,
		pq.Array(embedding.Embedding),
		embedding.ContentHash,
	); err != nil {
		return errors.Wrap(err, "failed to upsert memo embedding")
	}
	return nil
}

func (s *Store) DeleteMemoEmbeddingByMemoID(ctx context.Context, memoID int32) error {
	if err := s.ensurePostgresEmbeddingSupport(); err != nil {
		return err
	}

	const stmt = "DELETE FROM memo_embedding WHERE memo_id = $1"
	if _, err := s.driver.GetDB().ExecContext(ctx, stmt, memoID); err != nil {
		return errors.Wrap(err, "failed to delete memo embedding")
	}
	return nil
}

func (s *Store) ListMemoEmbeddingsByMemoIDs(ctx context.Context, memoIDList []int32) (map[int32][]float64, error) {
	if err := s.ensurePostgresEmbeddingSupport(); err != nil {
		return nil, err
	}
	result := make(map[int32][]float64, len(memoIDList))
	if len(memoIDList) == 0 {
		return result, nil
	}

	queryIDs := make([]int64, 0, len(memoIDList))
	for _, memoID := range memoIDList {
		queryIDs = append(queryIDs, int64(memoID))
	}

	const stmt = "SELECT memo_id, embedding FROM memo_embedding WHERE memo_id = ANY($1::bigint[])"
	rows, err := s.driver.GetDB().QueryContext(ctx, stmt, pq.Array(queryIDs))
	if err != nil {
		return nil, errors.Wrap(err, "failed to query memo embeddings")
	}
	defer rows.Close()

	for rows.Next() {
		var memoID int32
		var vector []float64
		if err := rows.Scan(&memoID, pq.Array(&vector)); err != nil {
			return nil, errors.Wrap(err, "failed to scan memo embedding row")
		}
		result[memoID] = vector
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "failed to iterate memo embedding rows")
	}

	return result, nil
}
