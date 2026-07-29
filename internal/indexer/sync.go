package indexer

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	hgapi "github.com/hypergnomon/hypergnomon/api"
	hgindexer "github.com/hypergnomon/hypergnomon/indexer"
	hgstructures "github.com/hypergnomon/hypergnomon/structures"
)

type EventSender func(msg map[string]any)

type TelaAppInfo struct {
	SCID          string `json:"scid"`
	DURL          string `json:"durl"`
	Name          string `json:"name"`
	DescrHdr      string `json:"descrHdr"`
	IconURL       string `json:"iconURL"`
	InstallHeight int64  `json:"install_height"`
	FromAPI       bool   `json:"from_api"`
}

type SyncManager struct {
	Indexer      *hgindexer.Indexer
	APIServer    *hgapi.Server
	DBDir        string
	syncCancel   chan struct{}
	tipSyncedSent bool
	sendEvent    EventSender
	gnomonPort   int
	gnomonWSPort int
	GnomonWS     *GnomonWSServer
}

func NewSyncManager(gnomonPort, gnomonWSPort int, sendEvent EventSender) *SyncManager {
	return &SyncManager{
		gnomonPort:   gnomonPort,
		gnomonWSPort: gnomonWSPort,
		sendEvent:    sendEvent,
	}
}

func (sm *SyncManager) InitStorage() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not find home dir: %w", err)
	}
	sm.DBDir = filepath.Join(home, ".hyperwolf", "gnomondb")

	if err := os.MkdirAll(sm.DBDir, 0755); err != nil {
		return fmt.Errorf("mkdir db dir: %w", err)
	}
	log.Printf("Storage ready: %s", sm.DBDir)
	return nil
}

