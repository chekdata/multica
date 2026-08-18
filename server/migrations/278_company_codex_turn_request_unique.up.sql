CREATE UNIQUE INDEX CONCURRENTLY company_codex_turn_request_unique_idx
    ON company_codex_turn (session_id, request_id);
