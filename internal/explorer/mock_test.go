package explorer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockDaemon implements a tiny derod /json_rpc endpoint for unit tests. It
// emulates the pruned Stargate behaviour observed in the field: near-tip
// headers and blocks resolve, a block blob is served for the sample block,
// tx lookups return ring/signer/code but no hex, and GetSC returns state.
func mockDaemon(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		write := func(result any) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "result": result})
		}
		writeErr := func(code int, msg string) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": "1", "error": map[string]any{"code": code, "message": msg}})
		}

		switch req.Method {
		case "DERO.GetInfo":
			write(map[string]any{
				"difficulty": 5312788, "height": 7580807, "stableheight": 7580799,
				"topoheight": 7580807, "top_block_hash": "aaa", "tx_pool_size": 1,
				"total_supply": 16767569, "version": "3.6.0-TEST", "testnet": false,
				"outgoing_connections_count": 34,
			})
		case "DERO.GetLastBlockHeader":
			write(map[string]any{"block_header": blockHeader(7580807, "aaa")})
		case "DERO.GetBlockHeaderByTopoHeight":
			var p struct {
				TopoHeight uint64 `json:"topoheight"`
			}
			json.Unmarshal(req.Params, &p)
			if p.TopoHeight == 0 {
				writeErr(-32098, "empty block")
				return
			}
			write(map[string]any{"block_header": blockHeader(int64(p.TopoHeight), strings.Repeat("b", 64))})
		case "DERO.GetBlockHeaderByHash":
			write(map[string]any{"block_header": blockHeader(7590000, strings.Repeat("b", 64))})
		case "DERO.GetBlock":
			write(map[string]any{
				"blob":         sampleBlockBlob,
				"json":         `{"major_version":3,"height":7590000,"miner_tx":{"txtype":2,"miner_address":"","value":0,"Payloads":[]},"tx_hashes":[]}`,
				"block_header": blockHeader(7590000, strings.Repeat("b", 64)),
				"status":       "OK",
			})
		case "DERO.GetTxPool":
			write(map[string]any{"txs": []string{strings.Repeat("c", 64)}, "status": "OK"})
		case "DERO.GetTransaction":
			write(map[string]any{"txs": []map[string]any{
				{
					"as_hex": "", "block_height": int64(7590000), "in_pool": false,
					"tx_hash":     strings.Repeat("c", 64),
					"valid_block": strings.Repeat("b", 64),
					"ring":        [][]string{{"dero1qy...1", "dero1qy...2"}},
					"signer":      "dero1qysigner",
					"code":        "Function Initialize() Uint64\nEND FUNCTION",
				},
			}, "status": "OK"})
		case "DERO.GetSC":
			write(map[string]any{
				"balance": uint64(12345), "code": "Function Init() Uint64",
				"stringkeys": map[string]any{"owner": "abc"}, "uint64keys": map[uint64]any{7: uint64(42)},
				"status": "OK",
			})
		case "DERO.NameToAddress":
			var p struct {
				Name string `json:"name"`
			}
			json.Unmarshal(req.Params, &p)
			if p.Name == "derolibrary" {
				write(map[string]any{"name": p.Name, "address": "dero1qyfound", "status": "OK"})
				return
			}
			writeErr(-32098, "leaf not found")
		default:
			writeErr(-32601, "method not found")
		}
	}))
	return srv
}

func blockHeader(top int64, hash string) map[string]any {
	return map[string]any{
		"depth": 5, "height": top, "topoheight": top, "hash": hash,
		"major_version": 3, "minor_version": 3, "nonce": 0,
		"orphan_status": false, "syncblock": true, "sideblock": false,
		"txcount": 1, "reward": 10000000, "timestamp": 1788712130000,
		"miners": []string{"dero1qyminer"}, "tips": []string{strings.Repeat("f", 64)},
		"difficulty": "5312788",
	}
}

func TestMockStatsAndHeader(t *testing.T) {
	c := New(mockDaemon(t).URL)
	s, err := c.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if s.TopoHeight != 7580807 || s.Network != "Mainnet" {
		t.Fatalf("bad stats %+v", s)
	}
	h, err := c.LastHeader()
	if err != nil || h.Hash != "aaa" {
		t.Fatalf("last header %+v err %v", h, err)
	}
	h2, err := c.HeaderByTopoHeight(100)
	if err != nil || h2 == nil {
		t.Fatalf("header by topo: %+v err %v", h2, err)
	}
}

