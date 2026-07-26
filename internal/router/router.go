package router

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hyperwolf/internal/daemon"
	"hyperwolf/internal/indexer"
	"hyperwolf/internal/state"
	telapkg "hyperwolf/internal/tela"
)

type Handlers struct {
	State       *state.AppState
	Sync        *indexer.SyncManager
	TELA        *telapkg.ProxyManager
	Hub         *Hub
	DBDir       string
	TelaPort    int
	GnomonPort  int
	OnConnected func(bool)
	ShutdownCh  chan<- struct{}
}

func NewServer(addr string, webFS fs.FS, h *Handlers) *http.Server {
	mux := http.NewServeMux()

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("embed web/: %v", err)
	}

	fileServer := http.FileServer(http.FS(sub))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" || r.Method == "HEAD" {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("GET /ws", h.Hub.ServeWS)

	// Frontend API (all POST — matches send() helper in dashboard.js)
	mux.HandleFunc("POST /api/set_node", h.handleSetNode)
	mux.HandleFunc("POST /api/disconnect_node", h.handleDisconnectNode)
	mux.HandleFunc("POST /api/server_status", h.handleStatus)
	mux.HandleFunc("POST /api/get_config", h.handleConfig)
	mux.HandleFunc("POST /api/load_scid", h.handleLoadSCID)

	// Direct-fetch endpoints (also used by search.js via native fetch, not send())
	mux.HandleFunc("GET /api/config", h.handleConfig)
	mux.HandleFunc("GET /api/tela/discover", h.handleDiscoverTELA)
	mux.HandleFunc("POST /api/tela/vars", h.handleTELAVars)
	mux.HandleFunc("GET /api/scids", h.handleListSCIDs)
	mux.HandleFunc("GET /api/probe_xswd", h.handleProbeXswd)
	mux.HandleFunc("POST /api/shutdown", h.handleShutdown)

	// Settings
	mux.HandleFunc("GET /api/settings", h.handleGetSettings)
	mux.HandleFunc("POST /api/settings", h.handleSaveSettings)

	// TELA proxy routes are handled by the TELA package on its own port,
	// but we also proxy /tela/ and /add/ through this server for convenience.
	// The TELA package registers its own mux on :telaPort.
	mux.HandleFunc("GET /add/{scid}", h.handleAddTELA)
	mux.HandleFunc("GET /tela/", h.handleTELAProxy)

	// Reverse-proxy /api/* to HyperGnomon API server on :gnomonPort
	gnomonURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d/api", h.GnomonPort))
	if err == nil {
		gnomonProxy := httputil.NewSingleHostReverseProxy(gnomonURL)
		mux.Handle("GET /gnomon/", http.StripPrefix("/gnomon", gnomonProxy))
	}

	return &http.Server{
		Addr:    addr,
		Handler: mux,
	}
}

// ---- Control Handlers ----

