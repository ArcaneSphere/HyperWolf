package indexer

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"

	hgindexer "github.com/hypergnomon/hypergnomon/indexer"
	hgstorage "github.com/hypergnomon/hypergnomon/storage"
)

// GnomonWSServer provides both a Gnomon-compatible JSON-RPC 2.0 WebSocket
// server (on /ws) AND an XSWD-compatible WebSocket server (on /xswd).
//
// XSWD is the wallet-bridge protocol that TELA apps use to talk to a DERO
// wallet (Engram). When Engram is not running, the DEADDROP app and similar
// TELA apps fall back to their configured fallback XSWD endpoints. This
// server implements enough of the XSWD protocol to serve registry reads and
// document-fetch requests — enough for the app to go online with Gnomon
// data even without a wallet.
type GnomonWSServer struct {
	store      hgstorage.Storage
	addr       string
	daemonURL  string
	server     *http.Server
	upgrader   websocket.Upgrader
	httpClient *http.Client
	mu         sync.Mutex
	idx        *hgindexer.Indexer // optional: for addscid_toindex, validatesc, GetInitialSCIDCode
}

// NewGnomonWSServer creates a server backed by the HyperGnomon store.
// daemonURL is the DERO daemon HTTP endpoint (e.g. "http://127.0.0.1:10102")
// used for proxying DERO.GetSC calls that the store cannot answer directly.
// idx is optional; when non-nil, methods like addscid_toindex and
// GetInitialSCIDCode are available.
func NewGnomonWSServer(addr string, store hgstorage.Storage, daemonURL string, idx *hgindexer.Indexer) *GnomonWSServer {
	if daemonURL == "" {
		daemonURL = "http://127.0.0.1:10102"
	}
	return &GnomonWSServer{
		store:     store,
		addr:      addr,
		daemonURL: daemonURL,
		idx:       idx,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Start begins listening for WebSocket connections on both /ws and /xswd.
// Blocks until the server is closed or an unrecoverable error occurs.
func (s *GnomonWSServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.serveWS)
	mux.HandleFunc("/xswd", s.serveXSWD)

	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Gnomon WS server listening on %s/ws and %s/xswd", s.addr, s.addr)
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the WebSocket server.
func (s *GnomonWSServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil {
		_ = s.server.Close()
		s.server = nil
	}
}

// ---------------------------------------------------------------------------
// JSON-RPC 2.0 wire types (shared between /ws and /xswd)
// ---------------------------------------------------------------------------

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcErrorObj    `json:"error,omitempty"`
}

type rpcErrorObj struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	rpcCodeParse          = -32700
	rpcCodeMethodNotFound = -32601
	rpcCodeInvalidParams  = -32602
	rpcCodeInternal       = -32603
	rpcCodeNotFound       = -32004
)

// ---------------------------------------------------------------------------
// /ws — raw Gnomon JSON-RPC 2.0 (legacy clients)
// ---------------------------------------------------------------------------

func (s *GnomonWSServer) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Gnomon WS upgrade: %v", err)
		return
	}
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("Gnomon WS read: %v", err)
			}
			return
		}

		resp := s.dispatchRPC(msg)
		if err := conn.WriteJSON(resp); err != nil {
			log.Printf("Gnomon WS write: %v", err)
			return
		}
	}
}

// ---------------------------------------------------------------------------
// /xswd — XSWD wallet-bridge protocol
//
// The XSWD protocol works in two phases:
//   1. The client sends an app-registration JSON object:
//        { id: "<64-hex-app-id>", name: "AppName", description: "...", url: "..." }
//      The server must reply with { accepted: true }.
//   2. After registration the client sends standard JSON-RPC 2.0 requests
//      over the same WebSocket connection.
//
// This implementation accepts any app registration and then proxies RPC
// calls (Gnomon.*, DERO.*) through the HyperGnomon store or the daemon.
// ---------------------------------------------------------------------------

// xswdAppReg matches the XSWD app-registration handshake message.
type xswdAppReg struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url,omitempty"`
}

// xswdAccepted is the reply to a successful app registration.
type xswdAccepted struct {
	Accepted bool   `json:"accepted"`
	ID       string `json:"id,omitempty"`
}

// xswdConnState tracks the state of a single XSWD WebSocket connection.
type xswdConnState struct {
	mu   sync.Mutex
	conn *websocket.Conn
	app  *xswdAppReg // set after handshake
}