// sampleBlockBlob is a real serialized block (topoheight 7580810, one SC tx)
// captured from a live mainnet-adjacent node, used to exercise decoding.
const sampleBlockBlob = "0303000001a0778d166a8ad9ce03010000020b68272c215cda4f02ece3782c2e79fbb4a3952da5e39f1b0627cc979b93ab810000000000000000000000000000000000000000000000000000000000000000000173c4971c5de95c813b1618a2ba01ab8e15e52c0db491643f9db75b57e336970e0a41e315000073ac8a73c4971c0000000016aa433a33e7245d60276663d81c9a110000000032a9fbde1e7ce1a95e2cc8fa41e22c000073ac8a73c4971c000000006cda4319eee6d897bb296c444006f8050b000000bda4df6037f199018e68236b41f191000073ac8a73c4971c00000000261755e56e2c64949ce0932c9c2ac03d00000000dd4ba7c246e860fabe0bd36541ebd2000073ac8a73c4971c000000003a0ef31c89ac9f188c014c6e9ff57c5c000000001e0000009800000000099b0841fc88000073ac8a73c4971c00000000255f45d84d4ffd0e9ceac82363a4a41e0000000010000000600000000000920441fad5000073ac8a73c4971c0000000016aa433a33e7245d60276663d81c9a11000000003f4a5733628d02ed852f2ab041fefe000073ac8a73c4971c0000000010adc19e430b07149ea888b666d804a700000000e91f5475b171715c5bbb65184106c3000073ac8a73c4971c00000000244f5bf886eafda70a6be303e9e9e0f1000000001f5105ac8badd3a3a9210cc0410d70000073ac8a73c4971c000000005a9e4023cb284148ec01e580b3becdde00000000b40272bbc8bf1808d5a3af0b71166a000073ac8a73c4971c00000000bbe2506136a0dffbd27b8608d538b4c100000000eee4b3e2f66e9f0df8747f0901df2bf18b286ed5e16e99a2bb23919c0b5a6d41bac4d880e5fde3c1a8e8dc9eab"

func TestMockBlockDecode(t *testing.T) {
	c := New(mockDaemon(t).URL)
	bl, err := c.Block("7590000")
	if err != nil {
		t.Fatalf("block: %v", err)
	}
	if bl.Header.TopoHeight != 7590000 || bl.MinerTx == nil {
		t.Fatalf("bad block %+v", bl)
	}
	if bl.MinerTx.Type != "COINBASE" {
		t.Fatalf("miner tx type %q", bl.MinerTx.Type)
	}
	if bl.MinerTx.Amount == "" {
		t.Fatalf("miner amount empty")
	}
}

func TestMockTxPruned(t *testing.T) {
	c := New(mockDaemon(t).URL)
	tx, err := c.Tx(strings.Repeat("c", 64))
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	if tx.Type != "SC" || tx.SCID != strings.Repeat("c", 64) {
		t.Fatalf("pruned tx decode: %+v", tx)
	}
	if tx.RingSize != 2 || tx.Signer == "" || tx.InPool {
		t.Fatalf("ring/signer/in_pool wrong: %+v", tx)
	}
}

func TestMockPoolAndSC(t *testing.T) {
	c := New(mockDaemon(t).URL)
	pool, err := c.Pool()
	if err != nil || len(pool) != 1 {
		t.Fatalf("pool: %v %v", pool, err)
	}
	sc, err := c.SC(strings.Repeat("c", 64))
	if err != nil {
		t.Fatalf("sc: %v", err)
	}
	if sc.Balance != 12345 || sc.BalanceDero != "0.12345" {
		t.Fatalf("sc balance: %+v", sc)
	}
	if sc.StringKeys["owner"] != "abc" || sc.Uint64Keys["7"] != "42" {
		t.Fatalf("sc keys: %+v", sc)
	}
}

func TestMockSearch(t *testing.T) {
	c := New(mockDaemon(t).URL)
	res, err := c.Search("7590000")
	if err != nil || res.Type != "block" {
		t.Fatalf("search height: %+v err %v", res, err)
	}
	res, err = c.Search("derolibrary")
	if err != nil || res.Type != "address" || res.Address != "dero1qyfound" {
		t.Fatalf("search name: %+v err %v", res, err)
	}
	res, err = c.Search("zzz")
	if err != nil || res.Type != "none" {
		t.Fatalf("search junk: %+v err %v", res, err)
	}
	dummy := c.rpc("", nil, nil)
	if dummy == nil {
		t.Fatal("empty method should error")
	}
}
