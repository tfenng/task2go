-- Effective per-run timeout, snapshotted at creation so retries and audits
-- use the limit that was actually applied.
ALTER TABLE runs ADD COLUMN timeout_seconds integer;
UPDATE runs r
SET timeout_seconds = v.timeout_seconds
FROM task_definition_versions v
WHERE v.id = r.definition_version_id;
ALTER TABLE runs ALTER COLUMN timeout_seconds SET NOT NULL;
ALTER TABLE runs ADD CONSTRAINT ck_runs_timeout
    CHECK (timeout_seconds BETWEEN 1 AND 86400);