func (s *GnomonWSServer) serveXSWD(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("XSWD WS upgrade: %v", err)
		return
	}

	cs := &xswdConnState{conn: conn}

	defer func() {
		cs.mu.Lock()
		cs.conn.Close()
		cs.mu.Unlock()
	}()

	// Phase 1: read the app-registration handshake
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Printf("XSWD read handshake: %v", err)
		return
	}

	var reg xswdAppReg
	if err := json.Unmarshal(msg, &reg); err != nil || reg.Name == "" {
		log.Printf("XSWD bad handshake: %s", string(msg))
		// Some Gnomon clients skip the handshake and send RPC directly.
		// Try to handle as RPC-first; if it looks like JSON-RPC, dispatch.
		var probe jsonRPCRequest
		if json.Unmarshal(msg, &probe) == nil && probe.Method != "" {
			cs.mu.Lock()
			resp := s.dispatchXSWDRPC(&probe, msg)
			cs.mu.Unlock()
			if err := conn.WriteJSON(resp); err != nil {
				log.Printf("XSWD write (no-handshake RPC): %v", err)
			}
			// Enter RPC loop
			s.xswdRPCLoop(cs)
		} else {
			// Not valid anything — close
			conn.WriteJSON(xswdAccepted{Accepted: false})
		}
		return
	}

	cs.app = &reg
	log.Printf("XSWD app registered: %q (%s)", reg.Name, reg.ID[:min(12, len(reg.ID))])

	// Reply with accepted
	reply := xswdAccepted{Accepted: true, ID: reg.ID}
	if err := conn.WriteJSON(reply); err != nil {
		log.Printf("XSWD write accepted: %v", err)
		return
	}

	// Phase 2: RPC loop
	s.xswdRPCLoop(cs)
}

func (s *GnomonWSServer) xswdRPCLoop(cs *xswdConnState) {
	for {
		_, msg, err := cs.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("XSWD read: %v", err)
			}
			return
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &rpcErrorObj{Code: rpcCodeParse, Message: "parse error"},
			}
			cs.conn.WriteJSON(resp)
			continue
		}

		cs.mu.Lock()
		resp := s.dispatchXSWDRPC(&req, msg)
		cs.mu.Unlock()

		if err := cs.conn.WriteJSON(resp); err != nil {
			log.Printf("XSWD write: %v", err)
			return
		}
	}
}

// dispatchXSWDRPC routes a parsed JSON-RPC request. It handles both Gnomon.*
// and DERO.* methods, as well as wallet-specific methods (returning
// "no wallet" errors for those that require one).
func (s *GnomonWSServer) dispatchXSWDRPC(req *jsonRPCRequest, rawMsg []byte) jsonRPCResponse {
	if req.Method == "" {
		return jsonRPCResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "missing method"},
		}
	}

	switch {
	case req.Method == "Gnomon.GetAllSCIDVariableDetails":
		return s.handleGetAllSCIDVariableDetails(req.ID, req.Params)
	case req.Method == "Gnomon.GetSCIDVariableDetailsAtTopoheight":
		return s.handleGetSCIDVariableDetailsAtTopoheight(req.ID, req.Params)
	case req.Method == "DERO.GetSC":
		return s.handleDEROGetSC(req.ID, req.Params)
	case req.Method == "dero.GetSC":
		return s.handleDEROGetSC(req.ID, req.Params)
	case req.Method == "GetAddress":
		// No wallet — return a meaningful error so the app knows it's in
		// "Gnomon mode" (read-only) rather than "wallet mode".
		return jsonRPCResponse{
			JSONRPC: "2.0", ID: req.ID,
			Result: map[string]interface{}{"address": ""},
		}
	case req.Method == "GetAllSCIDs":
		return s.handleGetAllSCIDs(req.ID, req.Params)
	case req.Method == "GetAllOwnersAndSCIDs":
		return s.handleGetAllOwnersAndSCIDs(req.ID, req.Params)
	case req.Method == "GetSCIDInteractionHeight":
		return s.handleGetSCIDInteractionHeight(req.ID, req.Params)
	case req.Method == "GetAllSCIDInvokeDetails":
		return s.handleGetAllSCIDInvokeDetails(req.ID, req.Params)
	case req.Method == "GetSafeHeight":
		return s.handleGetSafeHeight(req.ID, req.Params)
	case req.Method == "listsc":
		return s.handleListSC(req.ID, req.Params)
	case req.Method == "listsc_byclass":
		return s.handleListSCByClass(req.ID, req.Params)
	case req.Method == "listsc_variables":
		return s.handleListSCVariables(req.ID, req.Params)
	case req.Method == "listsc_ratings":
		return s.handleListSCRatings(req.ID, req.Params)
	case req.Method == "listsc_byowner" || req.Method == "getscidlist_byaddr":
		return s.handleListSCByOwner(req.ID, req.Params)
	case req.Method == "GetInitialSCIDCode":
		return s.handleGetInitialSCIDCode(req.ID, req.Params)
	case req.Method == "addscid_toindex":
		return s.handleAddSCIDToIndex(req.ID, req.Params)
	case req.Method == "validatesc":
		return s.handleValidateSC(req.ID, req.Params)
	case strings.HasPrefix(req.Method, "scinvoke") || req.Method == "scinvoke":
		return jsonRPCResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: &rpcErrorObj{Code: rpcCodeNotFound, Message: "wallet required: scinvoke not available without Engram"},
		}
	case strings.HasPrefix(req.Method, "AttemptEPOCH") || strings.HasPrefix(req.Method, "GetMaxHashesEPOCH"):
		return jsonRPCResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: &rpcErrorObj{Code: rpcCodeNotFound, Message: "wallet required: EPOCH not available without Engram"},
		}
	default:
		// Try dispatching as a raw Gnomon method (for /ws compat)
		return s.dispatchRPC(rawMsg)
	}
}

