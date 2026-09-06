// Package explorer provides a thin, cached JSON-RPC client against a DERO
// daemon (derod) for the in-app blockchain explorer. All methods hit the
// daemon's /json_rpc endpoint; confirmed data is cached briefly and pool
// data is always fetched live.
package explorer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// FormatMoney renders an atomic DERO amount using the canonical 5-decimal
// convention (1 DERO = 100000 atomic units), e.g. "1.23456".
func FormatMoney(amount uint64) string {
	return fmt.Sprintf("%.5f", float64(amount)/100000)
}

func formatDero(amount uint64) string {
	return FormatMoney(amount)
}

const jrpcTimeout = 15 * time.Second

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("rpc error %d", e.Code)
}

// ErrNodeUnset is returned when no daemon node has been configured.
var ErrNodeUnset = errors.New("node not set")

// Client talks to a single derod node with a small TTL cache.
type Client struct {
	Node  string
	hc    *http.Client
	cache *ttlCache
}

// New returns an Explorer client for the given daemon node URL.
func New(node string) *Client {
	return &Client{
		Node:  node,
		hc:    &http.Client{Timeout: jrpcTimeout},
		cache: newTTLCache(4096),
	}
}

// rpc performs a JSON-RPC 2.0 call and unmarshals result into out. Methods
// that take no parameters must pass nil for params; the daemon rejects a
// `"params":{}` key on those calls.
func (c *Client) rpc(method string, params, out any) error {
	if c.Node == "" {
		return ErrNodeUnset
	}
	reqObj := map[string]any{"jsonrpc": "2.0", "id": "1", "method": method}
	if params != nil {
		reqObj["params"] = params
	}
	req, err := json.Marshal(reqObj)
	if err != nil {
		return err
	}

	var wrapper struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}

	resp, err := c.hc.Post(c.Node+"/json_rpc", "application/json", bytes.NewReader(req))
	if err != nil {
		return fmt.Errorf("daemon unreachable: %w", err)
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&wrapper); err != nil {
		return fmt.Errorf("decode rpc response: %w", err)
	}
	if wrapper.Error != nil {
		return wrapper.Error
	}
	if len(wrapper.Result) == 0 {
		return fmt.Errorf("%s returned no result", method)
	}
	if out != nil {
		if err := json.Unmarshal(wrapper.Result, out); err != nil {
			return fmt.Errorf("unmarshal %s result: %w", method, err)
		}
	}
	return nil
}

// cacheable wraps a fetch function in a short-lived cache keyed by `key`.
// All explorer fetches use this singleton cache on the client.
func (c *Client) cacheable(key string, ttl time.Duration, fn func() (any, error)) (any, error) {
	return c.cache.getOrSet(c.Node+"|"+key, ttl, fn)
}

// ---- raw derod results ----------------------------------------------------

type rawInfo struct {
	Difficulty          uint64  `json:"difficulty"`
	Height              int64   `json:"height"`
	StableHeight        int64   `json:"stableheight"`
	TopoHeight          int64   `json:"topoheight"`
	AverageBlockTime50  float32 `json:"averageblocktime50"`
	Testnet             bool    `json:"testnet"`
	Network             string  `json:"network"`
	TopBlockHash        string  `json:"top_block_hash"`
	TxCount             uint64  `json:"tx_count"`
	TxPoolSize          uint64  `json:"tx_pool_size"`
	DynamicFeePerKB     uint64  `json:"dynamic_fee_per_kb"`
	TotalSupply         uint64  `json:"total_supply"`
	MedianBlockSize     uint64  `json:"median_block_size"`
	Version             string  `json:"version"`
	WhitePeerListSize   uint64  `json:"white_peerlist_size"`
	IncomingConnections uint64  `json:"incoming_connections_count"`
	OutgoingConnections uint64  `json:"outgoing_connections_count"`
}

type rawHeader struct {
	Depth        int64    `json:"depth"`
	Difficulty   string   `json:"difficulty"`
	Hash         string   `json:"hash"`
	Height       int64    `json:"height"`
	TopoHeight   int64    `json:"topoheight"`
	MajorVersion uint64   `json:"major_version"`
	MinorVersion uint64   `json:"minor_version"`
	Nonce        uint64   `json:"nonce"`
	OrphanStatus bool     `json:"orphan_status"`
	SyncBlock    bool     `json:"syncblock"`
	SideBlock    bool     `json:"sideblock"`
	TXCount      int64    `json:"txcount"`
	Miners       []string `json:"miners"`
	Reward       uint64   `json:"reward"`
	Tips         []string `json:"tips"`
	Timestamp    uint64   `json:"timestamp"`
	Status       string   `json:"status"`
}

