package tela

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
)

const testSCID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestProxyManagerForwardsLoadedTELAPath(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/assets/app.js" {
			t.Errorf("upstream path = %q, want %q", r.URL.Path, "/assets/app.js")
		}
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte("console.log('tela');"))
	}))
	defer backend.Close()

	target, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatalf("parse backend URL: %v", err)
	}
	pm := NewProxyManager(0, nil)
	pm.proxies[testSCID] = httputil.NewSingleHostReverseProxy(target)

	req := httptest.NewRequest(http.MethodGet, "/tela/"+testSCID+"/assets/app.js", nil)
	resp := httptest.NewRecorder()
	pm.handleTELA(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Result().Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if string(body) != "console.log('tela');" {
		t.Fatalf("body = %q", body)
	}
}

func TestProxyManagerRejectsUnloadedSCID(t *testing.T) {
	pm := NewProxyManager(0, nil)
	req := httptest.NewRequest(http.MethodGet, "/tela/"+testSCID+"/index.html", nil)
	resp := httptest.NewRecorder()
	pm.handleTELA(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNotFound)
	}
}

func TestProxyManagerRedirectsShardedEntrypoint(t *testing.T) {
	pm := NewProxyManager(0, nil)
	pm.proxies[testSCID] = &httputil.ReverseProxy{}
	pm.sharded[testSCID] = true
	pm.entries[testSCID] = "app index.html"

	req := httptest.NewRequest(http.MethodGet, "/tela/"+testSCID+"/", nil)
	resp := httptest.NewRecorder()
	pm.handleTELA(resp, req)

	if resp.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusFound)
	}
	want := "/tela/" + testSCID + "/app%20index.html"
	got := resp.Header().Get("Location")
	if got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}
