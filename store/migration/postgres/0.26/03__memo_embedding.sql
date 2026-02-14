CREATE TABLE memo_embedding (
  memo_id INTEGER PRIMARY KEY REFERENCES memo (id) ON DELETE CASCADE,
  model TEXT NOT NULL,
  dimension INTEGER NOT NULL,
  embedding DOUBLE PRECISION[] NOT NULL,
  content_hash TEXT NOT NULL,
  updated_ts BIGINT NOT NULL DEFAULT CAST(EXTRACT(EPOCH FROM NOW()) AS BIGINT)
);

CREATE INDEX memo_embedding_updated_ts_idx ON memo_embedding (updated_ts);
