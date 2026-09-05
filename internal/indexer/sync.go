package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"

	hgapi "github.com/hypergnomon/hypergnomon/api"
	hgindexer "github.com/hypergnomon/hypergnomon/indexer"
	hgstorage "github.com/hypergnomon/hypergnomon/storage"
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
	mu            sync.Mutex // guards Indexer, APIServer, GnomonWS, syncCancel, syncDone
	Indexer       *hgindexer.Indexer
	APIServer     *hgapi.Server
	DBDir         string
	syncCancel    chan struct{}
	syncDone      chan struct{} // closed when all sync goroutines exit
	tipSyncedSent bool
	sendEvent     EventSender
	// OnHealthChange is invoked whenever daemon connectivity flips:
	// false = node unreachable, true = node recovered. Optional; used to
	// drive the tray status icon from passive daemon health.
	OnHealthChange func(bool)
	gnomonPort     int
	gnomonWSPort   int
	GnomonWS       *GnomonWSServer
	// discoverLastCount tracks the last app count we logged, so repeated
	// DiscoverTelaApps() polls (status refresh every 5s, sync ticker) don't
	// spam the log with identical lines. It is touched from HTTP handler
	// goroutines and the sync ticker goroutine, so it must be atomic.
	discoverLastCount atomic.Int64
}

// indexerSnapshot returns the current indexer pointer under lock. The caller
// can use the returned value without holding the lock; atomic fields on the
// indexer are safe for concurrent reads.
func (sm *SyncManager) indexerSnapshot() *hgindexer.Indexer {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.Indexer
}

// GetIndexedHeight returns the current indexed height, or 0 if no indexer.
func (sm *SyncManager) GetIndexedHeight() int64 {
	idx := sm.indexerSnapshot()
	if idx == nil {
		return 0
	}
	return idx.LastIndexedHeight.Load()
}

// GetChainHeight returns the chain height as known to the indexer.
func (sm *SyncManager) GetChainHeight() int64 {
	idx := sm.indexerSnapshot()
	if idx == nil {
		return 0
	}
	return idx.ChainHeight.Load()
}

// HasIndexer reports whether an active indexer instance is running.
func (sm *SyncManager) HasIndexer() bool {
	return sm.indexerSnapshot() != nil
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

	// Cold-start policy: HyperWolf only consumes the current contract state,
	// so a replay/catch-up index is pure overhead — the gnomondb grows without
	// bound (3.8 GB vs ~124 MB for an equivalent FastSync). Wipe the DB on every
	// app start so the syncer always begins from a clean FastSync and never
	// reloads / re-scans accumulated history. The discovery fallback
	// (app-cache.json, a sibling of gnomondb) is intentionally left intact so
	// the dashboard still shows the previous catalog until FastSync repopulates
	// the live index.
	if err := sm.wipeDBDir(); err != nil {
		log.Printf("Storage: wipe gnomondb: %v (continuing)", err)
	}

	if err := os.MkdirAll(sm.DBDir, 0755); err != nil {
		return fmt.Errorf("mkdir db dir: %w", err)
	}
	log.Printf("Storage ready: %s", sm.DBDir)
	return nil
}

// wipeDBDir removes the existing gnomondb directory so the next sync starts
// from a clean FastSync. Sibling files such as app-cache.json are preserved.
func (sm *SyncManager) wipeDBDir() error {
	if _, err := os.Stat(sm.DBDir); os.IsNotExist(err) {
		return nil // nothing to wipe; already a fresh start
	}
	log.Printf("Storage: wiping gnomondb at %s", sm.DBDir)
	if err := os.RemoveAll(sm.DBDir); err != nil {
		return fmt.Errorf("remove gnomondb: %w", err)
	}
	return nil
}