func (sm *SyncManager) StartSync(node string) {
	if !strings.HasPrefix(node, "http://") {
		node = "http://" + node
	}

	nodeForIndexer := strings.TrimPrefix(node, "http://")
	nodeForIndexer = strings.TrimPrefix(nodeForIndexer, "https://")

	if sm.syncCancel != nil {
		close(sm.syncCancel)
		time.Sleep(500 * time.Millisecond)
	}
	sm.syncCancel = make(chan struct{})
	cancel := sm.syncCancel

	go sm.watchDaemonHealth(node, cancel)

	targetHeight := int64(0)
	retries := 0
	for targetHeight == 0 {
		select {
		case <-cancel:
			return
		default:
		}
		targetHeight = getChainTopoHeight(node, sm.sendEvent)
		if targetHeight == 0 {
			retries++
			log.Printf("Sync: waiting for daemon at %s (attempt %d)...", node, retries)
			if retries == 2 && sm.sendEvent != nil {
				sm.sendEvent(map[string]any{
					"event": "node_unreachable",
					"node":  node,
				})
			}
			select {
			case <-cancel:
				return
			case <-time.After(3 * time.Second):
			}
		}
	}
	log.Printf("Sync: target locked at height %d", targetHeight)

	home, _ := os.UserHomeDir()
	cfg := hgindexer.Config{
		Endpoint:       nodeForIndexer,
		DBDir:          filepath.Join(home, ".hyperwolf", "gnomondb"),
		SearchFilter:   nil,
		ParallelBlocks: 32,
		BatchSize:      1000,
		PoolSize:       16,
		TurboMode:      true,
		PostScanVarsMode: "lazy",
		AdaptBatchSize:   true,
		RecentBlocks:    500,
		CodePolicy:      "none",
		FinalityDepth:   3,
	}
	log.Printf("HyperGnomon config: DBDir=%s Endpoint=%s", cfg.DBDir, cfg.Endpoint)

	activeIndexer, err := hgindexer.New(cfg)
	if err != nil {
		log.Printf("HyperGnomon indexer: %v", err)
		return
	}
	sm.Indexer = activeIndexer

	hgstructures.Logger.SetLevel(logrus.WarnLevel)

	go func() {
		lastHeight := activeIndexer.LastIndexedHeight.Load()
		if lastHeight > 0 {
			// DB has data from a previous session — skip FastSync entirely.
			// StartDaemonMode resumes from LastIndexedHeight via scanLoop.
			log.Printf("Existing index found at height %d — skipping FastSync, resuming daemon scan", lastHeight)
		} else {
			// Fresh DB — run FastSync for initial bootstrap.
			log.Println("TELA discovery: FastSync starting...")
			if err := activeIndexer.FastSync(false); err != nil {
				log.Printf("FastSync error: %v", err)
			} else {
				log.Println("TELA discovery: FastSync complete, probeTELA running in background")
				// Save the freshly-discovered app catalog so the next startup
				// has immediate data without waiting for a new sync.
				sm.saveAppCache()
			}
		}

		go activeIndexer.StartDaemonMode()
	}()

	lastHeight := activeIndexer.LastIndexedHeight.Load()
	chainHeight := activeIndexer.ChainHeight.Load()
	log.Printf("HyperGnomon started: chain=%d indexed=%d", chainHeight, lastHeight)

	{
		sm.APIServer = hgapi.NewServer(
			activeIndexer.Store,
			activeIndexer.RPCPool,
			fmt.Sprintf("127.0.0.1:%d", sm.gnomonPort),
			&activeIndexer.SafeHeight,
			nil,
			activeIndexer,
			0,
		)
		go func() {
			if err := sm.APIServer.Start(); err != nil {
				log.Printf("HyperGnomon API server exited: %v", err)
			}
		}()
		log.Printf("HyperGnomon API listening on :%d", sm.gnomonPort)
	}

	// Start Gnomon-compatible WebSocket JSON-RPC server. TELA apps in the
	// browser fall back to this when XSWD (the wallet bridge) does not
	// expose DERO.GetSC. The standard Gnomon WS port is 40403.
	// Also serves /xswd for apps that speak the XSWD wallet-bridge protocol.
	{
		wsAddr := fmt.Sprintf("127.0.0.1:%d", sm.gnomonWSPort)
		daemonURL := node // reuse the same node the indexer connects to
		sm.GnomonWS = NewGnomonWSServer(wsAddr, activeIndexer.Store, daemonURL, activeIndexer)
		go func() {
			if err := sm.GnomonWS.Start(); err != nil {
				log.Printf("Gnomon WS server exited: %v", err)
			}
		}()
		log.Printf("Gnomon WS JSON-RPC listening on %s/ws", wsAddr)
	}

	go func() {
		known := sm.KnownSCIDs()
		if len(known) > 0 {
			filtered := sm.ValidatedSCIDCount()
			if sm.sendEvent != nil {
				sm.sendEvent(map[string]any{
					"event":    "catalog_progress",
					"total":    len(known),
					"filtered": filtered,
				})
			}
		}
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-cancel:
				return
			case <-ticker.C:
				indexed := activeIndexer.LastIndexedHeight.Load()
				chainH := activeIndexer.ChainHeight.Load()
				if indexed > 0 && chainH > 0 && sm.sendEvent != nil {
					sm.sendEvent(map[string]any{
						"event":   "sync_progress",
						"indexed": indexed,
						"chain":   chainH,
					})
					if indexed >= chainH-20 && !sm.tipSyncedSent {
						sm.tipSyncedSent = true
						sm.sendEvent(map[string]any{
							"event":  "tip_synced",
							"height": indexed,
						})
					}
				}

				current := sm.KnownSCIDs()
				filtered := sm.ValidatedSCIDCount()
				if sm.sendEvent != nil {
					sm.sendEvent(map[string]any{
						"event":    "catalog_progress",
						"total":    len(current),
						"filtered": filtered,
					})
				}

				if len(current) > len(known) {
					existing := make(map[string]struct{}, len(known))
					for _, scid := range known {
						existing[scid] = struct{}{}
					}
					for _, scid := range current {
						if _, ok := existing[scid]; !ok {
							if sm.sendEvent != nil {
								sm.sendEvent(map[string]any{
									"event": "new_tela_app",
									"scid":  scid,
								})
							}
						}
					}
					log.Printf("Progressive: %d SCIDs (%d new)", len(current), len(current)-len(known))
					known = current
					// Keep the on-disk cache fresh as the indexer discovers apps.
					sm.saveAppCache()
				}
			}
		}
	}()
}

func (sm *SyncManager) StopSync() {
	if sm.syncCancel != nil {
		close(sm.syncCancel)
		sm.syncCancel = nil
	}
	if sm.GnomonWS != nil {
		sm.GnomonWS.Stop()
		sm.GnomonWS = nil
	}
	sm.StopIndexer()
}

