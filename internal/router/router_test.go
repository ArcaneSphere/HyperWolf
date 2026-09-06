package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsAllowedDashboardOrigin(t *testing.T) {
	tests := []struct {
		origin string
		want   bool
	}{
		{"http://127.0.0.1:18080", true},
		{"http://localhost:18080", true},
		{"https://[::1]:18080", true},
		{"http://127.0.0.1:18081", false},
		{"https://example.com:18080", false},
		{"http://localhost.evil:18080", false},
		{"http://localhost:18080/path", false},
	}

	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			if got := isAllowedDashboardOrigin(tt.origin, "127.0.0.1:18080"); got != tt.want {
				t.Fatalf("isAllowedDashboardOrigin(%q) = %v, want %v", tt.origin, got, tt.want)
			}
		})
	}
}

func TestLocalOriginMiddlewareRejectsExternalAPIOrigin(t *testing.T) {
	handler := localOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler called for rejected origin")
	}), "127.0.0.1:18080")

	req := httptest.NewRequest(http.MethodPost, "/api/set_node", nil)
	req.Header.Set("Origin", "https://example.com")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusForbidden)
	}
}

func TestLocalOriginMiddlewareAllowsDashboardAndOriginlessRequests(t *testing.T) {
	handler := localOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "127.0.0.1:18080")

	tests := []struct {
		name   string
		origin string
	}{
		{name: "dashboard", origin: "http://localhost:18080"},
		{name: "originless", origin: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/set_node", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			if resp.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
			}
		})
	}
}

func TestLocalOriginMiddlewareDoesNotBlockStaticContent(t *testing.T) {
	handler := localOriginMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "127.0.0.1:18080")

	req := httptest.NewRequest(http.MethodGet, "/dashboard.js", nil)
	req.Header.Set("Origin", "https://example.com")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
}
