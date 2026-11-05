-- Migration: saga steps retry (Walking Skeleton - Step 6)
ALTER TABLE saga_steps ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE saga_steps ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMP;
CREATE INDEX IF NOT EXISTS idx_saga_steps_retry ON saga_steps (next_retry_at, retry_count);