// headerFound reports whether the node returned a real block header. Several
// derod builds answer lookup misses (unknown hash, out-of-range topoheight)
// with a zeroed header and a blank status instead of an RPC error.
func headerFound(h rawHeader) bool {
	if h.Hash == "" {
		return false
	}
	if s := strings.TrimSpace(strings.ToLower(h.Status)); s != "" && s != "ok" {
		return false
	}
	return true
}

type rawBlockResult struct {
	Blob        string    `json:"blob"`
	Json        string    `json:"json"`
	BlockHeader rawHeader `json:"block_header"`
	Status      string    `json:"status"`
}

type rawTxInfo struct {
	AsHex         string     `json:"as_hex"`
	BlockHeight   int64      `json:"block_height"`
	Reward        uint64     `json:"reward"`
	InPool        bool       `json:"in_pool"`
	OutputIndices []uint64   `json:"output_indices"`
	TxHash        string     `json:"tx_hash"`
	ValidBlock    string     `json:"valid_block"`
	InvalidBlock  []string   `json:"invalid_block"`
	Ring          [][]string `json:"ring"`
	Signer        string     `json:"signer"`
	Balance       uint64     `json:"balance"`
	Code          string     `json:"code"`
	BalanceNow    uint64     `json:"balancenow"`
	CodeNow       string     `json:"codenow"`
}

type rawTxResult struct {
	Txs    []rawTxInfo `json:"txs"`
	Status string      `json:"status"`
}

type rawPoolResult struct {
	TxList []string `json:"txs"`
	Status string   `json:"status"`
}

type rawSCResult struct {
	VariableStringKeys map[string]any    `json:"stringkeys"`
	VariableUint64Keys map[uint64]any    `json:"uint64keys"`
	Balances           map[string]uint64 `json:"balances"`
	Balance            uint64            `json:"balance"`
	Code               string            `json:"code"`
	Status             string            `json:"status"`
}

type rawNameResult struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Status  string `json:"status"`
}

// ---- public presentation types -------------------------------------------

// Stats summarizes the current network state from DERO.GetInfo.
type Stats struct {
	Network             string  `json:"network"`
	Version             string  `json:"version"`
	Testnet             bool    `json:"testnet"`
	Height              int64   `json:"height"`
	StableHeight        int64   `json:"stableheight"`
	TopoHeight          int64   `json:"topoheight"`
	Difficulty          uint64  `json:"difficulty"`
	TxCount             uint64  `json:"tx_count"`
	MempoolSize         uint64  `json:"mempool_size"`
	TotalSupply         uint64  `json:"total_supply"`
	TotalSupplyDero     string  `json:"total_supply_dero"`
	DynamicFeePerKB     uint64  `json:"dynamic_fee_per_kb"`
	DynamicFeePerKBDero string  `json:"dynamic_fee_per_kb_dero"`
	MedianBlockSize     uint64  `json:"median_block_size"`
	AverageBlockTime50  float32 `json:"average_block_time_50"`
	TopBlockHash        string  `json:"top_block_hash"`
	Peers               uint64  `json:"peers"`
	Connections         uint64  `json:"connections"`
	LastBlock           *Header `json:"last_block,omitempty"`
}

// Header is a compact block header, the core of most explorer views.
type Header struct {
	Depth        int64    `json:"depth"`
	Height       int64    `json:"height"`
	TopoHeight   int64    `json:"topoheight"`
	Hash         string   `json:"hash"`
	MajorVersion uint64   `json:"major_version"`
	MinorVersion uint64   `json:"minor_version"`
	Nonce        uint64   `json:"nonce"`
	Orphan       bool     `json:"orphan"`
	SyncBlock    bool     `json:"sync_block"`
	SideBlock    bool     `json:"side_block"`
	TXCount      int64    `json:"txcount"`
	Miners       []string `json:"miners"`
	Reward       uint64   `json:"reward"`
	RewardDero   string   `json:"reward_dero"`
	Tips         []string `json:"tips"`
	Timestamp    uint64   `json:"timestamp"`
	Difficulty   string   `json:"difficulty"`
}

