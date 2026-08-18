CREATE UNIQUE INDEX CONCURRENTLY company_codex_session_identity_unique_idx
    ON company_codex_session (workspace_id, user_id, client_session_id);
