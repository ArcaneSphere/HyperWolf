package router

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
	LogService  *LogService
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
	mux.HandleFunc("GET /api/update-check", h.handleUpdateCheck)
	mux.HandleFunc("GET /api/rss", h.handleRSSFeed)
	mux.HandleFunc("GET /api/tela/discover", h.handleDiscoverTELA)
	mux.HandleFunc("POST /api/tela/vars", h.handleTELAVars)
	mux.HandleFunc("GET /api/scids", h.handleListSCIDs)
	mux.HandleFunc("GET /api/probe_xswd", h.handleProbeXswd)
	mux.HandleFunc("POST /api/shutdown", h.handleShutdown)

	// Settings
	mux.HandleFunc("GET /api/settings", h.handleGetSettings)
	mux.HandleFunc("POST /api/settings", h.handleSaveSettings)

	// Logs
	mux.HandleFunc("GET /api/logs", h.handleGetLogs)

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

	gnomonOk := h.Sync.HasIndexer() && connected

	dbHeight := h.Sync.GetIndexedHeight()

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
		"version":         "0.8.5",
	}})
}

// GitHubRelease represents a GitHub release response.
type GitHubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Prerelease  bool   `json:"prerelease"`
}

// UpdateCheckResponse is the response for the update check endpoint.
type UpdateCheckResponse struct {
	UpdateAvailable bool   `json:"update_available"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	ReleaseURL      string `json:"release_url"`
	ReleaseNotes    string `json:"release_notes"`
	PublishedAt     string `json:"published_at"`
}

// handleUpdateCheck checks for application updates via GitHub API.
func (h *Handlers) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	currentVersion := "0.8.5"

	// Fetch latest release from GitHub
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/ArcaneSphere/HyperWolf/releases/latest", nil)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: "failed to create request"})
		return
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "HyperWolf-UpdateCheck")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: "failed to fetch release info"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeJSON(w, ctrlResp{OK: false, Error: fmt.Sprintf("GitHub API returned %d", resp.StatusCode)})
		return
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: "failed to parse release info"})
		return
	}

	// Extract version from tag (e.g., "v0.8.5" -> "0.8.5")
	latestVersion := strings.TrimPrefix(release.TagName, "v")

	// Simple version comparison
	updateAvailable := compareVersions(latestVersion, currentVersion) > 0

	response := UpdateCheckResponse{
		UpdateAvailable: updateAvailable,
		CurrentVersion:  currentVersion,
		LatestVersion:   latestVersion,
		ReleaseURL:      release.HTMLURL,
		ReleaseNotes:    release.Body,
		PublishedAt:     release.PublishedAt,
	}

	writeJSON(w, ctrlResp{OK: true, Result: response})
}

// compareVersions compares two semantic version strings.
// Returns 1 if v1 > v2, -1 if v1 < v2, 0 if equal.
func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int
		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &p1)
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &p2)
		}
		if p1 > p2 {
			return 1
		}
		if p1 < p2 {
			return -1
		}
	}
	return 0
}

// RSSItem represents a single RSS/Atom feed item.
type RSSItem struct {
	Title       string `json:"title"`
	Link        string `json:"link"`
	PubDate     string `json:"pub_date"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// RSSFeedResponse is the response for the RSS feed endpoint.
type RSSFeedResponse struct {
	Items   []RSSItem `json:"items"`
	Title   string    `json:"title"`
	Link    string    `json:"link"`
	Updated string    `json:"updated"`
	Error   string    `json:"error,omitempty"`
}

// RSS feed cache
var (
	rssCache      *RSSFeedResponse
	rssCacheTime  time.Time
	rssCacheMutex = make(chan struct{}, 1)
)

func init() {
	rssCacheMutex <- struct{}{}
}

// handleRSSFeed fetches and parses an RSS/Atom feed.
func (h *Handlers) handleRSSFeed(w http.ResponseWriter, r *http.Request) {
	feedURL := r.URL.Query().Get("url")
	if feedURL == "" {
		// Default to the Dero.World AnotherWorld feed
		feedURL = "https://dero.world/anotherworld/feed/"
	}

	// Check cache (5 min TTL)
	select {
	case <-rssCacheMutex:
		if rssCache != nil && time.Since(rssCacheTime) < 5*time.Minute && rssCache.Title != "" {
			// Return cached response but update the URL to match request
			cached := *rssCache
			rssCacheMutex <- struct{}{}
			writeJSON(w, ctrlResp{OK: true, Result: cached})
			return
		}
		rssCacheMutex <- struct{}{}
	default:
		// Cache miss or expired, proceed to fetch
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: "failed to create request"})
		return
	}
	req.Header.Set("User-Agent", "HyperWolf-RSS/1.0")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: "failed to fetch feed"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeJSON(w, ctrlResp{OK: false, Error: fmt.Sprintf("feed returned %d", resp.StatusCode)})
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: "failed to read feed"})
		return
	}

	// Try parsing as RSS 2.0 first, then Atom 1.0
	items, feedTitle, feedLink, feedUpdated := parseFeed(body)

	response := RSSFeedResponse{
		Items:   items,
		Title:   feedTitle,
		Link:    feedLink,
		Updated: feedUpdated,
	}

	// Update cache
	select {
	case <-rssCacheMutex:
		rssCache = &response
		rssCacheTime = time.Now()
		rssCacheMutex <- struct{}{}
	default:
	}

	writeJSON(w, ctrlResp{OK: true, Result: response})
}