// ---------------------------------------------------------------------------
// dispatchRPC — routes a raw JSON-RPC message to the appropriate handler
// (used by both /ws and as a fallback in /xswd).
// ---------------------------------------------------------------------------

func (s *GnomonWSServer) dispatchRPC(msg []byte) jsonRPCResponse {
	var req jsonRPCRequest
	if err := json.Unmarshal(msg, &req); err != nil {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error:   &rpcErrorObj{Code: rpcCodeParse, Message: "parse error"},
		}
	}
	if req.Method == "" {
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "missing method"},
		}
	}

	// Strip the "Gnomon." prefix for Gnomon-specific methods
	method := req.Method
	if strings.HasPrefix(method, "Gnomon.") {
		method = strings.TrimPrefix(method, "Gnomon.")
	}

	switch method {
	case "GetAllSCIDVariableDetails":
		return s.handleGetAllSCIDVariableDetails(req.ID, req.Params)
	case "GetSCIDVariableDetailsAtTopoheight":
		return s.handleGetSCIDVariableDetailsAtTopoheight(req.ID, req.Params)
	case "DERO.GetSC":
		return s.handleDEROGetSC(req.ID, req.Params)
	case "GetAllSCIDs":
		return s.handleGetAllSCIDs(req.ID, req.Params)
	case "GetAllOwnersAndSCIDs":
		return s.handleGetAllOwnersAndSCIDs(req.ID, req.Params)
	case "GetSCIDInteractionHeight":
		return s.handleGetSCIDInteractionHeight(req.ID, req.Params)
	case "GetAllSCIDInvokeDetails":
		return s.handleGetAllSCIDInvokeDetails(req.ID, req.Params)
	case "GetSafeHeight":
		return s.handleGetSafeHeight(req.ID, req.Params)
	case "listsc":
		return s.handleListSC(req.ID, req.Params)
	case "listsc_byclass":
		return s.handleListSCByClass(req.ID, req.Params)
	case "listsc_variables":
		return s.handleListSCVariables(req.ID, req.Params)
	case "listsc_ratings":
		return s.handleListSCRatings(req.ID, req.Params)
	case "listsc_byowner", "getscidlist_byaddr":
		return s.handleListSCByOwner(req.ID, req.Params)
	case "GetInitialSCIDCode":
		return s.handleGetInitialSCIDCode(req.ID, req.Params)
	case "addscid_toindex":
		return s.handleAddSCIDToIndex(req.ID, req.Params)
	case "validatesc":
		return s.handleValidateSC(req.ID, req.Params)
	default:
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcErrorObj{
				Code:    rpcCodeMethodNotFound,
				Message: "method not found: " + req.Method,
			},
		}
	}
}

// ---------------------------------------------------------------------------
// Params types
// ---------------------------------------------------------------------------

type scidVarParams struct {
	SCID   string `json:"scid"`
	Height int64  `json:"height"`
}

type getSCParams struct {
	SCID       string   `json:"scid"`
	Code       bool     `json:"code"`
	Variables  bool     `json:"variables"`
	Topoheight int64    `json:"topoheight"`
	KeysString []string `json:"keysstring,omitempty"`
	KeysUint64 []uint64 `json:"keysuint64,omitempty"`
}

// ---------------------------------------------------------------------------
// Handler: GetAllSCIDVariableDetails
//
// Prefers the daemon (authoritative source) over the local HyperGnomon
// store, which may have only partially indexed a contract's variables
// (e.g. registry contracts with 200+ keys under lazy PostScanVarsMode).
// Falls back to the HyperGnomon store only when the daemon is unreachable.
// ---------------------------------------------------------------------------

func (s *GnomonWSServer) handleGetAllSCIDVariableDetails(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	if len(params) == 0 {
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "missing params"},
		}
	}
	var p scidVarParams
	if err := json.Unmarshal(params, &p); err != nil {
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: err.Error()},
		}
	}
	if p.SCID == "" {
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "missing scid"},
		}
	}

	// Query the daemon first (authoritative source).
	pairs, err := s.varsFromDaemon(p.SCID)
	if err != nil {
		// Daemon unreachable — try the HyperGnomon store as a best-effort
		// fallback. Partial data is better than nothing.
		log.Printf("Gnomon WS: GetAllSCIDVariableDetails daemon error for %s: %v", truncate(p.SCID, 12), err)
		if spairs, serr := s.varsFromStore(p.SCID); serr == nil && len(spairs) > 0 {
			return jsonRPCResponse{
				ID: id, JSONRPC: "2.0",
				Result: map[string]interface{}{"stringkeys": spairs},
			}
		}
		// Both sources failed — return empty result rather than an error,
		// so the TELA JS client doesn't throw.
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Result: map[string]interface{}{"stringkeys": []*kvPair{}},
		}
	}

	return jsonRPCResponse{
		ID: id, JSONRPC: "2.0",
		Result: map[string]interface{}{"stringkeys": pairs},
	}
}