func (h *rawHeader) public() *Header {
	return &Header{
		Depth:        h.Depth,
		Height:       h.Height,
		TopoHeight:   h.TopoHeight,
		Hash:         h.Hash,
		MajorVersion: h.MajorVersion,
		MinorVersion: h.MinorVersion,
		Nonce:        h.Nonce,
		Orphan:       h.OrphanStatus,
		SyncBlock:    h.SyncBlock,
		SideBlock:    h.SideBlock,
		TXCount:      h.TXCount,
		Miners:       h.Miners,
		Reward:       h.Reward,
		RewardDero:   FormatMoney(h.Reward),
		Tips:         h.Tips,
		Timestamp:    h.Timestamp,
		Difficulty:   h.Difficulty,
	}
}

// Stats returns a snapshot of network information.
func (c *Client) Stats() (*Stats, error) {
	v, err := c.cacheable("stats", 3*time.Second, func() (any, error) {
		var info rawInfo
		if err := c.rpc("DERO.GetInfo", nil, &info); err != nil {
			return nil, err
		}
		if info.Network == "" {
			if info.Testnet {
				info.Network = "Testnet"
			} else {
				info.Network = "Mainnet"
			}
		}
		return &Stats{
			Network:             info.Network,
			Version:             info.Version,
			Testnet:             info.Testnet,
			Height:              info.Height,
			StableHeight:        info.StableHeight,
			TopoHeight:          info.TopoHeight,
			Difficulty:          info.Difficulty,
			TxCount:             info.TxCount,
			MempoolSize:         info.TxPoolSize,
			TotalSupply:         info.TotalSupply,
			TotalSupplyDero:     FormatMoney(info.TotalSupply),
			DynamicFeePerKB:     info.DynamicFeePerKB,
			DynamicFeePerKBDero: formatDero(info.DynamicFeePerKB),
			MedianBlockSize:     info.MedianBlockSize,
			AverageBlockTime50:  info.AverageBlockTime50,
			TopBlockHash:        info.TopBlockHash,
			Peers:               info.WhitePeerListSize,
			Connections:         info.IncomingConnections + info.OutgoingConnections,
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*Stats), nil
}

// LastHeader returns the block header at the current tip.
func (c *Client) LastHeader() (*Header, error) {
	v, err := c.cacheable("last_header", 2*time.Second, func() (any, error) {
		var out struct {
			BlockHeader rawHeader `json:"block_header"`
		}
		if err := c.rpc("DERO.GetLastBlockHeader", nil, &out); err != nil {
			return nil, err
		}
		if !headerFound(out.BlockHeader) {
			return nil, fmt.Errorf("no block header")
		}
		return out.BlockHeader.public(), nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*Header), nil
}

// HeaderByTopoHeight fetches (and briefly caches) a block header by topoheight.
func (c *Client) HeaderByTopoHeight(topo uint64) (*Header, error) {
	key := fmt.Sprintf("header:%d", topo)
	v, err := c.cacheable(key, 3*time.Second, func() (any, error) {
		var out struct {
			BlockHeader rawHeader `json:"block_header"`
		}
		if err := c.rpc("DERO.GetBlockHeaderByTopoHeight", map[string]uint64{"topoheight": topo}, &out); err != nil {
			return nil, err
		}
		if !headerFound(out.BlockHeader) {
			return nil, fmt.Errorf("no block at topoheight %d", topo)
		}
		return out.BlockHeader.public(), nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*Header), nil
}

// HeaderByHash fetches a block header by block hash.
func (c *Client) HeaderByHash(hash string) (*Header, error) {
	v, err := c.cacheable("hdrhash:"+hash, 10*time.Second, func() (any, error) {
		var out struct {
			BlockHeader rawHeader `json:"block_header"`
		}
		if err := c.rpc("DERO.GetBlockHeaderByHash", map[string]string{"hash": hash}, &out); err != nil {
			return nil, err
		}
		if !headerFound(out.BlockHeader) {
			return nil, fmt.Errorf("block not found")
		}
		return out.BlockHeader.public(), nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*Header), nil
}

// Block fetches a full block by hash or by topoheight (height param indexes
// the topological order, exactly like the daemon's DERO.GetBlock).
func (c *Client) Block(id string) (*Block, error) {
	isHeight := len(id) != 64
	key := "block:" + id
	ttl := 5 * time.Second
	v, err := c.cacheable(key, ttl, func() (any, error) {
		var raw rawBlockResult
		var params map[string]any
		if isHeight {
			params = map[string]any{"height": mustUint64(id)}
		} else {
			params = map[string]any{"hash": id}
		}
		if err := c.rpc("DERO.GetBlock", params, &raw); err != nil {
			return nil, err
		}
		if !headerFound(raw.BlockHeader) {
			return nil, fmt.Errorf("block %s not found", id)
		}
		return c.buildBlock(&raw)
	})
	if err != nil {
		return nil, err
	}
	return v.(*Block), nil
}

func mustUint64(s string) uint64 {
	var n uint64
	fmt.Sscanf(s, "%d", &n)
	return n
}

// Tx fetches a single transaction by hash.
func (c *Client) Tx(hash string) (*Tx, error) {
	key := "tx:" + hash
	c.cache.mu.Lock()
	if e, ok := c.cache.items[key]; ok && time.Now().Before(e.exp) {
		c.cache.mu.Unlock()
		return e.val.(*Tx), nil
	}
	c.cache.mu.Unlock()

	txs, err := c.rawTxs([]string{hash})
	if err != nil {
		return nil, err
	}
	var tx *Tx
	for i := range txs {
		if txs[i] == nil {
			continue
		}
		tx = txs[i]
		break
	}
	if tx == nil {
		return nil, fmt.Errorf("transaction not found")
	}

	ttl := 3 * time.Second
	if !tx.InPool {
		ttl = 30 * time.Second
	}

	c.cache.mu.Lock()
	now := time.Now()
	for k, e := range c.cache.items {
		if now.After(e.exp) {
			delete(c.cache.items, k)
		}
	}
	if len(c.cache.items) >= c.cache.max {
		var oldestKey string
		var oldest time.Time
		for k, e := range c.cache.items {
			if oldestKey == "" || e.exp.Before(oldest) {
				oldestKey = k
				oldest = e.exp
			}
		}
		delete(c.cache.items, oldestKey)
	}
	c.cache.items[key] = &cacheEntry{val: tx, exp: now.Add(ttl)}
	c.cache.mu.Unlock()
	return tx, nil
}

// Txs fetches transactions in batch (single daemon call).
func (c *Client) Txs(hashes []string) ([]*Tx, error) {
	return c.rawTxs(hashes)
}

// Pool returns the current mempool transaction hashes.
func (c *Client) Pool() ([]string, error) {
	v, err := c.cacheable("pool", 3*time.Second, func() (any, error) {
		var out rawPoolResult
		if err := c.rpc("DERO.GetTxPool", nil, &out); err != nil {
			return nil, err
		}
		if out.Status != "OK" {
			return nil, fmt.Errorf("txpool status: %s", out.Status)
		}
		return out.TxList, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]string), nil
}

// SC fetches a smart contract's code and state by SCID.
func (c *Client) SC(scid string) (*SCInfo, error) {
	v, err := c.cacheable("sc:"+scid, 30*time.Second, func() (any, error) {
		var out rawSCResult
		if err := c.rpc("DERO.GetSC", map[string]any{"scid": scid, "code": true, "variables": true}, &out); err != nil {
			return nil, err
		}
		if !scExists(out) {
			return nil, fmt.Errorf("sc %s not found", scid)
		}
		return &SCInfo{
			SCID:        strings.ToLower(scid),
			Balance:     out.Balance,
			BalanceDero: FormatMoney(out.Balance),
			Code:        out.Code,
			Balances:    out.Balances,
			StringKeys:  normalizedKeys(out.VariableStringKeys),
			Uint64Keys:  normalizedKeys(out.VariableUint64Keys),
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*SCInfo), nil
}

// scExists reports whether the node actually holds the contract. Several derod
// builds answer unknown SCIDs with status OK and an all-empty result instead of
// an RPC error, so the RPC call alone cannot distinguish a real contract from a
// miss. A contract that has never been installed has no code, no variables and
// no balance.
func scExists(out rawSCResult) bool {
	return out.Code != "" || out.Balance != 0 || len(out.Balances) > 0 ||
		len(out.VariableStringKeys) > 0 || len(out.VariableUint64Keys) > 0
}

type SCInfo struct {
	SCID        string            `json:"scid"`
	Balance     uint64            `json:"balance"`
	BalanceDero string            `json:"balance_dero"`
	Code        string            `json:"code"`
	Balances    map[string]uint64 `json:"balances"`
	StringKeys  map[string]string `json:"stringkeys"`
	Uint64Keys  map[string]string `json:"uint64keys"`
}

func normalizedKeys(in any) map[string]string {
	out := map[string]string{}
	switch m := in.(type) {
	case map[string]any:
		for k, v := range m {
			out[k] = scalarString(v)
		}
	case map[uint64]any:
		for k, v := range m {
			out[fmt.Sprintf("%d", k)] = scalarString(v)
		}
	}
	return out
}

func scalarString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%.0f", t)
	case json.Number:
		return t.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

// resolveAddress uses DERO.NameToAddress to resolve a registered name.
func (c *Client) resolveAddress(name string) (string, error) {
	var out rawNameResult
	if err := c.rpc("DERO.NameToAddress", map[string]any{"name": name}, &out); err != nil {
		return "", err
	}
	if out.Address == "" || out.Status != "OK" {
		return "", fmt.Errorf("name not registered")
	}
	return out.Address, nil
}

// AddressInfo holds the public facts about an address. DERO balances, tx
// history and registration topology are private to the owner's view key, so
// an explorer address page intentionally does not query the daemon: several
// derod builds panic on DERO.GetEncryptedBalance for arbitrary addresses.
type AddressInfo struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

// Address validates a dero1 address. No daemon round-trip is performed.
func (c *Client) Address(addr string) (*AddressInfo, error) {
	if len(addr) != 66 || !strings.HasPrefix(addr, "dero1") {
		return nil, fmt.Errorf("invalid dero address")
	}
	return &AddressInfo{Address: addr}, nil
}

// SearchResult describes what a free-text search resolved to.
type SearchResult struct {
	Type    string `json:"type,omitempty"` // block | tx | sc | address | none
	ID      string `json:"id,omitempty"`
	Address string `json:"address,omitempty"`
	Name    string `json:"name,omitempty"`
}

// Search resolves a query to an explorer object. It accepts a block
// height/topoheight, a 64-hex block/tx/scid, a dero1 address, or a
// registered name.
func (c *Client) Search(q string) (*SearchResult, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return &SearchResult{Type: "none"}, nil
	}

	isAllDigits := true
	for _, r := range q {
		if r < '0' || r > '9' {
			isAllDigits = false
			break
		}
	}

	if isAllDigits {
		if hdr, err := c.HeaderByTopoHeight(mustUint64(q)); err == nil && hdr != nil {
			return &SearchResult{Type: "block", ID: hdr.Hash}, nil
		}
		return &SearchResult{Type: "none"}, nil
	}

	if len(q) == 64 && isHex(q) {
		if _, err := c.HeaderByHash(q); err == nil {
			return &SearchResult{Type: "block", ID: q}, nil
		}
		// A DERO SCID is the tx hash of the contract's install transaction, so
		// any live contract also matches as a tx. Prefer the SC view (code +
		// state variables + balance) over the bare install-tx page.
		if _, err := c.SC(q); err == nil {
			return &SearchResult{Type: "sc", ID: q}, nil
		}
		if _, err := c.Tx(q); err == nil {
			return &SearchResult{Type: "tx", ID: q}, nil
		}
		return &SearchResult{Type: "none"}, nil
	}

	if len(q) == 66 && strings.HasPrefix(q, "dero1") {
		return &SearchResult{Type: "address", Address: q}, nil
	}

	if isValidName(q) {
		if addr, err := c.resolveAddress(q); err == nil {
			return &SearchResult{Type: "address", Address: addr, Name: q}, nil
		}
	}

	return &SearchResult{Type: "none"}, nil
}

func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func isValidName(s string) bool {
	if len(s) < 3 || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// ---- TTL cache -------------------------------------------------------------

type ttlCache struct {
	mu    sync.Mutex
	items map[string]*cacheEntry
	max   int
}

type cacheEntry struct {
	val any
	exp time.Time
}

func newTTLCache(max int) *ttlCache {
	return &ttlCache{items: map[string]*cacheEntry{}, max: max}
}

// getOrSet returns a cached value, or computes and caches it.
func (t *ttlCache) getOrSet(key string, ttl time.Duration, fn func() (any, error)) (any, error) {
	t.mu.Lock()
	if e, ok := t.items[key]; ok && time.Now().Before(e.exp) {
		t.mu.Unlock()
		return e.val, nil
	}
	t.mu.Unlock()

	val, err := fn()
	if err != nil {
		return nil, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	for k, e := range t.items {
		if now.After(e.exp) {
			delete(t.items, k)
		}
	}
	if len(t.items) >= t.max {
		var oldestKey string
		var oldest time.Time
		for k, e := range t.items {
			if oldestKey == "" || e.exp.Before(oldest) {
				oldestKey = k
				oldest = e.exp
			}
		}
		delete(t.items, oldestKey)
	}
	t.items[key] = &cacheEntry{val: val, exp: now.Add(ttl)}
	return val, nil
}