func (sm *SyncManager) StartSync(node string) {
	if !strings.HasPrefix(node, "http://") {
		node = "http://" + node
	}

	nodeForIndexer := strings.TrimPrefix(node, "http://")
	nodeForIndexer = strings.TrimPrefix(nodeForIndexer, "https://")

	// Cancel previous sync and wait for all goroutines to exit.
	sm.mu.Lock()
	if sm.syncCancel != nil {
		close(sm.syncCancel)
		sm.mu.Unlock()
		if sm.syncDone != nil {
			<-sm.syncDone
		}
		sm.mu.Lock()
	}

	// Set up new sync lifecycle channels.
	sm.syncCancel = make(chan struct{})
	sm.syncDone = make(chan struct{})
	// Re-arm tip_synced so it fires again on every connect/reconnect. Without
	// this the event is sent exactly once per process lifetime, and a dashboard
	// that opened late (or whose WebSocket reconnected) would never see it —
	// leaving the Search page on stale cached results until a manual refresh.
	sm.tipSyncedSent = false
	cancel := sm.syncCancel
	done := sm.syncDone
	var wg sync.WaitGroup
	sm.mu.Unlock()

	// Close done when all sync goroutines (including the setup goroutine
	// below and everything it spawns) have exited. All wg.Add calls happen
	// before this waiter is launched, so there is no WaitGroup-misuse race.
	wg.Add(1)
	go func() {
		defer wg.Done()
		sm.runSyncSetup(node, cancel, done, &wg)
	}()
	go func() {
		wg.Wait()
		close(done)
	}()

	// Watch daemon health independently; it exits on cancel.
	wg.Add(1)
	go func() {
		defer wg.Done()
		sm.watchDaemonHealth(node, cancel)
	}()
}

