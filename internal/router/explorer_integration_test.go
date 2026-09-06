// End-to-end test of the explorer API surface against a fake derod node,
// exercising router handlers -> explorer client -> JSON-RPC over HTTP.
package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"hyperwolf/internal/state"
)

// sampleBlockBlob mirrors internal/explorer's real serialized block fixture
// (one SC tx). Re-declared here to keep the router test self-contained.
const sampleBlockBlob = "0303000001a0778d166a8ad9ce03010000020b68272c215cda4f02ece3782c2e79fbb4a3952da5e39f1b0627cc979b93ab810000000000000000000000000000000000000000000000000000000000000000000173c4971c5de95c813b1618a2ba01ab8e15e52c0db491643f9db75b57e336970e0a41e315000073ac8a73c4971c0000000016aa433a33e7245d60276663d81c9a110000000032a9fbde1e7ce1a95e2cc8fa41e22c000073ac8a73c4971c000000006cda4319eee6d897bb296c444006f8050b000000bda4df6037f199018e68236b41f191000073ac8a73c4971c00000000261755e56e2c64949ce0932c9c2ac03d00000000dd4ba7c246e860fabe0bd36541ebd2000073ac8a73c4971c000000003a0ef31c89ac9f188c014c6e9ff57c5c000000001e0000009800000000099b0841fc88000073ac8a73c4971c00000000255f45d84d4ffd0e9ceac82363a4a41e0000000010000000600000000000920441fad5000073ac8a73c4971c0000000016aa433a33e7245d60276663d81c9a11000000003f4a5733628d02ed852f2ab041fefe000073ac8a73c4971c0000000010adc19e430b07149ea888b666d804a700000000e91f5475b171715c5bbb65184106c3000073ac8a73c4971c00000000244f5bf886eafda70a6be303e9e9e0f1000000001f5105ac8badd3a3a9210cc0410d70000073ac8a73c4971c000000005a9e4023cb284148ec01e580b3becdde00000000b40272bbc8bf1808d5a3af0b71166a000073ac8a73c4971c00000000bbe2506136a0dffbd27b8608d538b4c100000000eee4b3e2f66e9f0df8747f0901df2bf18b286ed5e16e99a2bb23919c0b5a6d41bac4d880e5fde3c1a8e8dc9eab"

const sampleBlockHash = "5d8b8142eff06aa6e19a2e50c8a2e7f6c5fdf4e0e9a165c0e8d83bfe7a915e3a"