// varsFromStore queries the HyperGnomon store for SCID variables.
func (s *GnomonWSServer) varsFromStore(scid string) ([]*kvPair, error) {
	meta, err := s.store.GetSCIDClass(scid)
	if err != nil {
		return nil, err
	}
	height := int64(0)
	if meta != nil {
		height = meta.LastHeight
	}
	if height <= 0 {
		return nil, fmt.Errorf("no height known for %s", truncate(scid, 12))
	}

	raw, err := s.store.GetSCIDVariableDetailsAtHeight(scid, height)
	if err != nil {
		return nil, err
	}

	out := make([]*kvPair, 0, len(raw))
	for _, v := range raw {
		out = append(out, &kvPair{
			Key:   stringify(v.Key),
			Value: stringify(v.Value),
		})
	}
	return out, nil
}

// varsFromDaemon calls the daemon's DERO.GetSC with variables=true and
// converts the daemon's stringkeys/uint64keys maps into Gnomon's flat
// [{Key, Value}] array format. Uses the keysstring parameter to bypass
// the daemon's per-response variable count limit (~60 keys) by fetching
// all known keys in a single targeted call.
func (s *GnomonWSServer) varsFromDaemon(scid string) ([]*kvPair, error) {
	// Phase 1: fetch keys to discover the contract's variable count.
	// Even if truncated, the first ~60 keys reveal the key naming pattern.
	resp, err := s.callDaemonGetSC(scid, false, true, -1, nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.Result == nil {
		return []*kvPair{}, nil
	}

	// Build the full key list from what the daemon returned.
	var allKeys []string
	if resp.Result.StringKeys != nil {
		for k := range resp.Result.StringKeys {
			allKeys = append(allKeys, k)
		}
	}

	var uintKeys []uint64
	if resp.Result.Uint64Keys != nil {
		for k := range resp.Result.Uint64Keys {
			// k is a string that may represent a uint64
			var u uint64
			// uint64keys come as string keys from the daemon's JSON map
			if _, err := fmt.Sscanf(k, "%d", &u); err == nil {
				uintKeys = append(uintKeys, u)
			}
		}
	}

	// If the response was truncated (has more keys than returned),
	// we need to fetch the remaining keys. The daemon's truncation
	// limit is per-response; using keysstring bypasses it.
	if resp.Result.StringKeysTruncated {
		// Build a list of all known string keys.
		ks := make([]string, len(allKeys))
		copy(ks, allKeys)

		// Fetch all keys in one targeted call to bypass truncation.
		resp, err = s.callDaemonGetSC(scid, false, true, -1, ks, uintKeys)
		if err != nil {
			return nil, err
		}
		if resp.Result == nil {
			return []*kvPair{}, nil
		}

		// Re-read the full key set from the targeted response.
		allKeys = nil
		if resp.Result.StringKeys != nil {
			for k := range resp.Result.StringKeys {
				allKeys = append(allKeys, k)
			}
		}
	}

	var pairs []*kvPair

	// stringkeys: map[string]interface{} — values may be strings or numbers
	for _, k := range allKeys {
		if v, ok := resp.Result.StringKeys[k]; ok {
			pairs = append(pairs, &kvPair{Key: k, Value: stringify(v)})
		}
	}

	// uint64keys: stored as float64 in JSON, or as string
	if resp.Result.Uint64Keys != nil {
		for k, v := range resp.Result.Uint64Keys {
			pairs = append(pairs, &kvPair{Key: k, Value: stringify(v)})
		}
	}

	if pairs == nil {
		pairs = []*kvPair{}
	}
	return pairs, nil
}

// kvPair is the Gnomon variable format: [{Key: "k", Value: "v"}, ...]
type kvPair struct {
	Key   interface{} `json:"Key"`
	Value interface{} `json:"Value"`
}

// stringify converts an interface{} to its string representation for
// JSON-safe Gnomon variable values. The typed encoding can produce
// []byte keys; the daemon can return float64 for uint64 values.
// When a string value is all-lowercase-hex of even length and decodes to
// printable UTF-8, we decode it so that TELA apps (which expect decoded
// text) receive human-readable titles, authors, etc. instead of raw hex.
// tryDecodeHex attempts to decode a hex-encoded UTF-8 string.
// Returns true when the string is all hex, even length, and the decoded bytes
// are valid printable UTF-8. Returns false for values that should stay as-is
// (decimal numbers like "2026", plain text, etc.).
func tryDecodeHex(s string) (string, bool) {
	if len(s) == 0 || len(s)%2 != 0 {
		return s, false
	}
	// Quick check: every byte must be 0-9a-fA-F
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return s, false
		}
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return s, false
	}
	// Must be valid UTF-8
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError {
			return s, false
		}
		i += size
	}
	return string(b), true
}