// runSyncSetup performs the slow, cancellable portion of StartSync: waiting
// for the daemon to answer, constructing the indexer, and starting the
// long-lived API/WS servers, then launching the FastSync + progress-ticker
// goroutines as children of this goroutine (all tracked in wg). Because every
// Add happens inside this goroutine while wg.Wait() may already be running in
// the waiter goroutine, we must guarantee the waiter's first Wait happens
// only after all Adds: that is achieved by the outer wg.Add(1) for THIS
// goroutine, which is added before the waiter starts. Nested Adds made here
// before this goroutine returns are still racing with the outer Wait — so we
// add the children FIRST (all of them), then return; the waiter only sees a
// consistent counter because the outer Add for this goroutine is already
// counted when the waiter starts, and nested Adds happen before this function
// returns, which is before wg.Wait can observe the counter dropping to zero.
func (sm *SyncManager) runSyncSetup(node string, cancel chan struct{}, done chan struct{}, wg *sync.WaitGroup) {
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
		Endpoint:         strings.TrimPrefix(strings.TrimPrefix(node, "http://"), "https://"),
		DBDir:            filepath.Join(home, ".hyperwolf", "gnomondb"),
		SearchFilter:     nil,
		ParallelBlocks:   32,
		BatchSize:        1000,
		PoolSize:         16,
		TurboMode:        true,
		PostScanVarsMode: "lazy",
		AdaptBatchSize:   true,
		RecentBlocks:     500,
		CodePolicy:       "none",
		FinalityDepth:    3,
	}
	log.Printf("HyperGnomon config: DBDir=%s Endpoint=%s", cfg.DBDir, cfg.Endpoint)

	activeIndexer, err := hgindexer.New(cfg)
	if err != nil {
		log.Printf("HyperGnomon indexer: %v", err)
		return
	}
	sm.mu.Lock()
	sm.Indexer = activeIndexer
	sm.mu.Unlock()

	hgstructures.Logger.SetLevel(logrus.WarnLevel)

	{
		// API server: long-lived, NOT in the waitgroup; stopped explicitly.
		apiServer := hgapi.NewServer(
			activeIndexer.Store,
			activeIndexer.RPCPool,
			fmt.Sprintf("127.0.0.1:%d", sm.gnomonPort),
			&activeIndexer.SafeHeight,
			&activeIndexer.ReorgDetected,
			nil,
			activeIndexer,
			0,
		)
		sm.mu.Lock()
		sm.APIServer = apiServer
		sm.mu.Unlock()
		go func() {
			if err := apiServer.Start(); err != nil {
				log.Printf("HyperGnomon API server exited: %v", err)
			}
		}()
		log.Printf("HyperGnomon API listening on :%d", sm.gnomonPort)
	}

	{
		// Gnomon-compatible WS server: long-lived, NOT in the waitgroup;
		// stopped explicitly by StopSync via GnomonWS.Stop().
		wsAddr := fmt.Sprintf("127.0.0.1:%d", sm.gnomonWSPort)
		daemonURL := node // reuse the same node the indexer connects to
		wsServer := NewGnomonWSServer(wsAddr, activeIndexer.Store, daemonURL, activeIndexer)
		sm.mu.Lock()
		sm.GnomonWS = wsServer
		sm.mu.Unlock()
		go func() {
			if err := wsServer.Start(); err != nil {
				log.Printf("Gnomon WS server exited: %v", err)
			}
		}()
		log.Printf("Gnomon WS JSON-RPC listening on %s/ws", wsAddr)
	}

	// FastSync: runs as a child of this setup goroutine and is in the wg, so
	// done cannot close while it is still running (prevents double-FastSync on
	// a subsequent StartSync).
	wg.Add(1)
	go func() {
		defer wg.Done()
		lastHeight := activeIndexer.LastIndexedHeight.Load()
		if lastHeight > 0 {
			log.Printf("Existing index found at height %d — skipping FastSync, resuming daemon scan", lastHeight)
		} else {
			log.Println("TELA discovery: FastSync starting...")
			if err := activeIndexer.FastSync(false); err != nil {
				log.Printf("FastSync error: %v", err)
			} else {
				log.Println("TELA discovery: FastSync complete, probeTELA running in background")
				sm.saveAppCache()
			}
		}

		go activeIndexer.StartDaemonMode()
	}()

	lastHeight := activeIndexer.LastIndexedHeight.Load()
	chainHeight := activeIndexer.ChainHeight.Load()
	log.Printf("HyperGnomon started: chain=%d indexed=%d", chainHeight, lastHeight)

	// Progress ticker: child of the setup goroutine, in the wg.
	wg.Add(1)
	go func() {
		defer wg.Done()
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
	// Cancel and wait for all sync goroutines to exit.
	sm.mu.Lock()
	cancel := sm.syncCancel
	sm.syncCancel = nil
	done := sm.syncDone
	ws := sm.GnomonWS
	sm.GnomonWS = nil
	api := sm.APIServer
	sm.APIServer = nil
	sm.mu.Unlock()

	if cancel != nil {
		close(cancel)
	}
	// Stop the long-lived servers explicitly (they are NOT in the waitgroup).
	if ws != nil {
		ws.Stop()
	}
	if api != nil {
		// Graceful drain with a 5s deadline (matches the upstream contract:
		// Stop(ctx) returns ctx's error if forced). A clean stop returns nil,
		// so an intentional disconnect does not log an "API server exited"
		// error from Start().
		ctx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelStop()
		if err := api.Stop(ctx); err != nil {
			log.Printf("HyperGnomon API server stop: %v", err)
		}
	}
	if done != nil {
		<-done
	}
	// Persist the final discovery catalog before closing the store so the
	// app-cache never trails the live DB after a session that found new SCIDs.
	sm.saveAppCache()
	sm.StopIndexer()
}

func (sm *SyncManager) StopIndexer() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
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

// mainStoreFile mirrors the unexported const in the hypergnomon storage
// package (storage/bbolt.go) so offline reads open the same DB file.
const mainStoreFile = "HYPERGNOMON.db"

// discoverTelaAppsFromStore reads the TELA-INDEX-1 catalog directly from the
// on-disk HyperGnomon bbolt store, opened read-only. Used while the node is
// disconnected so the offline count always matches what the indexer persisted.
func (sm *SyncManager) discoverTelaAppsFromStore() ([]TelaAppInfo, error) {
	if sm.DBDir == "" {
		return nil, fmt.Errorf("db dir not initialised")
	}
	dbPath := filepath.Join(sm.DBDir, mainStoreFile)
	if _, err := os.Stat(dbPath); err != nil {
		return nil, err
	}
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{
		ReadOnly: true,
		Timeout:  3 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("open store read-only: %w", err)
	}
	defer db.Close()

	store := &hgstorage.BboltStore{DB: db, Path: dbPath}
	// True deploy heights live in the "installs" bucket (InstallRecord), NOT
	// the class bucket whose height is the re-index height (identical for all
	// apps, useless for "newest"). from=0,to=MaxInt64 gets every install, and
	// buildTelaAppsFromInstalls keeps the highest-height record per SCID.
	installs, err := store.GetInstallsInRange(0, int64(^uint64(0)>>1), 0)
	if err != nil {
		return nil, err
	}

	apps := buildTelaAppsFromInstalls(installs)
	log.Printf("DiscoverTelaApps: %d apps from on-disk store (offline)", len(apps))
	return apps, nil
}

