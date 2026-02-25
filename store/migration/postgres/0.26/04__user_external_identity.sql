CREATE TABLE user_external_identity (
  provider TEXT NOT NULL,
  subject TEXT NOT NULL,
  user_id INTEGER NOT NULL REFERENCES "user" (id) ON DELETE CASCADE,
  email TEXT NOT NULL DEFAULT '',
  created_ts BIGINT NOT NULL DEFAULT CAST(EXTRACT(EPOCH FROM NOW()) AS BIGINT),
  updated_ts BIGINT NOT NULL DEFAULT CAST(EXTRACT(EPOCH FROM NOW()) AS BIGINT),
  PRIMARY KEY (provider, subject)
);

CREATE INDEX user_external_identity_user_id_idx ON user_external_identity (user_id);
