package router

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func newContractTestServer(t *testing.T) *http.Server {
	t.Helper()
	h := &Handlers{
		Hub:        NewHub(),
		DBDir:      filepath.Join(t.TempDir(), "gnomondb"),
		TelaPort:   18081,
		GnomonPort: 18082,
	}
	webFS := fstest.MapFS{
		"web/index.html": &fstest.MapFile{Data: []byte("<html>test</html>")},
	}
	return NewServer("127.0.0.1:18080", webFS, h)
}

func performRequest(server *http.Server, method, target string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, body)
	resp := httptest.NewRecorder()
	server.Handler.ServeHTTP(resp, req)
	return resp
}

func TestDashboardAPIConfigAndOriginProtection(t *testing.T) {
	server := newContractTestServer(t)

	allowed := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	allowed.Header.Set("Origin", "http://127.0.0.1:18080")
	allowedResp := httptest.NewRecorder()
	server.Handler.ServeHTTP(allowedResp, allowed)
	if allowedResp.Code != http.StatusOK {
		t.Fatalf("allowed config status = %d, want %d", allowedResp.Code, http.StatusOK)
	}
	if got := allowedResp.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("dashboard response unexpectedly exposed CORS header: %q", got)
	}

	denied := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	denied.Header.Set("Origin", "https://example.com")
	deniedResp := httptest.NewRecorder()
	server.Handler.ServeHTTP(deniedResp, denied)
	if deniedResp.Code != http.StatusForbidden {
		t.Fatalf("external config status = %d, want %d", deniedResp.Code, http.StatusForbidden)
	}
}

func TestDashboardAPIRejectsInvalidInputs(t *testing.T) {
	server := newContractTestServer(t)

	setNode := performRequest(server, http.MethodPost, "/api/set_node", bytes.NewBufferString(`{"node":""}`))
	if setNode.Code != http.StatusOK {
		t.Fatalf("set_node status = %d, want %d", setNode.Code, http.StatusOK)
	}
	var nodeResp ctrlResp
	if err := json.Unmarshal(setNode.Body.Bytes(), &nodeResp); err != nil {
		t.Fatalf("decode set_node response: %v", err)
	}
	if nodeResp.OK || nodeResp.Error == "" {
		t.Fatalf("set_node response = %#v, want failed envelope", nodeResp)
	}

	load := performRequest(server, http.MethodPost, "/api/load_scid", bytes.NewBufferString(`{"scid":"not-a-scid"}`))
	if load.Code != http.StatusOK {
		t.Fatalf("load_scid status = %d, want %d", load.Code, http.StatusOK)
	}
	var loadResp ctrlResp
	if err := json.Unmarshal(load.Body.Bytes(), &loadResp); err != nil {
		t.Fatalf("decode load_scid response: %v", err)
	}
	if loadResp.OK || loadResp.Error != "invalid SCID" {
		t.Fatalf("load_scid response = %#v, want invalid SCID failure", loadResp)
	}
}

func TestDashboardSettingsRoundTrip(t *testing.T) {
	server := newContractTestServer(t)
	body := bytes.NewBufferString(`{"settings":{"defaultNode":"127.0.0.1:10102","autoConnect":false}}`)
	save := performRequest(server, http.MethodPost, "/api/settings", body)
	if save.Code != http.StatusOK {
		t.Fatalf("settings save status = %d, want %d", save.Code, http.StatusOK)
	}

	get := performRequest(server, http.MethodGet, "/api/settings", nil)
	if get.Code != http.StatusOK {
		t.Fatalf("settings get status = %d, want %d", get.Code, http.StatusOK)
	}
	var response ctrlResp
	if err := json.Unmarshal(get.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	if !response.OK {
		t.Fatalf("settings response failed: %#v", response)
	}
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("settings result type = %T, want object", response.Result)
	}
	settings, ok := result["settings"].(map[string]interface{})
	if !ok {
		t.Fatalf("settings payload type = %T, want object", result["settings"])
	}
	if settings["defaultNode"] != "127.0.0.1:10102" || settings["autoConnect"] != false {
		t.Fatalf("settings payload = %#v", settings)
	}
}