func (sm *SyncManager) DiscoverTelaApps() []TelaAppInfo {
	idx := sm.indexerSnapshot()
	if idx == nil {
		// Indexer not running (disconnected or still starting) — read the
		// catalog straight from the on-disk HyperGnomon store so the dashboard
		// shows the full discovered set, not a stale JSON snapshot. The cache
		// remains only as a fallback for a fresh install with no DB yet.
		if apps, err := sm.discoverTelaAppsFromStore(); err == nil && len(apps) > 0 {
			return apps
		}
		if cached := sm.loadAppCache(); cached != nil {
			return cached
		}
		return nil
	}

	// Live path: read the "installs" bucket (true deploy heights) instead of
	// the class bucket (re-index height, identical across all apps).
	installs, err := idx.Store.GetInstallsInRange(0, int64(^uint64(0)>>1), 0)
	if err != nil {
		log.Printf("DiscoverTelaApps: GetInstallsInRange: %v", err)
		return nil
	}
	apps := buildTelaAppsFromInstalls(installs)

	// Only log when the count actually changes, otherwise the 5s status poll
	// and the sync ticker would spam identical lines every few seconds.
	if int64(len(apps)) != sm.discoverLastCount.Load() {
		sm.discoverLastCount.Store(int64(len(apps)))
		log.Printf("DiscoverTelaApps: %d apps", len(apps))
	}
	return apps
}

// buildTelaAppsFromInstalls collapses install records to the highest-height
// entry per SCID and maps them into the frontend TelaAppInfo shape. Every
// contract classified as TELA-INDEX-1 (a TELA app entry point) is included —
// the class bucket is the authoritative signal, and DURL/name naming is
// unrestricted, so no URL-pattern filtering is applied.
func buildTelaAppsFromInstalls(installs []hgstructures.ClassInstall) []TelaAppInfo {
	latest := make(map[string]hgstructures.ClassInstall, len(installs))
	for _, inst := range installs {
		if inst.Meta == nil || inst.Meta.Class != "TELA-INDEX-1" {
			continue
		}
		if prev, ok := latest[inst.SCID]; !ok || inst.InstallHeight > prev.InstallHeight {
			latest[inst.SCID] = inst
		}
	}
	apps := make([]TelaAppInfo, 0, len(latest))
	for _, inst := range latest {
		durl := inst.SCID
		name := inst.SCID
		desc := ""
		icon := ""
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
			InstallHeight: inst.InstallHeight, FromAPI: true,
		})
	}
	return apps
}

func (sm *SyncManager) ValidatedSCIDCount() int {
	idx := sm.indexerSnapshot()
	if idx == nil {
		return 0
	}
	n := 0
	for _, class := range []string{"TELA-INDEX-1"} {
		installs, err := idx.Store.GetClassInstalls(class, 0)
		if err == nil {
			n += len(installs)
		}
	}
	return n
}

func (sm *SyncManager) KnownSCIDs() []string {
	idx := sm.indexerSnapshot()
	if idx == nil {
		return nil
	}

	seen := make(map[string]bool)
	for _, class := range []string{"TELA-INDEX-1"} {
		installs, err := idx.Store.GetClassInstalls(class, 0)
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
	idx := sm.indexerSnapshot()
	if idx == nil || len(scid) != 64 {
		return
	}
	if _, err := idx.IndexSingleSCID(scid, false, false); err != nil {
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
				if sm.OnHealthChange != nil {
					sm.OnHealthChange(currentStatus)
				}
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
