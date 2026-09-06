package router

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"hyperwolf/internal/explorer"
)

func unixMilliString(ms uint64) string {
	return time.UnixMilli(int64(ms)).UTC().Format("2006-01-02 15:04:05")
}

func ageString(ms uint64) string {
	d := time.Since(time.UnixMilli(int64(ms)))
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds ago", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm ago", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd ago", int(d.Hours())/24)
}

// explorerClient returns (and lazily creates) an explorer client pinned to the
// currently configured daemon node. Clients are memoized per node so the TTL
// cache survives across requests but resets when the node changes.
func (h *Handlers) explorerClient() *explorer.Client {
	if h.expMu == nil {
		h.expMu = &sync.Mutex{}
		h.exps = map[string]*explorer.Client{}
	}
	node := h.State.GetNode()

	h.expMu.Lock()
	defer h.expMu.Unlock()
	if c, ok := h.exps[node]; ok {
		return c
	}
	c := explorer.New(node)
	h.exps[node] = c
	if len(h.exps) > 4 {
		for k := range h.exps {
			if k != node {
				delete(h.exps, k)
			}
		}
	}
	return c
}

func (h *Handlers) handleExplorerStats(w http.ResponseWriter, r *http.Request) {
	c := h.explorerClient()
	stats, err := c.Stats()
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: err.Error()})
		return
	}
	if last, err := c.LastHeader(); err == nil {
		stats.LastBlock = last
	}
	writeJSON(w, ctrlResp{OK: true, Result: stats})
}

func (h *Handlers) handleExplorerBlocks(w http.ResponseWriter, r *http.Request) {
	c := h.explorerClient()

	count := 15
	if v := r.URL.Query().Get("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			count = n
		}
	}

	stats, err := c.Stats()
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: err.Error()})
		return
	}

	from := uint64(stats.TopoHeight)
	if v := r.URL.Query().Get("from"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil && n >= 0 && n <= uint64(stats.TopoHeight) {
			from = n
		}
	}

	type hdrResult struct {
		idx int
		hdr *explorer.Header
		err error
	}
	results := make(chan hdrResult, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(top uint64) {
			defer wg.Done()
			if top == 0 {
				results <- hdrResult{idx: 0, hdr: nil, err: nil}
				return
			}
			hdr, err := c.HeaderByTopoHeight(top)
			results <- hdrResult{idx: int(top), hdr: hdr, err: err}
		}(from - uint64(i))
	}
	wg.Wait()
	close(results)

	headers := make([]*explorer.Header, 0, count)
	for res := range results {
		if res.err == nil && res.hdr != nil {
			headers = append(headers, res.hdr)
		}
	}
	for i := 0; i < len(headers); i++ {
		for j := i + 1; j < len(headers); j++ {
			if headers[j].TopoHeight > headers[i].TopoHeight {
				headers[i], headers[j] = headers[j], headers[i]
			}
		}
	}

	writeJSON(w, ctrlResp{OK: true, Result: map[string]any{
		"headers": headers,
		"from":    from,
		"count":   count,
		"top":     stats.TopoHeight,
	}})
}

func (h *Handlers) handleExplorerBlock(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeJSON(w, ctrlResp{OK: false, Error: "missing block id"})
		return
	}
	bl, err := h.explorerClient().Block(id)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: fmt.Sprintf("block %s not found: %s", id, err)})
		return
	}
	writeJSON(w, ctrlResp{OK: true, Result: bl})
}

func (h *Handlers) handleExplorerTx(w http.ResponseWriter, r *http.Request) {
	hash := strings.TrimSpace(r.PathValue("hash"))
	if len(hash) != 64 {
		writeJSON(w, ctrlResp{OK: false, Error: "invalid transaction hash"})
		return
	}
	c := h.explorerClient()
	tx, err := c.Tx(hash)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: fmt.Sprintf("tx %s not found: %s", hash, err)})
		return
	}
	if !tx.InPool && tx.ValidBlock != "" && tx.BlockTime == "" {
		if hdr, herr := c.HeaderByHash(tx.ValidBlock); herr == nil && hdr != nil {
			tx.BlockTime = unixMilliString(hdr.Timestamp)
			tx.Age = ageString(hdr.Timestamp)
			tx.Depth = hdr.Depth
			if tx.Height == 0 {
				tx.Height = hdr.Height
			}
		}
	}
	writeJSON(w, ctrlResp{OK: true, Result: tx})
}

func (h *Handlers) handleExplorerMempool(w http.ResponseWriter, r *http.Request) {
	c := h.explorerClient()
	hashes, err := c.Pool()
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: err.Error()})
		return
	}
	limit := 100
	if v := r.URL.Query().Get("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	if len(hashes) > limit {
		hashes = hashes[:limit]
	}
	txs, err := c.Txs(hashes)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, ctrlResp{OK: true, Result: map[string]any{
		"pool_size": len(hashes),
		"txs":       txs,
	}})
}

func (h *Handlers) handleExplorerSC(w http.ResponseWriter, r *http.Request) {
	scid := strings.ToLower(strings.TrimSpace(r.PathValue("scid")))
	if len(scid) != 64 {
		writeJSON(w, ctrlResp{OK: false, Error: "invalid scid"})
		return
	}
	sc, err := h.explorerClient().SC(scid)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: fmt.Sprintf("sc %s not found: %s", scid, err)})
		return
	}
	writeJSON(w, ctrlResp{OK: true, Result: sc})
}

func (h *Handlers) handleExplorerAddress(w http.ResponseWriter, r *http.Request) {
	addr := strings.TrimSpace(r.PathValue("addr"))
	if len(addr) != 66 || !strings.HasPrefix(addr, "dero1") {
		writeJSON(w, ctrlResp{OK: false, Error: "invalid dero address"})
		return
	}
	info, err := h.explorerClient().Address(addr)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: fmt.Sprintf("address lookup failed: %s", err)})
		return
	}
	if name := strings.TrimSpace(r.URL.Query().Get("name")); name != "" {
		info.Name = name
	}
	writeJSON(w, ctrlResp{OK: true, Result: info})
}

func (h *Handlers) handleExplorerSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, ctrlResp{OK: false, Error: "missing q"})
		return
	}
	res, err := h.explorerClient().Search(q)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, ctrlResp{OK: true, Result: res})
}