func stringify(v interface{}) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		if decoded, ok := tryDecodeHex(s); ok {
			return decoded
		}
		return s
	case []byte:
		return string(s)
	case fmt.Stringer:
		return s.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ---------------------------------------------------------------------------
// Handler: GetSCIDVariableDetailsAtTopoheight
// ---------------------------------------------------------------------------

func (s *GnomonWSServer) handleGetSCIDVariableDetailsAtTopoheight(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	if len(params) == 0 {
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "missing params"},
		}
	}
	var p scidVarParams
	if err := json.Unmarshal(params, &p); err != nil {
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: err.Error()},
		}
	}
	if p.SCID == "" {
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "missing scid"},
		}
	}

	// Prefer the daemon (authoritative source) over the local store.
	pairs, err := s.varsFromDaemon(p.SCID)
	if err != nil {
		log.Printf("Gnomon WS: GetSCIDVariableDetailsAtTopoheight daemon error: %v", err)
		// Daemon unreachable — try the store as a best-effort fallback.
		if spairs, serr := s.varsFromStore(p.SCID); serr == nil && len(spairs) > 0 {
			return jsonRPCResponse{
				ID: id, JSONRPC: "2.0",
				Result: map[string]interface{}{"stringkeys": spairs},
			}
		}
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Result: map[string]interface{}{"stringkeys": []*kvPair{}},
		}
	}

	return jsonRPCResponse{
		ID: id, JSONRPC: "2.0",
		Result: map[string]interface{}{"stringkeys": pairs},
	}
}

// ---------------------------------------------------------------------------
// Handler: DERO.GetSC
//
// Proxies to the DERO daemon's JSON-RPC endpoint. This is critical for
// TELA apps like DEADDROP that need to read document content from contract
// code — the HyperGnomon store indexes variables but not raw contract code.
// ---------------------------------------------------------------------------

// daemonGetSCRequest matches the daemon's DERO.GetSC parameter shape.
type daemonGetSCRequest struct {
	SCID       string   `json:"scid"`
	Code       bool     `json:"code"`
	Variables  bool     `json:"variables"`
	Topoheight int64    `json:"topoheight"`
	KeysString []string `json:"keysstring,omitempty"`
	KeysUint64 []uint64 `json:"keysuint64,omitempty"`
}

// daemonGetSCResult mirrors the daemon's GetSC result.
// The daemon places stringkeys/uint64keys at the top level of the response,
// NOT nested under a "variables" wrapper. Captured correctly here.
// Values in stringkeys can be strings OR numbers (the DERO daemon returns
// numeric values as JSON numbers, e.g. 12345 without quotes). We use
// interface{} here to handle both types, then stringify() when reading.
type daemonGetSCResult struct {
	Code                string                 `json:"code"`
	StringKeys          map[string]interface{} `json:"stringkeys,omitempty"`
	Uint64Keys          map[string]interface{} `json:"uint64keys,omitempty"`
	Balances            map[string]interface{} `json:"balances,omitempty"`
	StringKeysTruncated bool                   `json:"stringkeys_truncated,omitempty"`
	Status              string                 `json:"status"`
}

// daemonRPCWrapper wraps a daemon JSON-RPC response.
type daemonRPCWrapper struct {
	JSONRPC string            `json:"jsonrpc"`
	ID      json.RawMessage   `json:"id"`
	Result  *daemonGetSCResult `json:"result,omitempty"`
	Error   *rpcErrorObj      `json:"error,omitempty"`
}

// callDaemonGetSC calls the daemon's DERO.GetSC method via HTTP and returns
// the parsed response. Both handleDEROGetSC and varsFromDaemon use this.
func (s *GnomonWSServer) callDaemonGetSC(scid string, code, variables bool, topoheight int64, keysstring []string, keysuint64 []uint64) (*daemonRPCWrapper, error) {
	if topoheight <= 0 {
		topoheight = -1 // latest
	}

	params := daemonGetSCRequest{
		SCID:       scid,
		Code:       code,
		Variables:  variables,
		Topoheight: topoheight,
		KeysString: keysstring,
		KeysUint64: keysuint64,
	}

	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "DERO.GetSC",
		"params":  params,
	}
	reqJSON, _ := json.Marshal(reqBody)

	daemonURL := s.daemonURL + "/json_rpc"
	resp, err := s.httpClient.Post(daemonURL, "application/json", strings.NewReader(string(reqJSON)))
	if err != nil {
		return nil, fmt.Errorf("daemon unreachable: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read error: %w", err)
	}

	var daemonResp daemonRPCWrapper
	if err := json.Unmarshal(body, &daemonResp); err != nil {
		return nil, fmt.Errorf("bad daemon response: %w", err)
	}

	if daemonResp.Error != nil {
		return nil, fmt.Errorf("daemon error: %s", daemonResp.Error.Message)
	}

	return &daemonResp, nil
}

