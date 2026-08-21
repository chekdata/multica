package handler

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const companyCodexBodyLimit = 2 << 20
const companyCodexMaxSafeInteger = int64(9007199254740991)

type companyCodexKeyStatus struct {
	Active                   bool     `json:"active"`
	KeyPrefix                string   `json:"key_prefix,omitempty"`
	CreatedAt                string   `json:"created_at,omitempty"`
	WeeklyTokenLimit         int64    `json:"weekly_token_limit,omitempty"`
	CurrentTokens            int64    `json:"current_tokens,omitempty"`
	ResetAt                  string   `json:"reset_at,omitempty"`
	UpstreamRemainingPercent *float64 `json:"upstream_remaining_percent,omitempty"`
}

type companyCodexBrokerCreateResponse struct {
	GatewayKeyID     string `json:"gateway_key_id"`
	GatewayKeyPrefix string `json:"gateway_key_prefix"`
	WeeklyTokenLimit int64  `json:"weekly_token_limit"`
	Credential       string `json:"credential"`
}

type companyCodexQuotaResponse struct {
	WeeklyTokenLimit         int64    `json:"weekly_token_limit"`
	CurrentTokens            int64    `json:"current_tokens"`
	ResetAt                  string   `json:"reset_at"`
	UpstreamRemainingPercent *float64 `json:"upstream_remaining_percent"`
}

type companyCodexQuotaIncreaseRequest struct {
	AdditionalTokens int64 `json:"additional_tokens"`
}

type companyCodexCreateResponse struct {
	companyCodexKeyStatus
	Credential   string `json:"credential"`
	CCSwitchURL  string `json:"cc_switch_url"`
	ConfigTOML   string `json:"config_toml"`
	SetupCommand string `json:"setup_command"`
}

type companyCodexSessionSummary struct {
	ID                string `json:"id"`
	UserName          string `json:"user_name"`
	UserEmail         string `json:"user_email"`
	ClientSessionID   string `json:"client_session_id"`
	ClientThreadID    string `json:"client_thread_id,omitempty"`
	Title             string `json:"title"`
	Model             string `json:"model,omitempty"`
	Status            string `json:"status"`
	InputTokens       int64  `json:"input_tokens"`
	OutputTokens      int64  `json:"output_tokens"`
	CachedInputTokens int64  `json:"cached_input_tokens"`
	StartedAt         string `json:"started_at"`
	LastActivityAt    string `json:"last_activity_at"`
	LastPrompt        string `json:"last_prompt,omitempty"`
	LastResponse      string `json:"last_response,omitempty"`
}

type companyCodexTurnResponse struct {
	ID                string `json:"id"`
	RequestID         string `json:"request_id"`
	Prompt            string `json:"prompt"`
	Response          string `json:"response"`
	Model             string `json:"model,omitempty"`
	Status            string `json:"status"`
	InputTokens       int64  `json:"input_tokens"`
	OutputTokens      int64  `json:"output_tokens"`
	CachedInputTokens int64  `json:"cached_input_tokens"`
	StartedAt         string `json:"started_at"`
	CompletedAt       string `json:"completed_at"`
}

type companyCodexSessionDetail struct {
	Session companyCodexSessionSummary `json:"session"`
	Turns   []companyCodexTurnResponse `json:"turns"`
}

type companyCodexAccessRequest struct {
	UserID       string `json:"user_id"`
	WorkspaceID  string `json:"workspace_id"`
	GatewayKeyID string `json:"gateway_key_id"`
}

type companyCodexTaskAccessRequest struct {
	UserID      string `json:"user_id"`
	WorkspaceID string `json:"workspace_id"`
	AgentID     string `json:"agent_id"`
	TaskID      string `json:"task_id"`
}