func fakeDerod(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/json_rpc", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		rpcErr := func(code int, msg string) {
			json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      "1",
				"error":   map[string]any{"code": code, "message": msg},
			})
		}

		header := map[string]any{
			"depth":         0,
			"difficulty":    "1000000",
			"hash":          sampleBlockHash,
			"height":        100,
			"topoheight":    101,
			"major_version": 3,
			"minor_version": 0,
			"nonce":         7,
			"orphan_status": false,
			"syncblock":     false,
			"sideblock":     false,
			"txcount":       1,
			"miners":        []string{},
			"reward":        250000,
			"timestamp":     1700000000000,
		}

		switch req.Method {
		case "DERO.GetInfo":
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "result": map[string]any{
				"difficulty":                 1000000,
				"height":                     791,
				"stableheight":               790,
				"topoheight":                 101,
				"averageblocktime50":         20,
				"testnet":                    false,
				"network":                    "Mainnet",
				"top_block_hash":             sampleBlockHash,
				"tx_count":                   1,
				"tx_pool_size":               0,
				"dynamic_fee_per_kb":         100,
				"total_supply":               uint64(100000000000),
				"median_block_size":          1024,
				"version":                    "3.6.0",
				"white_peerlist_size":        3,
				"incoming_connections_count": 1,
				"outgoing_connections_count": 2,
			}})
		case "DERO.GetLastBlockHeader", "DERO.GetBlockHeaderByTopoHeight":
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "result": map[string]any{"block_header": header, "status": "OK"}})
		case "DERO.GetBlockHeaderByHash":
			// Mirror real derod behaviour: unknown hashes answer with a zeroed
			// header and a blank status rather than an RPC error.
			var params struct {
				Hash string `json:"hash"`
			}
			_ = json.Unmarshal(req.Params, &params)
			if params.Hash == sampleBlockHash {
				json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "result": map[string]any{"block_header": header, "status": "OK"}})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "result": map[string]any{
				"block_header": map[string]any{"depth": 0, "difficulty": "", "hash": "", "height": 0, "topoheight": 0, "major_version": 0, "minor_version": 0, "nonce": 0, "orphan_status": false, "syncblock": false, "sideblock": false, "txcount": 0, "miners": nil, "reward": 0, "tips": nil, "timestamp": 0},
				"status":       "",
			}})
		case "DERO.GetBlock":
			var params struct {
				Hash   string `json:"hash"`
				Height uint64 `json:"height"`
			}
			_ = json.Unmarshal(req.Params, &params)
			if params.Hash != "" && params.Hash != sampleBlockHash {
				rpcErr(-32000, "block not found")
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "result": map[string]any{"blob": sampleBlockBlob, "json": "", "block_header": header, "status": "OK"}})
		case "DERO.GetTransaction":
			// Mirror real derod: a known tx hash returns identifying data, while
			// any unknown/pruned hash answers with an all-empty tx entry and
			// status OK instead of an RPC error.
			var params struct {
				Hashes []string `json:"txs_hashes"`
			}
			_ = json.Unmarshal(req.Params, &params)
			if len(params.Hashes) == 1 && params.Hashes[0] == scIDFixture {
				json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "result": map[string]any{
					"txs": []any{
						map[string]any{"tx_hash": "", "valid_block": strings.Repeat("c", 64), "block_height": 7572335, "as_hex": "0a00f", "in_pool": false, "status": "OK"},
					},
					"status": "OK",
				}})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "result": map[string]any{
				"txs": []any{
					map[string]any{"as_hex": "", "block_height": 0, "in_pool": false, "output_indices": nil, "tx_hash": "", "valid_block": "", "invalid_block": nil, "ring": nil, "signer": "", "balance": 0, "code": "", "balancenow": 0, "codenow": "", "status": "OK"},
				},
				"status": "OK",
			}})
		case "DERO.GetTxPool":
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "result": map[string]any{"txs": []string{}, "status": "OK"}})
		case "DERO.GetSC":
			// Unknown SCIDs answer with status OK and an all-empty result.
			var params struct {
				SCID string `json:"scid"`
			}
			_ = json.Unmarshal(req.Params, &params)
			if params.SCID != scIDFixture {
				json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "result": map[string]any{
					"stringkeys": map[string]any{}, "uint64keys": map[uint64]any{},
					"balances": map[string]uint64{}, "balance": 0, "code": "", "status": "OK",
				}})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "result": map[string]any{
				"stringkeys": map[string]any{"name": "scname"},
				"uint64keys": map[uint64]any{},
				"balances":   map[string]uint64{},
				"balance":    5000,
				"code":       "function main() {}",
				"status":     "OK",
			}})
		case "DERO.NameToAddress":
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "result": map[string]any{
				"name": "derolibrary", "address": addrFixture(), "status": "OK",
			}})
		default:
			rpcErr(-32601, req.Method+" not stubbed")
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func addrFixture() string {
	return "dero1qyfound1500xct8v6t" + strings.Repeat("c", 43) // 66 chars
}

// scIDFixture is a "live" contract in the fake derod: GetSC returns code, and
// the same hash also resolves as its install tx.
const scIDFixture = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func newExplorerStore(node string) *Handlers {
	st := state.New()
	st.SetNode(node)
	return &Handlers{State: st, Hub: NewHub()}
}

func getJSON(t *testing.T, srv *http.Server, path string) ctrlResp {
	t.Helper()
	resp := performRequest(srv, http.MethodGet, path, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d", path, resp.Code)
	}
	var out ctrlResp
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET %s: decode %v", path, err)
	}
	return out
}

func expectOK(t *testing.T, srv *http.Server, path string) map[string]any {
	t.Helper()
	out := getJSON(t, srv, path)
	if !out.OK {
		t.Fatalf("GET %s failed: %#v (%s)", path, out.Result, out.Error)
	}
	m, ok := out.Result.(map[string]any)
	if !ok {
		t.Fatalf("GET %s result type %T", path, out.Result)
	}
	return m
}

func stubWebFS() *fstest.MapFS {
	return &fstest.MapFS{
		"web/index.html": &fstest.MapFile{Data: []byte("<html>test</html>")},
	}
}