// parseFeed attempts to parse RSS 2.0 or Atom 1.0 feed.
func parseFeed(data []byte) ([]RSSItem, string, string, string) {
	// Try RSS 2.0
	var rss struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Title         string `xml:"title"`
			Link          string `xml:"link"`
			LastBuildDate string `xml:"lastBuildDate"`
			Items         []struct {
				Title       string `xml:"title"`
				Link        string `xml:"link"`
				PubDate     string `xml:"pubDate"`
				Description string `xml:"description"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(data, &rss); err == nil && len(rss.Channel.Items) > 0 {
		items := make([]RSSItem, 0, len(rss.Channel.Items))
		for _, item := range rss.Channel.Items {
			items = append(items, RSSItem{
				Title:       strings.TrimSpace(item.Title),
				Link:        strings.TrimSpace(item.Link),
				PubDate:     strings.TrimSpace(item.PubDate),
				Description: strings.TrimSpace(item.Description),
				Source:      strings.TrimSpace(rss.Channel.Title),
			})
		}
		return items, rss.Channel.Title, rss.Channel.Link, rss.Channel.LastBuildDate
	}

	// Try Atom 1.0
	var atom struct {
		XMLName xml.Name `xml:"feed"`
		Title   string   `xml:"title"`
		Link    []struct {
			Href string `xml:"href,attr"`
			Rel  string `xml:"rel,attr"`
		} `xml:"link"`
		Updated string `xml:"updated"`
		Entries []struct {
			Title string `xml:"title"`
			Link  []struct {
				Href string `xml:"href,attr"`
			} `xml:"link"`
			Published string `xml:"published"`
			Updated   string `xml:"updated"`
			Summary   string `xml:"summary"`
			Content   string `xml:"content"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(data, &atom); err == nil && len(atom.Entries) > 0 {
		// Find alternate link
		feedLink := ""
		for _, l := range atom.Link {
			if l.Rel == "alternate" || l.Rel == "" {
				feedLink = l.Href
				break
			}
		}
		if feedLink == "" && len(atom.Link) > 0 {
			feedLink = atom.Link[0].Href
		}

		items := make([]RSSItem, 0, len(atom.Entries))
		for _, entry := range atom.Entries {
			link := ""
			if len(entry.Link) > 0 {
				link = entry.Link[0].Href
			}
			pubDate := entry.Published
			if pubDate == "" {
				pubDate = entry.Updated
			}
			desc := entry.Summary
			if desc == "" {
				desc = entry.Content
			}
			items = append(items, RSSItem{
				Title:       strings.TrimSpace(entry.Title),
				Link:        strings.TrimSpace(link),
				PubDate:     strings.TrimSpace(pubDate),
				Description: strings.TrimSpace(desc),
				Source:      strings.TrimSpace(atom.Title),
			})
		}
		return items, atom.Title, feedLink, atom.Updated
	}

	return nil, "", "", ""
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

	// Read and log the raw body for diagnostics, then re-decode.
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB limit
	log.Printf("load_scid: proxy responded with status %d for %s", resp.StatusCode, scid)

	if resp.StatusCode != http.StatusOK {
		// The TELA proxy now returns JSON errors, so try to extract the message.
		var errResp struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if json.Unmarshal(bodyBytes, &errResp) == nil && errResp.Error != "" {
			writeJSON(w, ctrlResp{OK: false, Error: "TELA proxy: " + errResp.Error})
			return
		}
		writeJSON(w, ctrlResp{OK: false, Error: "TELA proxy returned status " + http.StatusText(resp.StatusCode)})
		return
	}

	var res struct {
		Result struct {
			URL string `json:"url"`
		} `json:"result"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil || res.Result.URL == "" {
		log.Printf("load_scid: JSON parse failure or empty url for %s: %v — body: %s", scid, err, string(bodyBytes))
		writeJSON(w, ctrlResp{OK: false, Error: "SCID loaded but no URL returned from proxy"})
		return
	}

	if h.Sync.HasIndexer() {
		go h.Sync.IndexSCIDNow(scid)
	} else {
		log.Printf("load_scid: indexer not ready yet, skipping SCID indexing for %s", scid)
	}

	writeJSON(w, ctrlResp{OK: true, Result: map[string]any{
		"url": res.Result.URL,
	}})
}

func (h *Handlers) handleAddTELA(w http.ResponseWriter, r *http.Request) {
	scid := r.PathValue("scid")
	if scid == "" {
		writeJSON(w, ctrlResp{OK: false, Error: "missing SCID"})
		return
	}
	// Forward to the TELA proxy's add handler
	proxyURL := fmt.Sprintf("http://127.0.0.1:%d/add/%s", h.TelaPort, scid)
	resp, err := http.Get(proxyURL)
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: err.Error()})
		return
	}
	defer resp.Body.Close()
	// Forward the exact status code and body (now always JSON with ok/error fields).
	// Set Content-Type before WriteHeader so it is not silently dropped.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (h *Handlers) handleTELAProxy(w http.ResponseWriter, r *http.Request) {
	// Extract SCID from path: /tela/{scid}/...
	path := strings.TrimPrefix(r.URL.Path, "/tela/")
	scid, subPath, _ := strings.Cut(path, "/")

	if scid == "" {
		http.NotFound(w, r)
		return
	}
	if len(scid) != 64 {
		http.Error(w, "invalid SCID", 400)
		return
	}

	proxy := h.TELA.GetProxy(scid)
	if proxy == nil {
		http.Error(w, "SCID not loaded", 404)
		return
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

// ---- Logs Handler ----

func (h *Handlers) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	if h.LogService == nil {
		writeJSON(w, ctrlResp{OK: true, Result: map[string]any{
			"logs": []LogEntry{},
		}})
		return
	}

	// Optional query params: since=unixMs, level=INFO
	// `since` is an absolute Unix timestamp in milliseconds (age from now).
	sinceStr := r.URL.Query().Get("since")
	level := r.URL.Query().Get("level")

	since := time.Time{}
	if sinceStr != "" {
		if ms, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			since = time.Now().Add(-time.Duration(ms) * time.Millisecond)
		}
	}

	entries := h.LogService.GetEntries(since, level)
	writeJSON(w, ctrlResp{OK: true, Result: map[string]any{
		"logs": entries,
	}})
}

// ---- Settings Handlers ----

type AppConfig struct {
	Bookmarks struct {
		SCIDs map[string]any `json:"scids"`
		Nodes map[string]any `json:"nodes"`
	} `json:"bookmarks"`
	Settings struct {
		DefaultNode          string `json:"defaultNode"`
		AutoConnect          bool   `json:"autoConnect"`
		DirectLoad           bool   `json:"directLoad"`
		OpenDashboardOnStart *bool  `json:"openDashboardOnStart"`
		HiddenExtensions     string `json:"hiddenExtensions"`
		ShowSearchCards      *bool  `json:"showSearchCards"`
		ShowTopBar           *bool  `json:"showTopBar"`
		SearchGradient       string `json:"searchGradient"`
		CheckUpdates         bool   `json:"checkUpdates"`
		RSSFeedUrl           string `json:"rssFeedUrl"`
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
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: "marshal config: " + err.Error()})
		return
	}
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		writeJSON(w, ctrlResp{OK: false, Error: "write config: " + err.Error()})
		return
	}
	writeJSON(w, ctrlResp{OK: true})
}