type companyCodexIngestRequest struct {
	companyCodexAccessRequest
	ClientSessionID   string    `json:"client_session_id"`
	ClientThreadID    string    `json:"client_thread_id"`
	RequestID         string    `json:"request_id"`
	Prompt            string    `json:"prompt"`
	Response          string    `json:"response"`
	Model             string    `json:"model"`
	Status            string    `json:"status"`
	InputTokens       int64     `json:"input_tokens"`
	OutputTokens      int64     `json:"output_tokens"`
	CachedInputTokens int64     `json:"cached_input_tokens"`
	StartedAt         time.Time `json:"started_at"`
	CompletedAt       time.Time `json:"completed_at"`
}

func (h *Handler) companyCodexConfigured() bool {
	return h.cfg.CompanyCodexBrokerURL != "" && h.cfg.CompanyCodexBrokerSecret != ""
}

func (h *Handler) requireCompanyCodexInternal(w http.ResponseWriter, r *http.Request) bool {
	expected := []byte(h.cfg.CompanyCodexBrokerSecret)
	provided := []byte(strings.TrimSpace(r.Header.Get("X-Company-Codex-Secret")))
	if len(expected) == 0 || len(expected) != len(provided) || subtle.ConstantTimeCompare(expected, provided) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid company Codex service credential")
		return false
	}
	return true
}

func decodeCompanyCodexJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, companyCodexBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func (h *Handler) companyCodexBrokerRequest(ctx context.Context, method, path string, payload, dst any) error {
	if !h.companyCodexConfigured() {
		return errors.New("company Codex broker is not configured")
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, h.cfg.CompanyCodexBrokerURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Company-Codex-Secret", h.cfg.CompanyCodexBrokerSecret)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("company Codex broker returned %d", resp.StatusCode)
	}
	if dst != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, dst); err != nil {
			return fmt.Errorf("decode company Codex broker response: %w", err)
		}
	}
	return nil
}

func (h *Handler) deleteCompanyCodexGatewayKeys(ctx context.Context, gatewayKeyIDs []string) {
	for _, gatewayKeyID := range gatewayKeyIDs {
		if err := h.companyCodexBrokerRequest(ctx, http.MethodDelete,
			"/internal/company-codex/keys/"+url.PathEscape(gatewayKeyID), nil, nil); err != nil {
			slog.Warn("company Codex upstream key cleanup failed", "error", err, "gateway_key_id", gatewayKeyID)
		}
	}
}