type ctrlResp struct {
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (h *Handlers) handleSetNode(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Node string `json:"node"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.Node == "" {
		writeJSON(w, ctrlResp{OK: false, Error: "missing node"})
		return
	}
	node := strings.TrimSpace(p.Node)
	if !strings.HasPrefix(node, "http://") {
		node = "http://" + node
	}

	existing := h.State.GetNode()
	if existing == node {
		writeJSON(w, ctrlResp{OK: true, Result: "already connected"})
		return
	}

	h.State.SetNode(node)
	h.TELA.Start()
	time.Sleep(100 * time.Millisecond)
	go h.Sync.StartSync(node)
	if h.OnConnected != nil {
		h.OnConnected(true)
	}

	writeJSON(w, ctrlResp{OK: true})
}

func (h *Handlers) handleDisconnectNode(w http.ResponseWriter, r *http.Request) {
	h.Sync.StopSync()
	h.TELA.Reset()
	h.State.ClearNode()
	if h.OnConnected != nil {
		h.OnConnected(false)
	}
	writeJSON(w, ctrlResp{OK: true})
}

func (h *Handlers) handleStatus(w http.ResponseWriter, r *http.Request) {
	node := h.State.GetNode()
	connected := h.State.IsConnected()

	telaOk := false
	if connected {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", h.TelaPort))
		if err == nil {
			resp.Body.Close()
			telaOk = true
		}
	}

	gnomonOk := h.Sync.Indexer != nil && connected

	dbHeight := int64(0)
	if h.Sync.Indexer != nil {
		dbHeight = h.Sync.Indexer.LastIndexedHeight.Load()
	}

	telaAppsCount := int64(len(h.Sync.DiscoverTelaApps()))

	var daemonInfo *daemon.Info
	if connected {
		daemonInfo = daemon.GetInfo(node)
	}

	chainHeight := int64(0)
	stableHeight := int64(0)
	difficulty := int64(0)
	daemonVersion := ""
	daemonNetwork := ""
	mempoolSize := 0
	if daemonInfo != nil {
		chainHeight = daemonInfo.TopoHeight
		stableHeight = daemonInfo.StableHeight
		difficulty = daemonInfo.Difficulty
		daemonVersion = daemonInfo.Version
		daemonNetwork = daemonInfo.Network
		mempoolSize = daemonInfo.MempoolSize
	}

	var connectedAtMs int64
	if t := h.State.GetConnectedAt(); !t.IsZero() {
		connectedAtMs = t.UnixMilli()
	}

	writeJSON(w, ctrlResp{OK: true, Result: map[string]any{
		"tela":            telaOk,
		"gnomon":          gnomonOk,
		"connected":       telaOk && gnomonOk,
		"node":            node,
		"connected_at":    connectedAtMs,
		"tela_apps_count": telaAppsCount,
		"daemon": map[string]any{
			"version":      daemonVersion,
			"network":      daemonNetwork,
			"difficulty":   difficulty,
			"mempool_size": mempoolSize,
		},
		"heights": map[string]any{
			"indexed": dbHeight,
			"chain":   chainHeight,
			"stable":  stableHeight,
		},
	}})
}

func (h *Handlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, ctrlResp{OK: true, Result: map[string]any{
		"gnomon_api_port": h.GnomonPort,
		"tela_port":       h.TelaPort,
	}})
}

func (h *Handlers) handleDiscoverTELA(w http.ResponseWriter, r *http.Request) {
	apps := h.Sync.DiscoverTelaApps()
	writeJSON(w, ctrlResp{OK: true, Result: map[string]any{"apps": apps}})
}

func (h *Handlers) handleTELAVars(w http.ResponseWriter, r *http.Request) {
	var p struct {
		SCIDs []string `json:"scids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: "bad request"})
		return
	}
	node := h.State.GetNode()
	if node == "" {
		writeJSON(w, ctrlResp{OK: false, Error: "node not set"})
		return
	}
	vars := daemon.FetchSCIDVariables(node, p.SCIDs)
	writeJSON(w, ctrlResp{OK: true, Result: map[string]any{"vars": vars}})
}

func (h *Handlers) handleListSCIDs(w http.ResponseWriter, r *http.Request) {
	scids := h.TELA.GetProxiedSCIDs()
	writeJSON(w, ctrlResp{OK: true, Result: map[string]any{"scids": scids}})
}

func (h *Handlers) handleShutdown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, ctrlResp{OK: true})
	log.Println("Shutdown requested via API")
	select {
	case h.ShutdownCh <- struct{}{}:
	default:
	}
}

