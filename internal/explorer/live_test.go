package explorer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Live test against a configured derod node. Run with HYPERWOLF_DERO_NODE,
// e.g. HYPERWOLF_DERO_NODE=http://192.168.1.154:10102 go test ./internal/explorer/ -run Live -v
func liveNode(t *testing.T) *Client {
	t.Helper()
	node := os.Getenv("HYPERWOLF_DERO_NODE")
	if node == "" {
		t.Skip("HYPERWOLF_DERO_NODE not set")
	}
	resp, err := http.Get(node + "/json_rpc")
	if err != nil {
		t.Skipf("node unreachable: %v", err)
	}
	resp.Body.Close()
	return New(node)
}

func dump(t *testing.T, label string, v any) {
	t.Helper()
	b, _ := json.MarshalIndent(v, "", "  ")
	t.Logf("%s:\n%s", label, b)
}

func TestLiveStats(t *testing.T) {
	c := liveNode(t)
	s, err := c.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if s.TopoHeight <= 0 {
		t.Fatalf("bad topoheight %d", s.TopoHeight)
	}
	dump(t, "stats", s)

	last, err := c.LastHeader()
	if err != nil {
		t.Fatalf("last header: %v", err)
	}
	dump(t, "last header", last)
}

func TestLiveBlockAndSearch(t *testing.T) {
	c := liveNode(t)
	s, err := c.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}

	hdr, err := c.HeaderByTopoHeight(uint64(s.TopoHeight) - 5)
	if err != nil {
		t.Fatalf("header: %v", err)
	}
	bl, err := c.Block(hdr.Hash)
	if err != nil {
		t.Fatalf("block by hash: %v", err)
	}
	dump(t, "block", bl)

	bl2, err := c.Block(fmt.Sprintf("%d", hdr.TopoHeight))
	if err != nil {
		t.Fatalf("block by topoheight: %v", err)
	}
	if bl2.Header.Hash != hdr.Hash {
		t.Fatalf("hash mismatch by topoheight: %s vs %s", bl2.Header.Hash, hdr.Hash)
	}

	res, err := c.Search(hdr.Hash)
	if err != nil || res.Type != "block" {
		t.Fatalf("search hash -> %+v err %v", res, err)
	}
	res, err = c.Search(fmt.Sprintf("%d", hdr.TopoHeight))
	if err != nil || res.Type != "block" {
		t.Fatalf("search height -> %+v err %v", res, err)
	}
	if bl2.Header.TXCount == 0 {
		t.Log("block has no txs on this node; skipping pool/tx checks")
		return
	}
}

func TestLivePoolAndTx(t *testing.T) {
	c := liveNode(t)
	hashes, err := c.Pool()
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	if len(hashes) == 0 {
		t.Log("empty pool on this node")
		return
	}
	txs, err := c.Txs(hashes)
	if err != nil {
		t.Fatalf("pool txs: %v", err)
	}
	dump(t, "pool", txs)
}

func TestLiveSC(t *testing.T) {
	c := liveNode(t)
	s, err := c.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	// walk back until we find a block with an SC tx to fetch real SCIDs
	for i := int64(0); i < 20; i++ {
		bl, err := c.Block(fmt.Sprintf("%d", s.TopoHeight-i))
		if err != nil {
			continue
		}
		scid := ""
		if bl.MinerTx != nil && strings.HasPrefix(bl.MinerTx.Type, "SC") && bl.MinerTx.Hash != "" {
			scid = bl.MinerTx.Hash
		}
		if scid == "" {
			for _, tx := range bl.Txs {
				if tx != nil && tx.SCID != "" {
					scid = tx.SCID
					break
				}
			}
		}
		if scid == "" {
			continue
		}
		sc, err := c.SC(scid)
		if err != nil {
			t.Fatalf("sc %s: %v", scid, err)
		}
		dump(t, "sc "+scid, sc)
		return
	}
	t.Skipf("no SC tx found in last 20 blocks (chain: %d)", s.TopoHeight)
}

func TestLiveSearchTypes(t *testing.T) {
	c := liveNode(t)
	for _, q := range []string{"0", "zzzz-not-a-real-query", "notarealname12345"} {
		res, err := c.Search(q)
		if err != nil {
			t.Fatalf("search %q: %v", q, err)
		}
		dump(t, "search "+q, res)
	}
}

var _ = time.Second