func (sm *SyncManager) StopIndexer() {
	if sm.Indexer != nil {
		sm.Indexer.Close()
		sm.Indexer = nil
	}
}

func (sm *SyncManager) CloseStorage() {
	sm.StopIndexer()
}

// cacheFilePath returns the path to the on-disk app discovery cache.
// Lives alongside config.json in ~/.hyperwolf/app-cache.json.
func (sm *SyncManager) cacheFilePath() string {
	return filepath.Join(filepath.Dir(sm.DBDir), "app-cache.json")
}

// saveAppCache serialises the current DiscoverTelaApps() output to disk.
// Only writes when the data contains real install heights (not placeholder zeros).
func (sm *SyncManager) saveAppCache() {
	apps := sm.DiscoverTelaApps()
	if len(apps) == 0 {
		return
	}
	hasRealData := false
	for _, a := range apps {
		if a.InstallHeight > 0 {
			hasRealData = true
			break
		}
	}
	if !hasRealData {
		return
	}
	data, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		log.Printf("saveAppCache: marshal error: %v", err)
		return
	}
	if err := os.WriteFile(sm.cacheFilePath(), data, 0644); err != nil {
		log.Printf("saveAppCache: write error: %v", err)
	} else {
		log.Printf("saveAppCache: saved %d apps", len(apps))
	}
}

// loadAppCache reads a previously-saved discovery cache from disk.
// Returns nil on cache-miss or corruption (caller falls back to live indexer).
func (sm *SyncManager) loadAppCache() []TelaAppInfo {
	data, err := os.ReadFile(sm.cacheFilePath())
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("loadAppCache: read error: %v", err)
		}
		return nil
	}
	var apps []TelaAppInfo
	if err := json.Unmarshal(data, &apps); err != nil {
		log.Printf("loadAppCache: unmarshal error: %v", err)
		return nil
	}
	// Mark as from_api so the frontend treats these as live indexer results.
	for i := range apps {
		apps[i].FromAPI = true
		if apps[i].DURL == "" {
			apps[i].DURL = apps[i].SCID
		}
		if apps[i].Name == "" {
			apps[i].Name = apps[i].SCID
		}
	}
	log.Printf("loadAppCache: restored %d apps", len(apps))
	return apps
}

func (sm *SyncManager) DiscoverTelaApps() []TelaAppInfo {
	if sm.Indexer == nil {
		// Indexer not ready yet — try the on-disk cache from a previous run.
		if cached := sm.loadAppCache(); cached != nil {
			return cached
		}
		return nil
	}

	seen := make(map[string]bool)
	var apps []TelaAppInfo

	for _, class := range []string{"TELA-INDEX-1"} {
		installs, err := sm.Indexer.Store.GetClassInstalls(class, 0)
		if err != nil {
			log.Printf("DiscoverTelaApps: GetClassInstalls(%s): %v", class, err)
			continue
		}
		for _, inst := range installs {
			if seen[inst.SCID] {
				continue
			}
			seen[inst.SCID] = true
			durl := inst.SCID
			name := inst.SCID
			desc := ""
			icon := ""
			installH := inst.InstallHeight
			if meta := inst.Meta; meta != nil {
				if meta.DURL != "" {
					durl = meta.DURL
				}
				if meta.Name != "" {
					name = meta.Name
				}
				desc = meta.Desc
				icon = meta.IconURL
			}
			apps = append(apps, TelaAppInfo{
				SCID: inst.SCID, DURL: durl, Name: name,
				DescrHdr: desc, IconURL: icon,
				InstallHeight: installH, FromAPI: true,
			})
		}
	}
	classBucketCount := len(apps)

	log.Printf("DiscoverTelaApps: %d apps (%d from class bucket)", len(apps), classBucketCount)
	return apps
}

func (sm *SyncManager) ValidatedSCIDCount() int {
	if sm.Indexer == nil {
		return 0
	}
	n := 0
	for _, class := range []string{"TELA-INDEX-1"} {
		installs, err := sm.Indexer.Store.GetClassInstalls(class, 0)
		if err == nil {
			n += len(installs)
		}
	}
	return n
}

