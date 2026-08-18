CREATE INDEX CONCURRENTLY company_codex_turn_session_time_idx
    ON company_codex_turn (session_id, completed_at ASC);