func (h *Handler) GetCompanyCodexKey(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	var gatewayKeyID, prefix string
	var createdAt time.Time
	err := h.DB.QueryRow(r.Context(), `
		SELECT gateway_key_id, gateway_key_prefix, created_at
		FROM company_codex_key
		WHERE workspace_id = $1 AND user_id = $2 AND status = 'active'
	`, workspaceID, userID).Scan(&gatewayKeyID, &prefix, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, companyCodexKeyStatus{Active: false})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load company Codex access")
		return
	}
	status := companyCodexKeyStatus{
		Active:    true,
		KeyPrefix: prefix,
		CreatedAt: createdAt.UTC().Format(time.RFC3339),
	}
	var quota companyCodexQuotaResponse
	if err := h.companyCodexBrokerRequest(r.Context(), http.MethodGet,
		"/internal/company-codex/keys/"+url.PathEscape(gatewayKeyID), nil, &quota); err != nil {
		slog.Warn("company Codex quota lookup failed", "error", err, "gateway_key_id", gatewayKeyID)
	} else {
		status.WeeklyTokenLimit = quota.WeeklyTokenLimit
		status.CurrentTokens = quota.CurrentTokens
		status.ResetAt = quota.ResetAt
		status.UpstreamRemainingPercent = quota.UpstreamRemainingPercent
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) CreateCompanyCodexKey(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	if !h.companyCodexConfigured() {
		writeError(w, http.StatusServiceUnavailable, "company Codex access is not configured")
		return
	}

	user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load current user")
		return
	}
	var previousGatewayKeyID string
	err = h.DB.QueryRow(r.Context(), `
		SELECT gateway_key_id
		FROM company_codex_key
		WHERE workspace_id = $1 AND user_id = $2
	`, workspaceID, userID).Scan(&previousGatewayKeyID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to load company Codex access")
		return
	}

	var created companyCodexBrokerCreateResponse
	err = h.companyCodexBrokerRequest(r.Context(), http.MethodPost, "/internal/company-codex/keys", map[string]any{
		"workspace_id":          workspaceID,
		"user_id":               userID,
		"email":                 user.Email,
		"name":                  user.Name,
		"revoke_gateway_key_id": previousGatewayKeyID,
	}, &created)
	if err != nil || created.GatewayKeyID == "" || created.Credential == "" {
		writeError(w, http.StatusBadGateway, "company Codex credential is temporarily unavailable")
		return
	}

	var storedCreatedAt time.Time
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO company_codex_key (
			workspace_id, user_id, gateway_key_id, gateway_key_prefix, status
		) VALUES ($1, $2, $3, $4, 'active')
		ON CONFLICT (workspace_id, user_id) DO UPDATE SET
			gateway_key_id = EXCLUDED.gateway_key_id,
			gateway_key_prefix = EXCLUDED.gateway_key_prefix,
			status = 'active',
			created_at = now(),
			updated_at = now(),
			revoked_at = NULL
		RETURNING created_at
	`, workspaceID, userID, created.GatewayKeyID, created.GatewayKeyPrefix).Scan(&storedCreatedAt)
	if err != nil {
		_ = h.companyCodexBrokerRequest(context.Background(), http.MethodDelete,
			"/internal/company-codex/keys/"+url.PathEscape(created.GatewayKeyID), nil, nil)
		writeError(w, http.StatusInternalServerError, "failed to save company Codex access")
		return
	}

	baseURL := h.cfg.CompanyCodexBaseURL
	if baseURL == "" {
		baseURL = "https://codex.chekkk.com"
	}
	model := h.cfg.CompanyCodexDefaultModel
	if model == "" {
		model = "gpt-5.4"
	}
	deepLink := url.URL{Scheme: "ccswitch", Host: "v1", Path: "/import"}
	params := url.Values{
		"resource": {"provider"},
		"app":      {"codex"},
		"name":     {"CHEK Company Codex"},
		"endpoint": {baseURL + "/v1"},
		"apiKey":   {created.Credential},
		"homepage": {h.cfg.PublicURL},
		"model":    {model},
		"enabled":  {"true"},
		"notes":    {"Managed by CHEK Multica; conversations are synchronized for company audit."},
	}
	deepLink.RawQuery = params.Encode()
	configTOML := fmt.Sprintf(`model_provider = "chek"
model = %q

