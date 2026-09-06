package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsLoopbackOrigin(t *testing.T) {
	tests := []struct {
		origin string
		want   bool
	}{
		{"http://127.0.0.1:18080", true},
		{"https://localhost:443", true},
		{"http://[::1]:6000", true},
		{"https://example.com", false},
		{"http://localhost.example", false},
		{"http://127.0.0.1.evil.example", false},
		{"http://localhost/path", false},
	}

	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			if got := isLoopbackOrigin(tt.origin); got != tt.want {
				t.Fatalf("isLoopbackOrigin(%q) = %v, want %v", tt.origin, got, tt.want)
			}
		})
	}
}

func TestCORSMiddlewareAllowsLoopbackOrigin(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/getinfo", nil)
	req.Header.Set("Origin", "http://127.0.0.1:18080")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:18080" {
		t.Fatalf("allow-origin = %q", got)
	}
}

func TestCORSMiddlewareRejectsExternalPreflight(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler called for rejected preflight")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/getinfo", nil)
	req.Header.Set("Origin", "https://example.com")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusForbidden)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("external origin received allow-origin %q", got)
	}
}

func TestCORSMiddlewareLeavesNonBrowserRequestsUsable(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/getinfo", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("request without Origin received allow-origin %q", got)
	}
}
