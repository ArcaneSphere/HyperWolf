package indexer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hypergnomon/hypergnomon/structures"
)

func TestFastSyncEmptyRegistryAgainstFakeDaemon(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return
			}

			var result interface{}
			switch req.Method {
			case "DERO.GetInfo":
				result = map[string]interface{}{"topoheight": 100}
			case "DERO.GetSC":
				result = map[string]interface{}{
					// The daemon rejects a completely empty variable map; this
					// non-registry key still represents an empty usable catalog.
					"stringkeys": map[string]interface{}{"metadata": "test"},
					"uint64keys": map[string]interface{}{},
				}
			default:
				_ = conn.WriteJSON(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"error":   map[string]interface{}{"code": -32601, "message": "method not found"},
				})
				continue
			}
			if err := conn.WriteJSON(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  result,
			}); err != nil {
				return
			}
		}
	}))
	server.Start()
	defer server.Close()

	idx, err := New(Config{
		Endpoint:         server.Listener.Addr().String(),
		DBDir:            t.TempDir(),
		PoolSize:         1,
		ParallelBlocks:   1,
		BatchSize:        1,
		TurboMode:        true,
		CodePolicy:       "none",
		FinalityDepth:    1,
		PostScanVarsMode: "lazy",
	})
	if err != nil {
		t.Fatalf("New indexer: %v", err)
	}
	defer idx.Close()

	if err := idx.FastSync(false); err != nil {
		t.Fatalf("FastSync: %v", err)
	}
	if got := idx.ChainHeight.Load(); got != 100 {
		t.Fatalf("chain height = %d, want 100", got)
	}
	if got, err := idx.Store.GetLastIndexHeight(); err != nil || got != 100 {
		t.Fatalf("stored last height = %d, err=%v, want 100", got, err)
	}
	if !structures.TELAProbeSettled.Load() {
		t.Fatal("empty FastSync did not settle TELA discovery")
	}
}

func TestFastSyncClassifiesTELARegistryCandidate(t *testing.T) {
	candidate := strings.Repeat("b", 64)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params struct {
					SCID string `json:"scid"`
				} `json:"params"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return
			}

			var result interface{}
			switch req.Method {
			case "DERO.GetInfo":
				result = map[string]interface{}{"topoheight": 100}
			case "DERO.GetSC":
				if req.Params.SCID == structures.GnomonSCID_Mainnet {
					result = map[string]interface{}{
						"stringkeys": map[string]interface{}{
							candidate:            "owner",
							candidate + "owner":  "owner-address",
							candidate + "height": 42,
						},
					}
				} else {
					result = map[string]interface{}{
						"code": "Function Initialize() TELA-INDEX-1 END",
						"stringkeys": map[string]interface{}{
							"var_header_name": "Fixture App",
							"dURL":            "fixture.tela",
						},
					}
				}
			default:
				_ = conn.WriteJSON(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"error":   map[string]interface{}{"code": -32601, "message": "method not found"},
				})
				continue
			}
			if err := conn.WriteJSON(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  result,
			}); err != nil {
				return
			}
		}
	}))
	server.Start()
	defer server.Close()

	structures.TELAProbeSettled.Store(false)
	idx, err := New(Config{
		Endpoint:         server.Listener.Addr().String(),
		DBDir:            t.TempDir(),
		PoolSize:         2,
		ParallelBlocks:   1,
		BatchSize:        1,
		TurboMode:        false,
		CodePolicy:       "none",
		FinalityDepth:    1,
		PostScanVarsMode: "lazy",
	})
	if err != nil {
		t.Fatalf("New indexer: %v", err)
	}
	defer idx.Close()

	if err := idx.FastSync(false); err != nil {
		t.Fatalf("FastSync: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for !structures.TELAProbeSettled.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !structures.TELAProbeSettled.Load() {
		t.Fatal("TELA probe did not settle")
	}

	installs, err := idx.Store.GetClassInstalls("TELA-INDEX-1", 0)
	if err != nil {
		t.Fatalf("GetClassInstalls: %v", err)
	}
	if len(installs) != 1 || installs[0].SCID != candidate {
		t.Fatalf("TELA installs = %#v, want candidate %s", installs, candidate)
	}
}
