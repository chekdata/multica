package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestTruncateCompanyCodexTextPreservesUTF8(t *testing.T) {
	got := truncateCompanyCodexText("你好世界", 3)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateCompanyCodexText returned invalid UTF-8: %q", got)
	}
	if got != "你好世\n[truncated]" {
		t.Fatalf("truncateCompanyCodexText() = %q", got)
	}
}

func TestRequireCompanyCodexInternal(t *testing.T) {
	h := &Handler{cfg: Config{CompanyCodexBrokerSecret: "service-secret"}}

	for _, tc := range []struct {
		name   string
		secret string
		want   bool
	}{
		{name: "valid", secret: "service-secret", want: true},
		{name: "wrong", secret: "wrong", want: false},
		{name: "empty", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/internal/company-codex/access-check", strings.NewReader("{}"))
			req.Header.Set("X-Company-Codex-Secret", tc.secret)
			res := httptest.NewRecorder()
			if got := h.requireCompanyCodexInternal(res, req); got != tc.want {
				t.Fatalf("requireCompanyCodexInternal() = %v, want %v", got, tc.want)
			}
			if !tc.want && res.Code != 401 {
				t.Fatalf("status = %d, want 401", res.Code)
			}
		})
	}
}

func TestCompanyCodexQuotaStatusAndIncrease(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}

	const gatewayKeyID = "gateway-company-codex-quota-test"
	if _, err := testPool.Exec(context.Background(), `
		DELETE FROM company_codex_key WHERE workspace_id = $1 AND user_id = $2
	`, testWorkspaceID, testUserID); err != nil {
		t.Fatalf("clear company Codex key: %v", err)
	}
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO company_codex_key (
			workspace_id, user_id, gateway_key_id, gateway_key_prefix, status
		) VALUES ($1, $2, $3, 'sk-clb-quota', 'active')
	`, testWorkspaceID, testUserID, gatewayKeyID); err != nil {
		t.Fatalf("insert company Codex key: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `
			DELETE FROM company_codex_key WHERE workspace_id = $1 AND user_id = $2
		`, testWorkspaceID, testUserID)
	})

	weeklyLimit := int64(25_000_000)
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Company-Codex-Secret"); got != "test-broker-secret" {
			http.Error(w, "missing broker secret", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/internal/company-codex/keys/"+gatewayKeyID {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPatch {
			var body struct {
				WorkspaceID      string `json:"workspace_id"`
				UserID           string `json:"user_id"`
				AdditionalTokens int64  `json:"additional_tokens"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if body.WorkspaceID != testWorkspaceID || body.UserID != testUserID {
				http.Error(w, "wrong member identity", http.StatusForbidden)
				return
			}
			weeklyLimit += body.AdditionalTokens
		} else if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, companyCodexQuotaResponse{
			WeeklyTokenLimit:         weeklyLimit,
			CurrentTokens:            5_000_000,
			ResetAt:                  "2026-08-28T00:00:00Z",
			UpstreamRemainingPercent: float64Pointer(74.25),
		})
	}))
	defer broker.Close()

	h := *testHandler
	h.cfg.CompanyCodexBrokerURL = broker.URL
	h.cfg.CompanyCodexBrokerSecret = "test-broker-secret"
	memberRow, err := h.Queries.GetMemberByUserAndWorkspace(context.Background(), db.GetMemberByUserAndWorkspaceParams{
		UserID:      util.MustParseUUID(testUserID),
		WorkspaceID: util.MustParseUUID(testWorkspaceID),
	})
	if err != nil {
		t.Fatalf("load test member: %v", err)
	}
	withWorkspace := func(req *http.Request) *http.Request {
		return req.WithContext(middleware.SetMemberContext(req.Context(), testWorkspaceID, memberRow))
	}

	getResponse := httptest.NewRecorder()
	h.GetCompanyCodexKey(getResponse, withWorkspace(newRequest(http.MethodGet, "/api/company-codex/key", nil)))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GetCompanyCodexKey status = %d: %s", getResponse.Code, getResponse.Body.String())
	}
	var status companyCodexKeyStatus
	if err := json.NewDecoder(getResponse.Body).Decode(&status); err != nil {
		t.Fatalf("decode quota status: %v", err)
	}
	if status.WeeklyTokenLimit != 25_000_000 || status.CurrentTokens != 5_000_000 {
		t.Fatalf("quota status = %+v", status)
	}

	patchResponse := httptest.NewRecorder()
	h.IncreaseCompanyCodexKeyQuota(patchResponse, withWorkspace(newRequest(
		http.MethodPatch,
		"/api/company-codex/key",
		map[string]any{"additional_tokens": 7_000_000},
	)))
	if patchResponse.Code != http.StatusOK {
		t.Fatalf("IncreaseCompanyCodexKeyQuota status = %d: %s", patchResponse.Code, patchResponse.Body.String())
	}
	if err := json.NewDecoder(patchResponse.Body).Decode(&status); err != nil {
		t.Fatalf("decode increased quota: %v", err)
	}
	if status.WeeklyTokenLimit != 32_000_000 || status.KeyPrefix != "sk-clb-quota" {
		t.Fatalf("increased quota status = %+v", status)
	}
}

func TestIncreaseCompanyCodexKeyQuotaRejectsNonPositiveAmount(t *testing.T) {
	h := &Handler{}
	for _, additionalTokens := range []int64{0, -1} {
		response := httptest.NewRecorder()
		h.IncreaseCompanyCodexKeyQuota(response, newRequest(
			http.MethodPatch,
			"/api/company-codex/key",
			map[string]any{"additional_tokens": additionalTokens},
		))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("additional_tokens=%d status = %d, want 400", additionalTokens, response.Code)
		}
	}
}

func float64Pointer(value float64) *float64 {
	return &value
}