func (sm *SyncManager) KnownSCIDs() []string {
	if sm.Indexer == nil {
		return nil
	}

	seen := make(map[string]bool)
	for _, class := range []string{"TELA-INDEX-1"} {
		installs, err := sm.Indexer.Store.GetClassInstalls(class, 0)
		if err != nil {
			log.Printf("KnownSCIDs: GetClassInstalls(%s): %v", class, err)
			continue
		}
		for _, inst := range installs {
			seen[inst.SCID] = true
		}
	}
	out := make([]string, 0, len(seen))
	for scid := range seen {
		out = append(out, scid)
	}
	return out
}

func (sm *SyncManager) IndexSCIDNow(scid string) {
	scid = strings.TrimSpace(scid)
	if sm.Indexer == nil || len(scid) != 64 {
		return
	}
	if _, err := sm.Indexer.IndexSingleSCID(scid, false, false); err != nil {
		log.Printf("IndexSCIDNow: %v", err)
		return
	}
	log.Printf("IndexSCIDNow: indexed %s", scid)
}

func (sm *SyncManager) watchDaemonHealth(node string, cancel chan struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var lastKnownStatus bool = true

	for {
		select {
		case <-cancel:
			return
		case <-ticker.C:
			h := getChainTopoHeight(node, sm.sendEvent)
			currentStatus := h > 0

			if currentStatus != lastKnownStatus {
				lastKnownStatus = currentStatus
				if !currentStatus {
					log.Printf("HealthWatch: daemon unreachable at %s", node)
					if sm.sendEvent != nil {
						sm.sendEvent(map[string]any{
							"event": "node_unreachable",
							"node":  node,
						})
					}
				} else {
					log.Printf("HealthWatch: daemon recovered at %s", node)
					if sm.sendEvent != nil {
						sm.sendEvent(map[string]any{
							"event": "node_recovered",
							"node":  node,
						})
					}
				}
			}
		}
	}
}

// Package-level helper to avoid import cycle — indexer calls daemon's GetInfo.
func getChainTopoHeight(node string, sendEvent EventSender) int64 {
	di := getDaemonInfo(node)
	if di == nil {
		return 0
	}
	return di.TopoHeight
}

func getDaemonInfo(node string) *daemonInfo {
	return getDaemonInfoFromDaemon(node)
}

// Minimal daemon RPC duplicated to avoid import cycle with internal/daemon.
// Only used by health-check polling in the indexer.
type daemonInfo struct {
	TopoHeight   int64  `json:"topoheight"`
	StableHeight int64  `json:"stableheight"`
	Difficulty   int64  `json:"difficulty"`
	Version      string `json:"version"`
	Network      string `json:"network"`
	MempoolSize  int    `json:"tx_pool_size"`
}

func getDaemonInfoFromDaemon(node string) *daemonInfo {
	type jsonRPCResponse struct {
		Result map[string]any `json:"result"`
	}
	client := &http.Client{Timeout: 5 * time.Second}
	body := strings.NewReader(`{"jsonrpc":"2.0","id":"1","method":"DERO.GetInfo"}`)
	resp, err := client.Post(node+"/json_rpc", "application/json", body)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var raw jsonRPCResponse
	json.NewDecoder(resp.Body).Decode(&raw)
	if raw.Result == nil {
		return nil
	}

	info := &daemonInfo{}
	if h, ok := raw.Result["topoheight"].(float64); ok {
		info.TopoHeight = int64(h)
	}
	if h, ok := raw.Result["stableheight"].(float64); ok {
		info.StableHeight = int64(h)
	}
	if d, ok := raw.Result["difficulty"].(float64); ok {
		info.Difficulty = int64(d)
	}
	if v, ok := raw.Result["version"].(string); ok {
		info.Version = v
	}
	if n, ok := raw.Result["network"].(string); ok {
		info.Network = n
	}
	if info.Network == "" {
		if testnet, ok := raw.Result["testnet"].(bool); ok && testnet {
			info.Network = "Testnet"
		} else {
			info.Network = "Mainnet"
		}
	}
	if m, ok := raw.Result["tx_pool_size"].(float64); ok {
		info.MempoolSize = int(m)
	}

	return info
}