[model_providers.chek]
name = "CHEK Company Codex"
base_url = %q
wire_api = "responses"
requires_openai_auth = true
`, model, baseURL+"/v1")

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, companyCodexCreateResponse{
		companyCodexKeyStatus: companyCodexKeyStatus{
			Active:           true,
			KeyPrefix:        created.GatewayKeyPrefix,
			CreatedAt:        storedCreatedAt.UTC().Format(time.RFC3339),
			WeeklyTokenLimit: created.WeeklyTokenLimit,
		},
		Credential:   created.Credential,
		CCSwitchURL:  deepLink.String(),
		ConfigTOML:   configTOML,
		SetupCommand: "poolctl gui configure",
	})
}

func (h *Handler) IncreaseCompanyCodexKeyQuota(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	var requested companyCodexQuotaIncreaseRequest
	if !decodeCompanyCodexJSON(w, r, &requested) {
		return
	}
	if requested.AdditionalTokens <= 0 || requested.AdditionalTokens > companyCodexMaxSafeInteger {
		writeError(w, http.StatusBadRequest, "additional_tokens must be a positive safe integer")
		return
	}

	var gatewayKeyID, prefix string
	var createdAt time.Time
	err := h.DB.QueryRow(r.Context(), `
		SELECT gateway_key_id, gateway_key_prefix, created_at
		FROM company_codex_key
		WHERE workspace_id = $1 AND user_id = $2 AND status = 'active'
	`, workspaceID, userID).Scan(&gatewayKeyID, &prefix, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "active company Codex access required")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load company Codex access")
		return
	}

	var quota companyCodexQuotaResponse
	err = h.companyCodexBrokerRequest(r.Context(), http.MethodPatch,
		"/internal/company-codex/keys/"+url.PathEscape(gatewayKeyID), map[string]any{
			"workspace_id":      workspaceID,
			"user_id":           userID,
			"additional_tokens": requested.AdditionalTokens,
		}, &quota)
	if err != nil {
		slog.Warn("company Codex quota increase failed", "error", err, "gateway_key_id", gatewayKeyID)
		writeError(w, http.StatusBadGateway, "company Codex quota is temporarily unavailable")
		return
	}
	slog.Info("company Codex quota increased",
		"user_id", userID,
		"workspace_id", workspaceID,
		"gateway_key_id", gatewayKeyID,
		"additional_tokens", requested.AdditionalTokens,
		"weekly_token_limit", quota.WeeklyTokenLimit,
	)
	writeJSON(w, http.StatusOK, companyCodexKeyStatus{
		Active:                   true,
		KeyPrefix:                prefix,
		CreatedAt:                createdAt.UTC().Format(time.RFC3339),
		WeeklyTokenLimit:         quota.WeeklyTokenLimit,
		CurrentTokens:            quota.CurrentTokens,
		ResetAt:                  quota.ResetAt,
		UpstreamRemainingPercent: quota.UpstreamRemainingPercent,
	})
}

func (h *Handler) RevokeCompanyCodexKey(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	var gatewayKeyID string
	err := h.DB.QueryRow(r.Context(), `
		UPDATE company_codex_key
		SET status = 'revoked', revoked_at = now(), updated_at = now()
		WHERE workspace_id = $1 AND user_id = $2 AND status = 'active'
		RETURNING gateway_key_id
	`, workspaceID, userID).Scan(&gatewayKeyID)
	if errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke company Codex access")
		return
	}
	if err := h.companyCodexBrokerRequest(r.Context(), http.MethodDelete,
		"/internal/company-codex/keys/"+url.PathEscape(gatewayKeyID), nil, nil); err != nil {
		// The wrapper is already denied by the database state. Return success so
		// users are not encouraged to keep using a locally cached credential.
		slog.Warn("company Codex upstream key cleanup failed", "error", err, "gateway_key_id", gatewayKeyID)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListCompanyCodexSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	member, ok := ctxMember(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "workspace membership required")
		return
	}
	showAll := member.Role == "owner" || member.Role == "admin"
	rows, err := h.DB.Query(r.Context(), `
		SELECT s.id::text, u.name, u.email, s.client_session_id,
		       COALESCE(s.client_thread_id, ''), COALESCE(s.title, ''),
		       COALESCE(s.model, ''), s.status, s.input_tokens,
		       s.output_tokens, s.cached_input_tokens, s.started_at,
		       s.last_activity_at, COALESCE(last_turn.prompt, ''),
		       COALESCE(last_turn.response, '')
		FROM company_codex_session s
		JOIN "user" u ON u.id = s.user_id
		LEFT JOIN LATERAL (
			SELECT prompt, response
			FROM company_codex_turn
			WHERE session_id = s.id
			ORDER BY completed_at DESC
			LIMIT 1
		) last_turn ON true
		WHERE s.workspace_id = $1 AND ($2::boolean OR s.user_id = $3)
		ORDER BY s.last_activity_at DESC
		LIMIT 100
	`, workspaceID, showAll, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list company Codex sessions")
		return
	}
	defer rows.Close()

	sessions := make([]companyCodexSessionSummary, 0)
	for rows.Next() {
		var item companyCodexSessionSummary
		var startedAt, lastActivityAt time.Time
		if err := rows.Scan(
			&item.ID, &item.UserName, &item.UserEmail, &item.ClientSessionID,
			&item.ClientThreadID, &item.Title, &item.Model, &item.Status,
			&item.InputTokens, &item.OutputTokens, &item.CachedInputTokens,
			&startedAt, &lastActivityAt, &item.LastPrompt, &item.LastResponse,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read company Codex sessions")
			return
		}
		item.StartedAt = startedAt.UTC().Format(time.RFC3339)
		item.LastActivityAt = lastActivityAt.UTC().Format(time.RFC3339)
		sessions = append(sessions, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read company Codex sessions")
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (h *Handler) GetCompanyCodexSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := ctxWorkspaceID(r.Context())
	member, ok := ctxMember(r.Context())
	if !ok {
		writeError(w, http.StatusForbidden, "workspace membership required")
		return
	}
	showAll := member.Role == "owner" || member.Role == "admin"
	sessionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "session id")
	if !ok {
		return
	}
	var summary companyCodexSessionSummary
	var startedAt, lastActivityAt time.Time
	err := h.DB.QueryRow(r.Context(), `
		SELECT s.id::text, u.name, u.email, s.client_session_id,
		       COALESCE(s.client_thread_id, ''), COALESCE(s.title, ''),
		       COALESCE(s.model, ''), s.status, s.input_tokens,
		       s.output_tokens, s.cached_input_tokens, s.started_at,
		       s.last_activity_at, '', ''
		FROM company_codex_session s
		JOIN "user" u ON u.id = s.user_id
		WHERE s.id = $1 AND s.workspace_id = $2
		  AND ($3::boolean OR s.user_id = $4)
	`, sessionID, workspaceID, showAll, userID).Scan(
		&summary.ID, &summary.UserName, &summary.UserEmail, &summary.ClientSessionID,
		&summary.ClientThreadID, &summary.Title, &summary.Model, &summary.Status,
		&summary.InputTokens, &summary.OutputTokens, &summary.CachedInputTokens,
		&startedAt, &lastActivityAt, &summary.LastPrompt, &summary.LastResponse,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "company Codex session not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load company Codex session")
		return
	}
	summary.StartedAt = startedAt.UTC().Format(time.RFC3339)
	summary.LastActivityAt = lastActivityAt.UTC().Format(time.RFC3339)

	rows, err := h.DB.Query(r.Context(), `
		SELECT id::text, request_id, prompt, response, COALESCE(model, ''),
		       status, input_tokens, output_tokens, cached_input_tokens,
		       started_at, completed_at
		FROM company_codex_turn
		WHERE session_id = $1
		ORDER BY completed_at ASC
		LIMIT 500
	`, sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load company Codex turns")
		return
	}
	defer rows.Close()
	turns := make([]companyCodexTurnResponse, 0)
	for rows.Next() {
		var turn companyCodexTurnResponse
		var turnStartedAt, completedAt time.Time
		if err := rows.Scan(
			&turn.ID, &turn.RequestID, &turn.Prompt, &turn.Response, &turn.Model,
			&turn.Status, &turn.InputTokens, &turn.OutputTokens,
			&turn.CachedInputTokens, &turnStartedAt, &completedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read company Codex turns")
			return
		}
		turn.StartedAt = turnStartedAt.UTC().Format(time.RFC3339)
		turn.CompletedAt = completedAt.UTC().Format(time.RFC3339)
		turns = append(turns, turn)
	}
	writeJSON(w, http.StatusOK, companyCodexSessionDetail{Session: summary, Turns: turns})
}

func (h *Handler) CheckCompanyCodexAccess(w http.ResponseWriter, r *http.Request) {
	if !h.requireCompanyCodexInternal(w, r) {
		return
	}
	var req companyCodexAccessRequest
	if !decodeCompanyCodexJSON(w, r, &req) {
		return
	}
	userID, ok := parseUUIDOrBadRequest(w, req.UserID, "user_id")
	if !ok {
		return
	}
	workspaceID, ok := parseUUIDOrBadRequest(w, req.WorkspaceID, "workspace_id")
	if !ok {
		return
	}
	if strings.TrimSpace(req.GatewayKeyID) == "" || len(req.GatewayKeyID) > 512 {
		writeError(w, http.StatusBadRequest, "invalid gateway_key_id")
		return
	}
	var active bool
	err := h.DB.QueryRow(r.Context(), `
		SELECT EXISTS (
			SELECT 1
			FROM company_codex_key k
			JOIN member m ON m.workspace_id = k.workspace_id AND m.user_id = k.user_id
			WHERE k.workspace_id = $1 AND k.user_id = $2
			  AND k.gateway_key_id = $3 AND k.status = 'active'
		)
	`, workspaceID, userID, req.GatewayKeyID).Scan(&active)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify company Codex access")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"active": active})
}

func (h *Handler) CheckCompanyCodexTaskAccess(w http.ResponseWriter, r *http.Request) {
	if !h.requireCompanyCodexInternal(w, r) {
		return
	}
	var req companyCodexTaskAccessRequest
	if !decodeCompanyCodexJSON(w, r, &req) {
		return
	}
	userID, ok := parseUUIDOrBadRequest(w, req.UserID, "user_id")
	if !ok {
		return
	}
	workspaceID, ok := parseUUIDOrBadRequest(w, req.WorkspaceID, "workspace_id")
	if !ok {
		return
	}
	agentID, ok := parseUUIDOrBadRequest(w, req.AgentID, "agent_id")
	if !ok {
		return
	}
	taskID, ok := parseUUIDOrBadRequest(w, req.TaskID, "task_id")
	if !ok {
		return
	}
	var active bool
	err := h.DB.QueryRow(r.Context(), `
		SELECT EXISTS (
			SELECT 1
			FROM agent_task_queue task
			JOIN agent a ON a.id = task.agent_id
			JOIN member m ON m.workspace_id = a.workspace_id AND m.user_id = $1
			WHERE task.id = $2 AND task.agent_id = $3 AND a.workspace_id = $4
			  AND task.status IN ('queued', 'dispatched', 'running', 'waiting_local_directory', 'deferred')
		)
	`, userID, taskID, agentID, workspaceID).Scan(&active)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify company Codex task access")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"active": active})
}

func (h *Handler) IngestCompanyCodexTurn(w http.ResponseWriter, r *http.Request) {
	if !h.requireCompanyCodexInternal(w, r) {
		return
	}
	var req companyCodexIngestRequest
	if !decodeCompanyCodexJSON(w, r, &req) {
		return
	}
	userID, ok := parseUUIDOrBadRequest(w, req.UserID, "user_id")
	if !ok {
		return
	}
	workspaceID, ok := parseUUIDOrBadRequest(w, req.WorkspaceID, "workspace_id")
	if !ok {
		return
	}
	req.ClientSessionID = strings.TrimSpace(req.ClientSessionID)
	req.ClientThreadID = strings.TrimSpace(req.ClientThreadID)
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.GatewayKeyID = strings.TrimSpace(req.GatewayKeyID)
	if req.GatewayKeyID == "" || req.ClientSessionID == "" || req.RequestID == "" ||
		len(req.GatewayKeyID) > 512 || len(req.ClientSessionID) > 512 ||
		len(req.ClientThreadID) > 512 || len(req.RequestID) > 512 {
		writeError(w, http.StatusBadRequest, "incomplete company Codex turn")
		return
	}
	if req.Status != "failed" {
		req.Status = "completed"
	}
	if req.StartedAt.IsZero() {
		req.StartedAt = time.Now().UTC()
	}
	if req.CompletedAt.IsZero() {
		req.CompletedAt = time.Now().UTC()
	}
	req.Prompt = truncateCompanyCodexText(req.Prompt, 250000)
	req.Response = truncateCompanyCodexText(req.Response, 500000)
	title := truncateCompanyCodexText(strings.TrimSpace(req.Prompt), 120)
	if title == "" {
		title = "Codex GUI session"
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save company Codex turn")
		return
	}
	defer tx.Rollback(r.Context())
	var sessionID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO company_codex_session (
			workspace_id, user_id, gateway_key_id, client_session_id,
			client_thread_id, title, model, status, started_at, last_activity_at
		)
		SELECT $1, $2, $3, $4, NULLIF($5, ''), $6, NULLIF($7, ''), $8, $9, $10
		WHERE EXISTS (
			SELECT 1
			FROM company_codex_key k
			JOIN member m ON m.workspace_id = k.workspace_id AND m.user_id = k.user_id
			WHERE k.workspace_id = $1 AND k.user_id = $2
			  AND k.gateway_key_id = $3 AND k.status = 'active'
		)
		ON CONFLICT (workspace_id, user_id, client_session_id) DO UPDATE SET
			client_thread_id = COALESCE(EXCLUDED.client_thread_id, company_codex_session.client_thread_id),
			title = CASE
				WHEN company_codex_session.title = 'Codex GUI session' THEN EXCLUDED.title
				ELSE company_codex_session.title
			END,
			model = COALESCE(EXCLUDED.model, company_codex_session.model),
			status = EXCLUDED.status,
			last_activity_at = GREATEST(company_codex_session.last_activity_at, EXCLUDED.last_activity_at)
		RETURNING id::text
	`, workspaceID, userID, req.GatewayKeyID, req.ClientSessionID,
		req.ClientThreadID, title, req.Model, req.Status, req.StartedAt, req.CompletedAt).Scan(&sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusForbidden, "company Codex access is no longer active")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save company Codex session")
		return
	}

	_, err = tx.Exec(r.Context(), `
		INSERT INTO company_codex_turn (
			session_id, request_id, prompt, response, model, status,
			input_tokens, output_tokens, cached_input_tokens, started_at, completed_at
		) VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, $10, $11)
		ON CONFLICT (session_id, request_id) DO UPDATE SET
			prompt = COALESCE(NULLIF(EXCLUDED.prompt, ''), company_codex_turn.prompt),
			response = COALESCE(NULLIF(EXCLUDED.response, ''), company_codex_turn.response),
			model = EXCLUDED.model,
			status = EXCLUDED.status,
			input_tokens = EXCLUDED.input_tokens,
			output_tokens = EXCLUDED.output_tokens,
			cached_input_tokens = EXCLUDED.cached_input_tokens,
			completed_at = EXCLUDED.completed_at
	`, sessionID, req.RequestID, req.Prompt, req.Response, req.Model, req.Status,
		req.InputTokens, req.OutputTokens, req.CachedInputTokens, req.StartedAt, req.CompletedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save company Codex turn")
		return
	}
	_, err = tx.Exec(r.Context(), `
		UPDATE company_codex_session s SET
			input_tokens = totals.input_tokens,
			output_tokens = totals.output_tokens,
			cached_input_tokens = totals.cached_input_tokens,
			last_activity_at = totals.last_activity_at,
			status = $2,
			model = COALESCE(NULLIF($3, ''), s.model)
		FROM (
			SELECT COALESCE(SUM(input_tokens), 0) AS input_tokens,
			       COALESCE(SUM(output_tokens), 0) AS output_tokens,
			       COALESCE(SUM(cached_input_tokens), 0) AS cached_input_tokens,
			       MAX(completed_at) AS last_activity_at
			FROM company_codex_turn WHERE session_id = $1
		) totals
		WHERE s.id = $1
	`, sessionID, req.Status, req.Model)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update company Codex session")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save company Codex turn")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"session_id": sessionID})
}

func truncateCompanyCodexText(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "\n[truncated]"
}