func (h *Handlers) handleLoadSCID(w http.ResponseWriter, r *http.Request) {
	var p struct {
		SCID string `json:"scid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.SCID == "" {
		writeJSON(w, ctrlResp{OK: false, Error: "missing scid"})
		return
	}
	scid := strings.TrimSpace(p.SCID)
	if len(scid) != 64 {
		writeJSON(w, ctrlResp{OK: false, Error: "invalid SCID"})
		return
	}
	node := h.State.GetNode()
	if node == "" {
		writeJSON(w, ctrlResp{OK: false, Error: "node not set"})
		return
	}

	addURL := fmt.Sprintf("http://127.0.0.1:%d/add/%s", h.TelaPort, scid)
	resp, err := http.Get(addURL)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: err.Error()})
		return
	}
	defer resp.Body.Close()

	var res map[string]any
	json.NewDecoder(resp.Body).Decode(&res)

	result, _ := res["result"].(map[string]any)

	if h.Sync.Indexer != nil {
		go h.Sync.IndexSCIDNow(scid)
	} else {
		log.Printf("load_scid: indexer not ready yet, skipping SCID indexing for %s", scid)
	}

	urlStr, _ := result["url"].(string)
	writeJSON(w, ctrlResp{OK: true, Result: map[string]any{
		"url": urlStr,
	}})
}

func (h *Handlers) handleAddTELA(w http.ResponseWriter, r *http.Request) {
	scid := r.PathValue("scid")
	if scid == "" {
		http.Error(w, "missing SCID", 400)
		return
	}
	// Forward to the TELA proxy's add handler
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d/add/%s", h.TelaPort, scid)
	resp, err := http.Get(proxyURL)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

func (h *Handlers) handleTELAProxy(w http.ResponseWriter, r *http.Request) {
	// Extract SCID from path: /tela/{scid}/...
	path := strings.TrimPrefix(r.URL.Path, "/tela/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	scid := parts[0]
	proxy := h.TELA.GetProxy(scid)
	if proxy == nil {
		http.Error(w, "SCID not loaded", 404)
		return
	}

	subPath := ""
	if len(parts) == 2 {
		subPath = parts[1]
	}

	if subPath == "" {
		r.URL.Path = "/"
	} else {
		r.URL.Path = "/" + subPath
	}

	log.Printf("[PROXY] %s -> %s", scid, r.URL.Path)
	proxy.ServeHTTP(w, r)
}

// ---- XSWD Probe ----

func (h *Handlers) handleProbeXswd(w http.ResponseWriter, r *http.Request) {
	client := http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get("http://127.0.0.1:44326/")
	if err != nil {
		writeJSON(w, map[string]bool{"xswd": false})
		return
	}
	resp.Body.Close()
	writeJSON(w, map[string]bool{"xswd": resp.StatusCode == 200})
}

// ---- Settings Handlers ----

type AppConfig struct {
	Bookmarks struct {
		SCIDs map[string]any `json:"scids"`
		Nodes map[string]any `json:"nodes"`
	} `json:"bookmarks"`
	Settings struct {
		DefaultNode        string `json:"defaultNode"`
		AutoConnect        bool   `json:"autoConnect"`
		DirectLoad         bool   `json:"directLoad"`
		OpenDashboardOnStart *bool `json:"openDashboardOnStart"`
		HiddenExtensions   string `json:"hiddenExtensions"`
	} `json:"settings"`
	Theme string `json:"theme"`
}

func (h *Handlers) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfgPath := filepath.Join(filepath.Dir(h.DBDir), "config.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		writeJSON(w, ctrlResp{OK: true, Result: AppConfig{}})
		return
	}
	var cfg AppConfig
	if json.Unmarshal(data, &cfg) != nil {
		writeJSON(w, ctrlResp{OK: true, Result: AppConfig{}})
		return
	}
	writeJSON(w, ctrlResp{OK: true, Result: cfg})
}

func (h *Handlers) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	var cfg AppConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: "bad request"})
		return
	}
	cfgDir := filepath.Dir(h.DBDir)
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: err.Error()})
		return
	}
	cfgPath := filepath.Join(cfgDir, "config.json")
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(cfgPath, data, 0644)
	writeJSON(w, ctrlResp{OK: true})
}