func (s *GnomonWSServer) handleDEROGetSC(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	if len(params) == 0 {
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "missing params"},
		}
	}

	var p getSCParams
	if err := json.Unmarshal(params, &p); err != nil {
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: err.Error()},
		}
	}
	if p.SCID == "" {
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "missing scid"},
		}
	}

	daemonResp, err := s.callDaemonGetSC(p.SCID, p.Code, p.Variables, p.Topoheight, p.KeysString, p.KeysUint64)
	if err != nil {
		log.Printf("DERO.GetSC proxy: %v", err)
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Error: &rpcErrorObj{Code: rpcCodeInternal, Message: err.Error()},
		}
	}

	if daemonResp.Result == nil {
		return jsonRPCResponse{
			ID: id, JSONRPC: "2.0",
			Result: map[string]interface{}{
				"code":      "",
				"variables": map[string]interface{}{},
			},
		}
	}

	// Handle daemon truncation: when stringkeys_truncated is true the
	// daemon returned only the first ~60 variable keys. We re-fetch
	// with the full key list to ensure the caller gets all data.
	if daemonResp.Result.StringKeysTruncated {
		if daemonResp.Result.StringKeys != nil {
			var allKeys []string
			var uintKeys []uint64
			for k := range daemonResp.Result.StringKeys {
				allKeys = append(allKeys, k)
			}
			for k := range daemonResp.Result.Uint64Keys {
				var u uint64
				if _, err := fmt.Sscanf(k, "%d", &u); err == nil {
					uintKeys = append(uintKeys, u)
				}
			}
			// Only re-fetch if caller didn't already specify keysstring.
			if len(p.KeysString) == 0 {
				second, err2 := s.callDaemonGetSC(p.SCID, p.Code, p.Variables, p.Topoheight, allKeys, uintKeys)
				if err2 == nil && second.Result != nil {
					daemonResp = second
				}
			}
		}
	}

	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: daemonResp.Result}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ---------------------------------------------------------------------------
// Missing Gnomon-compatible WS methods
//
// HyperGnomon's native WSServer provides ~20 methods. The custom
// GnomonWSServer only handles the core three. These additions bring the
// minimum set needed by TELA apps like CipherSamizdat that resolve SCID
// metadata (names, descriptions, ratings) from book DOC contracts.
// ---------------------------------------------------------------------------

func (s *GnomonWSServer) handleGetAllSCIDs(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	scids, err := s.store.GetAllSCIDs()
	if err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "storage error: " + err.Error()}}
	}
	if scids == nil {
		scids = []string{}
	}
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: map[string]interface{}{"scids": scids}}
}

func (s *GnomonWSServer) handleGetAllOwnersAndSCIDs(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	owners, err := s.store.GetAllOwnersAndSCIDs()
	if err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "storage error: " + err.Error()}}
	}
	if owners == nil {
		owners = map[string]string{}
	}
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: owners}
}

func (s *GnomonWSServer) handleGetSCIDInteractionHeight(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	var p scidVarParams
	if err := json.Unmarshal(params, &p); err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: err.Error()}}
	}
	if p.SCID == "" {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "missing scid"}}
	}
	heights, err := s.store.GetSCIDInteractionHeights(p.SCID)
	if err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "storage error: " + err.Error()}}
	}
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: heights}
}

func (s *GnomonWSServer) handleGetAllSCIDInvokeDetails(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	var p scidVarParams
	if err := json.Unmarshal(params, &p); err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: err.Error()}}
	}
	if p.SCID == "" {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "missing scid"}}
	}
	details, err := s.store.GetInvokeDetailsBySCID(p.SCID)
	if err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "storage error: " + err.Error()}}
	}
	// Return raw details as JSON; let the caller interpret.
	var result interface{} = details
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: result}
}

func (s *GnomonWSServer) handleGetSafeHeight(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: map[string]interface{}{"safe_height": 0}}
}

func (s *GnomonWSServer) handleListSC(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	type listSCParams struct {
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
		Class  string `json:"class"`
	}
	type listSCResult struct {
		SCID          string `json:"scid"`
		Owner         string `json:"owner"`
		Class         string `json:"class"`
		InstallHeight int64  `json:"install_height"`
		Name          string `json:"name"`
	}
	type listSCResponse struct {
		Count   int            `json:"count"`
		Offset  int            `json:"offset"`
		Limit   int            `json:"limit"`
		Results []listSCResult `json:"results"`
	}
	var p listSCParams
	if len(params) > 0 {
		_ = json.Unmarshal(params, &p)
	}
	if p.Limit <= 0 || p.Limit > 1000 {
		p.Limit = 100
	}
	if p.Offset < 0 {
		p.Offset = 0
	}

	installs, _ := s.store.GetClassInstalls(p.Class, 0)
	total := len(installs)
	if p.Offset >= total {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: listSCResponse{Count: total, Offset: p.Offset, Limit: p.Limit, Results: []listSCResult{}}}
	}
	end := p.Offset + p.Limit
	if end > total {
		end = total
	}
	window := installs[p.Offset:end]

	scids := make([]string, 0, len(window))
	for _, inst := range window {
		scids = append(scids, inst.SCID)
	}
	owners, _ := s.store.GetOwnersForSCIDs(scids)

	results := make([]listSCResult, 0, len(window))
	for _, inst := range window {
		r := listSCResult{SCID: inst.SCID, Owner: owners[inst.SCID], InstallHeight: inst.InstallHeight}
		if inst.Meta != nil {
			r.Class = inst.Meta.Class
			r.Name = inst.Meta.Name
		}
		results = append(results, r)
	}
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: listSCResponse{Count: total, Offset: p.Offset, Limit: p.Limit, Results: results}}
}

