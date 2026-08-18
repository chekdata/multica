CREATE INDEX CONCURRENTLY company_codex_session_activity_idx
    ON company_codex_session (workspace_id, last_activity_at DESC);
