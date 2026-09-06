package daemon

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetInfoParsesDERORPCResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request["method"] != "DERO.GetInfo" {
			t.Errorf("method = %v, want DERO.GetInfo", request["method"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"topoheight":   1234,
				"stableheight": 1200,
				"difficulty":   987654,
				"version":      "test-daemon",
				"network":      "Mainnet",
				"tx_pool_size": 7,
			},
		})
	}))
	defer server.Close()

	info := GetInfo(server.URL)
	if info == nil {
		t.Fatal("GetInfo returned nil")
	}
	if info.TopoHeight != 1234 || info.StableHeight != 1200 || info.Difficulty != 987654 {
		t.Fatalf("info heights/difficulty = %#v", info)
	}
	if info.Version != "test-daemon" || info.Network != "Mainnet" || info.MempoolSize != 7 {
		t.Fatalf("info metadata = %#v", info)
	}
}

func TestParseInfoDefaultsNetworkFromTestnetFlag(t *testing.T) {
	info := parseInfo(map[string]interface{}{
		"topoheight": 42.0,
		"testnet":    true,
	})
	if info.Network != "Testnet" {
		t.Fatalf("network = %q, want Testnet", info.Network)
	}
}

func TestFetchSCIDVariablesDecodesMetadataAndRatings(t *testing.T) {
	scid := strings.Repeat("a", 64)
	name := hex.EncodeToString([]byte("Example App"))
	durl := hex.EncodeToString([]byte("example.tela"))
	icon := hex.EncodeToString([]byte("https://example.test/icon.png"))
	ratingKey := "dero1" + strings.Repeat("d", 60)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
			Params struct {
				SCID string `json:"scid"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if request.Method != "DERO.GetSC" {
			t.Errorf("method = %q, want DERO.GetSC", request.Method)
		}
		if request.Params.SCID != scid {
			t.Errorf("SCID = %q, want %q", request.Params.SCID, scid)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"stringkeys": map[string]interface{}{
					"nameHdr":    name,
					"dURL":       durl,
					"iconURLHdr": icon,
					"likes":      "2",
					ratingKey:    "80_321",
				},
			},
		})
	}))
	defer server.Close()

	results := FetchSCIDVariables(server.URL, []string{scid})
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	got := results[0]
	if got.SCID != scid || got.NameHdr != "Example App" || got.DURL != "example.tela" || got.IconURL != "https://example.test/icon.png" {
		t.Fatalf("decoded metadata = %#v", got)
	}
	if got.Likes != 2 || got.Dislikes != 0 || got.Average != 80 || got.CreatedHeight != 321 {
		t.Fatalf("decoded ratings = %#v", got)
	}
}

func TestFetchSCIDVariablesSkipsRPCFailures(t *testing.T) {
	goodSCID := strings.Repeat("b", 64)
	badSCID := strings.Repeat("c", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Params struct {
				SCID string `json:"scid"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Params.SCID == badSCID {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{"code": -1, "message": "not found"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"stringkeys": map[string]interface{}{"nameHdr": "Good"},
			},
		})
	}))
	defer server.Close()

	results := FetchSCIDVariables(server.URL, []string{goodSCID, badSCID})
	if len(results) != 1 || results[0].SCID != goodSCID {
		t.Fatalf("results = %#v, want only successful SCID", results)
	}
}