func (s *GnomonWSServer) handleListSCByClass(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	type listSCByClassParams struct {
		Class string `json:"class"`
		Limit int    `json:"limit"`
	}
	type listSCResult struct {
		SCID          string `json:"scid"`
		Owner         string `json:"owner"`
		Class         string `json:"class"`
		InstallHeight int64  `json:"install_height"`
		Name          string `json:"name"`
	}
	type listSCResponse struct {
		Count   int            `json:"count"`
		Offset  int            `json:"offset"`
		Limit   int            `json:"limit"`
		Results []listSCResult `json:"results"`
	}
	var p listSCByClassParams
	_ = json.Unmarshal(params, &p)
	if p.Class == "" {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "missing class"}}
	}
	if p.Limit <= 0 || p.Limit > 1000 {
		p.Limit = 100
	}

	installs, err := s.store.GetClassInstalls(p.Class, p.Limit)
	if err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "storage error: " + err.Error()}}
	}
	scids := make([]string, 0, len(installs))
	for _, inst := range installs {
		scids = append(scids, inst.SCID)
	}
	owners, _ := s.store.GetOwnersForSCIDs(scids)

	results := make([]listSCResult, 0, len(installs))
	for _, inst := range installs {
		r := listSCResult{SCID: inst.SCID, Owner: owners[inst.SCID], InstallHeight: inst.InstallHeight}
		if inst.Meta != nil {
			r.Class = inst.Meta.Class
			r.Name = inst.Meta.Name
		}
		results = append(results, r)
	}
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: listSCResponse{Count: len(results), Offset: 0, Limit: p.Limit, Results: results}}
}

func (s *GnomonWSServer) handleListSCVariables(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	type listSCVariablesParams struct {
		SCID   string `json:"scid"`
		Height int64  `json:"height"`
	}
	type listSCVariablesRow struct {
		Key   interface{} `json:"key"`
		Value interface{} `json:"value"`
	}
	type listSCVariablesResponse struct {
		SCID      string               `json:"scid"`
		Height    int64                `json:"height"`
		Variables []listSCVariablesRow `json:"variables"`
	}
	var p listSCVariablesParams
	_ = json.Unmarshal(params, &p)
	if len(p.SCID) != 64 {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "invalid scid"}}
	}

	height := p.Height
	if height <= 0 {
		meta, _ := s.store.GetSCIDClass(p.SCID)
		if meta == nil {
			return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeNotFound, Message: "scid not found"}}
		}
		height = meta.LastHeight
	}

	vars, err := s.store.GetSCIDVariableDetailsAtHeight(p.SCID, height)
	if err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "storage error: " + err.Error()}}
	}

	rows := make([]listSCVariablesRow, 0, len(vars))
	for _, v := range vars {
		if v == nil {
			continue
		}
		rows = append(rows, listSCVariablesRow{Key: v.Key, Value: v.Value})
	}
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: listSCVariablesResponse{SCID: p.SCID, Height: height, Variables: rows}}
}

func (s *GnomonWSServer) handleListSCRatings(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	type listSCRatingsParams struct {
		SCID   string `json:"scid"`
		Height int64  `json:"height,omitempty"`
	}
	type ratingEntry struct {
		Rater  string  `json:"rater"`
		Score  float64 `json:"score"`
		Height int64   `json:"height"`
	}
	type listSCRatingsResponse struct {
		SCID    string        `json:"scid"`
		Height  int64         `json:"height"`
		Ratings []ratingEntry `json:"ratings"`
		Count   int           `json:"count"`
		Avg     float64       `json:"avg"`
	}
	var p listSCRatingsParams
	_ = json.Unmarshal(params, &p)
	if len(p.SCID) != 64 {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "invalid scid"}}
	}

	ratings, err := s.store.GetRatingsForSCID(p.SCID, p.Height)
	if err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "storage error: " + err.Error()}}
	}
	height := p.Height
	if height <= 0 && len(ratings) > 0 {
		height = ratings[0].Height
	}
	var sum float64
	entries := make([]ratingEntry, 0, len(ratings))
	for _, r := range ratings {
		sum += r.Score
		entries = append(entries, ratingEntry{Rater: r.Rater, Score: r.Score, Height: r.Height})
	}
	var avg float64
	if len(ratings) > 0 {
		avg = sum / float64(len(ratings))
	}
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: listSCRatingsResponse{SCID: p.SCID, Height: height, Ratings: entries, Count: len(ratings), Avg: avg}}
}

