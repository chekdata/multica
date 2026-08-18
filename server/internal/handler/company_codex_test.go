package handler

import (
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
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
