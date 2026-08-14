package handler

import "net/http"

// TaskContextResponse is an attestation derived exclusively from the current
// mat_ task token. The auth middleware overwrites these headers from the token
// row, so callers cannot substitute another task, agent, or workspace.
type TaskContextResponse struct {
	UserID      string `json:"user_id"`
	AgentID     string `json:"agent_id"`
	TaskID      string `json:"task_id"`
	WorkspaceID string `json:"workspace_id"`
}

// GetTaskContext exposes the server-bound identity of the current task token.
// Human JWTs and personal access tokens are rejected even if they forge the
// context headers because only Auth may stamp X-Actor-Source=task_token.
func (h *Handler) GetTaskContext(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Actor-Source") != "task_token" {
		writeError(w, http.StatusForbidden, "task context requires a task token")
		return
	}

	context := TaskContextResponse{
		UserID:      r.Header.Get("X-User-ID"),
		AgentID:     r.Header.Get("X-Agent-ID"),
		TaskID:      r.Header.Get("X-Task-ID"),
		WorkspaceID: r.Header.Get("X-Workspace-ID"),
	}
	if context.UserID == "" || context.AgentID == "" || context.TaskID == "" || context.WorkspaceID == "" {
		writeError(w, http.StatusUnauthorized, "task token context is incomplete")
		return
	}

	writeJSON(w, http.StatusOK, context)
}