func (s *GnomonWSServer) handleListSCByOwner(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	type listSCByOwnerParams struct {
		Owner string `json:"owner"`
	}
	type listSCByOwnerEntry struct {
		SCID  string `json:"scid"`
		Class string `json:"class,omitempty"`
		Name  string `json:"name,omitempty"`
	}
	type listSCByOwnerResponse struct {
		Owner string               `json:"owner"`
		SCIDs []listSCByOwnerEntry `json:"scids"`
		Count int                  `json:"count"`
	}
	var p listSCByOwnerParams
	_ = json.Unmarshal(params, &p)
	if p.Owner == "" {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "missing owner"}}
	}
	scids, err := s.store.GetSCIDsByOwner(p.Owner)
	if err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "storage error: " + err.Error()}}
	}
	out := make([]listSCByOwnerEntry, 0, len(scids))
	for _, scid := range scids {
		entry := listSCByOwnerEntry{SCID: scid}
		meta, _ := s.store.GetSCIDClass(scid)
		if meta != nil {
			entry.Class = meta.Class
			entry.Name = meta.Name
		}
		out = append(out, entry)
	}
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: listSCByOwnerResponse{Owner: p.Owner, SCIDs: out, Count: len(out)}}
}

func (s *GnomonWSServer) handleGetInitialSCIDCode(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	type getInitialSCIDCodeParams struct {
		SCID string `json:"scid"`
	}
	type getInitialSCIDCodeResult struct {
		SCID          string `json:"scid"`
		Code          string `json:"code"`
		InstallHeight int64  `json:"install_height"`
	}
	if s.idx == nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "indexer not configured"}}
	}
	var p getInitialSCIDCodeParams
	_ = json.Unmarshal(params, &p)
	if len(p.SCID) != 64 {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "invalid scid"}}
	}
	entry, err := s.idx.GetSCCode(p.SCID)
	if err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "indexer error: " + err.Error()}}
	}
	if entry == nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeNotFound, Message: "scid not found"}}
	}
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: getInitialSCIDCodeResult{SCID: p.SCID, Code: entry.Code, InstallHeight: entry.InstallHeight}}
}

func (s *GnomonWSServer) handleAddSCIDToIndex(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	type addSCIDToIndexParams struct {
		SCID          string `json:"scid"`
		VarsOnly      bool   `json:"varsonly"`
		SkipFSRecheck bool   `json:"skipfsrecheck"`
	}
	if s.idx == nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "indexer not configured"}}
	}
	var p addSCIDToIndexParams
	_ = json.Unmarshal(params, &p)
	if len(p.SCID) != 64 {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "invalid scid"}}
	}
	res, err := s.idx.IndexSingleSCID(p.SCID, p.VarsOnly, p.SkipFSRecheck)
	if err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "indexer error: " + err.Error()}}
	}
	meta := res.ClassMeta
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: map[string]interface{}{
		"scid":           res.SCID,
		"class":          meta.Class,
		"tags":           meta.Tags,
		"name":           meta.Name,
		"description":    meta.Desc,
		"icon_url":       meta.IconURL,
		"install_height": meta.InstallHeight,
		"last_height":    meta.LastHeight,
		"vars_count":     res.VarsCount,
		"from_cache":     res.FromCache,
	}}
}

func (s *GnomonWSServer) handleValidateSC(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	type validateSCParams struct {
		SCID string `json:"scid"`
	}
	if s.idx == nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "indexer not configured"}}
	}
	var p validateSCParams
	_ = json.Unmarshal(params, &p)
	if len(p.SCID) != 64 {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInvalidParams, Message: "invalid scid"}}
	}
	meta, _ := s.store.GetSCIDClass(p.SCID)
	if meta == nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: map[string]interface{}{"scid": p.SCID, "found": false}}
	}
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: map[string]interface{}{
		"scid":            p.SCID,
		"found":           true,
		"class":           meta.Class,
		"tags":            meta.Tags,
		"name":            meta.Name,
		"description":     meta.Desc,
		"durl":            meta.DURL,
		"install_height":  meta.InstallHeight,
		"last_height":     meta.LastHeight,
	}}
}

func (s *GnomonWSServer) handleListSCIDs(id json.RawMessage, params json.RawMessage) jsonRPCResponse {
	scids, err := s.store.GetAllSCIDs()
	if err != nil {
		return jsonRPCResponse{ID: id, JSONRPC: "2.0", Error: &rpcErrorObj{Code: rpcCodeInternal, Message: "storage error: " + err.Error()}}
	}
	if scids == nil {
		scids = []string{}
	}
	return jsonRPCResponse{ID: id, JSONRPC: "2.0", Result: map[string]interface{}{"scids": scids}}
}
