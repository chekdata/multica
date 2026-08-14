package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetTaskContextReturnsServerStampedBinding(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/task-context", nil)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-User-ID", "user-1")
	req.Header.Set("X-Agent-ID", "agent-1")
	req.Header.Set("X-Task-ID", "task-1")
	req.Header.Set("X-Workspace-ID", "workspace-1")
	w := httptest.NewRecorder()

	(&Handler{}).GetTaskContext(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var got TaskContextResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := (TaskContextResponse{
		UserID:      "user-1",
		AgentID:     "agent-1",
		TaskID:      "task-1",
		WorkspaceID: "workspace-1",
	})
	if got != want {
		t.Fatalf("response = %+v, want %+v", got, want)
	}
}

func TestGetTaskContextRejectsHumanCredentialShape(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/task-context", nil)
	// A human PAT can submit these headers, but Auth does not stamp the source.
	req.Header.Set("X-Agent-ID", "forged-agent")
	req.Header.Set("X-Task-ID", "forged-task")
	req.Header.Set("X-Workspace-ID", "forged-workspace")
	w := httptest.NewRecorder()

	(&Handler{}).GetTaskContext(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", w.Code, w.Body.String())
	}
}

func TestGetTaskContextRejectsIncompleteStampedContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/task-context", nil)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-User-ID", "user-1")
	w := httptest.NewRecorder()

	(&Handler{}).GetTaskContext(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", w.Code, w.Body.String())
	}
}
