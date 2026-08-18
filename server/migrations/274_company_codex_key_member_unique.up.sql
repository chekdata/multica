CREATE UNIQUE INDEX CONCURRENTLY company_codex_key_member_unique_idx
    ON company_codex_key (workspace_id, user_id);