func TestExplorerEndpointsAgainstFakeDerod(t *testing.T) {
	node := fakeDerod(t)
	h := newExplorerStore(node.URL)
	srv := NewServer("127.0.0.1:18080", stubWebFS(), h)

	stats := expectOK(t, srv, "/api/explorer/stats")
	if stats["network"] != "Mainnet" {
		t.Fatalf("stats network = %v", stats["network"])
	}
	if stats["topoheight"] != float64(101) {
		t.Fatalf("stats topoheight = %v", stats["topoheight"])
	}

	blocks := expectOK(t, srv, "/api/explorer/blocks?count=5")
	if blocks["top"] != float64(101) {
		t.Fatalf("blocks top = %v", blocks["top"])
	}
	headers, ok := blocks["headers"].([]any)
	if !ok || len(headers) == 0 {
		t.Fatalf("blocks headers = %#v", blocks["headers"])
	}
	if headers[0].(map[string]any)["hash"] != sampleBlockHash {
		t.Fatalf("newest block should come first, got %v", headers[0].(map[string]any)["hash"])
	}

	blk := expectOK(t, srv, "/api/explorer/block/"+sampleBlockHash)
	blkHeader, ok := blk["header"].(map[string]any)
	if !ok || blkHeader["hash"] != sampleBlockHash {
		t.Fatalf("block header = %#v", blk["header"])
	}
	if blkHeader["reward_dero"] != "2.50000" {
		t.Fatalf("block header reward_dero = %v", blkHeader["reward_dero"])
	}
	if blk["tx_count"] != float64(2) { // miner tx + 1 embedded SC tx from the fixture blob
		t.Fatalf("block tx_count = %v", blk["tx_count"])
	}
	if blk["size"] == "" || blk["size_bytes"] == float64(0) {
		t.Fatalf("block size missing: %#v", blk)
	}

	pool := expectOK(t, srv, "/api/explorer/mempool")
	if pool["pool_size"] != float64(0) {
		t.Fatalf("mempool size = %v", pool["pool_size"])
	}

	sc := expectOK(t, srv, "/api/explorer/sc/"+scIDFixture)
	if sc["balance_dero"] != "0.05000" {
		t.Fatalf("sc balance_dero = %v", sc["balance_dero"])
	}
	if sc["code"] != "function main() {}" {
		t.Fatalf("sc code = %v", sc["code"])
	}

	addr := expectOK(t, srv, "/api/explorer/address/"+addrFixture())
	if addr["address"] != addrFixture() {
		t.Fatalf("address info = %#v", addr)
	}
	addrName := expectOK(t, srv, "/api/explorer/address/"+addrFixture()+"?name=derotary")
	if addrName["name"] != "derotary" {
		t.Fatalf("address name = %#v", addrName)
	}

	res := expectOK(t, srv, "/api/explorer/search?q=101")
	if res["type"] != "block" {
		t.Fatalf("search height -> %#v", res)
	}
	res = expectOK(t, srv, "/api/explorer/search?q=derolibrary")
	if res["type"] != "address" {
		t.Fatalf("search name -> %#v", res)
	}
	addrStr, _ := res["address"].(string)
	if addrStr == "" || !strings.HasPrefix(addrStr, "dero1") {
		t.Fatalf("search name address = %#v", res)
	}
	if res["name"] != "derolibrary" {
		t.Fatalf("search name should carry the resolved name: %#v", res)
	}

	scID := scIDFixture
	res = expectOK(t, srv, "/api/explorer/search?q="+scID)
	if res["type"] != "sc" {
		t.Fatalf("search scid -> %#v", res)
	}
	sc = expectOK(t, srv, "/api/explorer/sc/"+scID)
	if sc["code"] != "function main() {}" {
		t.Fatalf("sc info = %#v", sc)
	}

	// Regression: an unknown 64-hex must NOT classify as a tx just because derod
	// answered GetTransaction with an all-empty entry + status OK, nor as an SC
	// because GetSC answered with an all-empty result + status OK.
	unknown := strings.Repeat("b", 64)
	out := getJSON(t, srv, "/api/explorer/search?q="+unknown)
	if out.OK && out.Result.(map[string]any)["type"] != "none" {
		t.Fatalf("search unknown hex -> %#v (want none)", out.Result)
	}
	if res["type"] != "sc" {
		t.Fatalf("search scid after unknown -> %#v", res)
	}
}

func TestExplorerEndpointsErrors(t *testing.T) {
	node := fakeDerod(t)
	h := newExplorerStore(node.URL)
	srv := NewServer("127.0.0.1:18080", stubWebFS(), h)

	out := getJSON(t, srv, "/api/explorer/block/"+strings.Repeat("f", 64))
	if out.OK || !strings.Contains(out.Error, "not found") {
		t.Fatalf("unknown block hash response = %#v", out)
	}

	out = getJSON(t, srv, "/api/explorer/tx/abc")
	if out.OK || out.Error == "" {
		t.Fatalf("invalid tx hash response = %#v", out)
	}

	out = getJSON(t, srv, "/api/explorer/sc/short")
	if out.OK || out.Error == "" {
		t.Fatalf("invalid scid response = %#v", out)
	}

	// A 64-hex SCID reference that no contract owns must report not-found even
	// though derod answered GetSC with status OK and an empty result.
	out = getJSON(t, srv, "/api/explorer/sc/"+strings.Repeat("b", 64))
	if out.OK || !strings.Contains(out.Error, "not found") {
		t.Fatalf("unknown scid response = %#v", out)
	}
}
