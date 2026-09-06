package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/civilware/tela"

	"hyperwolf/internal/buildinfo"
	"hyperwolf/internal/desktop"
	"hyperwolf/internal/indexer"
	"hyperwolf/internal/router"
	"hyperwolf/internal/state"
	telapkg "hyperwolf/internal/tela"
	"hyperwolf/tray"
)

//go:embed web/*
var webFS embed.FS

// Version is the HyperWolf application version.
// Single source of truth lives in internal/buildinfo.
const Version = buildinfo.Version

var (
	dashboardPort = flag.Int("dashboard-port", 18080, "dashboard HTTP server port")
	telaPort      = flag.Int("tela-port", 18081, "TELA proxy port")
	gnomonPort    = flag.Int("gnomon-api", 18082, "HyperGnomon API port")
	gnomonWSPort  = flag.Int("gnomon-ws", 40403, "Gnomon WebSocket JSON-RPC port (TELA apps use this as fallback when XSWD is unavailable)")
	logFile       = flag.String("log-file", "", "log file path (default: ~/.hyperwolf/hyperwolf.log)")
	keepDB        = flag.Bool("keep-db", false, "preserve the existing HyperGnomon database instead of wiping it on startup")
	installFlag   = flag.Bool("install", false, "install desktop entries and autostart, then exit")
	uninstallFlag = flag.Bool("uninstall", false, "remove all desktop entries and installed binary, then exit")
)

func main() {
	flag.Parse()

	if *installFlag {
		iconData, err := webFS.ReadFile("web/icons/hyperwolf-96x96.png")
		if err != nil {
			log.Fatalf("Read icon: %v", err)
		}
		exe, err := os.Executable()
		if err != nil {
			log.Fatalf("Get executable: %v", err)
		}
		if err := desktop.Install(exe, iconData); err != nil {
			log.Fatalf("Install failed: %v", err)
		}
		fmt.Println("HyperWolf installed — desktop entries created")
		return
	}

	if *uninstallFlag {
		if err := desktop.Uninstall(); err != nil {
			log.Fatalf("Uninstall failed: %v", err)
		}
		fmt.Println("HyperWolf uninstalled — desktop entries removed")
		return
	}

	appState := state.New()
	hub := router.NewHub()
	logSvc := router.NewLogService()

	// Stream log entries to dashboard clients over the WebSocket hub so the
	// Terminal Logs page updates live without polling or manual refresh.
	logCh, logUnsub := logSvc.Subscribe()
	go func() {
		defer logUnsub()
		for entries := range logCh {
			for _, entry := range entries {
				hub.Send(map[string]any{"event": "log_entry", "entry": entry})
			}
		}
	}()

	syncMgr := indexer.NewSyncManager(*gnomonPort, *gnomonWSPort, func(msg map[string]any) {
		hub.Send(msg)
	})
	syncMgr.SetPreserveDB(*keepDB)
	// Mirror passive daemon connectivity loss/recovery to the tray icon.
	syncMgr.OnHealthChange = tray.SetConnected

	if err := syncMgr.InitStorage(); err != nil {
		log.Fatalf("Failed to init storage: %v", err)
	}
	defer syncMgr.CloseStorage()

	setupLogging(syncMgr.DBDir, logSvc)

	hyperwolfDir := filepath.Dir(syncMgr.DBDir)
	if err := tela.SetShardPath(hyperwolfDir); err != nil {
		log.Printf("Warning: cannot set shard path: %v", err)
	}

	telaProxy := telapkg.NewProxyManager(*telaPort, func() string {
		return appState.GetNode()
	})

	shutdownCh := make(chan struct{}, 1)

	handlers := &router.Handlers{
		State:       appState,
		Sync:        syncMgr,
		TELA:        telaProxy,
		Hub:         hub,
		DBDir:       syncMgr.DBDir,
		TelaPort:    *telaPort,
		GnomonPort:  *gnomonPort,
		OnConnected: tray.SetConnected,
		ShutdownCh:  shutdownCh,
		LogService:  logSvc,
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *dashboardPort)
	srv := router.NewServer(addr, webFS, handlers)

	// Start HTTP server in background
	go func() {
		log.Printf("HyperWolf dashboard: http://%s/", addr)
		logSvc.Add(router.LogLevelInfo, "HyperWolf dashboard started on "+addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server: %v", err)
		}
	}()

	fmt.Printf("\n  HyperWolf started — open http://%s/ in your browser\n\n", addr)
	logSvc.Add(router.LogLevelInfo, "HyperWolf started — dashboard at http://"+addr+"/")

	// Wire tray callbacks
	startNode := func() {
		node := appState.GetNode()
		if node == "" {
			node = readDefaultNode(hyperwolfDir)
		}
		if node == "" {
			node = "http://127.0.0.1:10102"
		}
		if !strings.HasPrefix(node, "http://") {
			node = "http://" + node
		}
		appState.SetNode(node)
		telaProxy.Start()
		logSvc.Add(router.LogLevelInfo, "Starting TELA proxy")
		time.Sleep(100 * time.Millisecond)
		go syncMgr.StartSync(node)
		logSvc.Add(router.LogLevelSuccess, "Connected to node: "+node)
	}

	stopNode := func() {
		syncMgr.StopSync()
		telaProxy.Reset()
		appState.ClearNode()
		logSvc.Add(router.LogLevelWarn, "Node disconnected")
	}

	quitFn := func() {
		log.Println("Shutting down from tray...")
		shutdownCh <- struct{}{}
	}

	// Set tray icon from embedded logo
	if iconData, err := webFS.ReadFile("web/icons/hyperwolf-48x48.png"); err == nil {
		tray.SetIcon(iconData)
	}

	// Unified shutdown coordinator
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		select {
		case <-sigCh:
			log.Println("Shutting down from signal...")
			logSvc.Add(router.LogLevelWarn, "Received shutdown signal")
		case <-shutdownCh:
		}
		logSvc.Add(router.LogLevelInfo, "Stopping node synchronization")
		syncMgr.StopSync()
		telaProxy.Shutdown()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Dashboard shutdown: %v", err)
		}
		cancel()
		logSvc.Add(router.LogLevelInfo, "Shutting down...")
		tray.Stop()
	}()

	// Run tray on main goroutine (blocks until tray.Stop() is called)
	openDash := readOpenDashboardOnStart(hyperwolfDir)
	tray.Run(addr, startNode, stopNode, quitFn, openDash)
}

func readDefaultNode(hyperwolfDir string) string {
	cfgPath := filepath.Join(hyperwolfDir, "config.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return ""
	}
	var cfg struct {
		Settings struct {
			DefaultNode string `json:"defaultNode"`
		} `json:"settings"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return cfg.Settings.DefaultNode
}

// readOpenDashboardOnStart reads the openDashboardOnStart setting from config.json.
// Returns true (open by default) when the setting is absent or unset.
func readOpenDashboardOnStart(hyperwolfDir string) bool {
	cfgPath := filepath.Join(hyperwolfDir, "config.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return true
	}
	var cfg struct {
		Settings struct {
			OpenDashboardOnStart *bool `json:"openDashboardOnStart"`
		} `json:"settings"`
	}
	if json.Unmarshal(data, &cfg) != nil || cfg.Settings.OpenDashboardOnStart == nil {
		return true
	}
	return *cfg.Settings.OpenDashboardOnStart
}

// logBridge pipes every line written through the standard Go logger into the
// LogService (so the Terminal Logs page shows the app's full log stream) while
// still writing the original line to the configured log file.
type logBridge struct {
	svc *router.LogService
	w   io.Writer
}

func (b *logBridge) Write(p []byte) (int, error) {
	line := strings.TrimSpace(string(p))
	if line != "" {
		// Strip the standard Go logger prefix (e.g. "2026/02/08 01:42:22 ")
		// since the LogEntry carries its own timestamp. Use SplitN instead
		// of hardcoded character positions so this works across Go versions
		// and custom log flag configurations.
		if parts := strings.SplitN(line, " ", 3); len(parts) == 3 {
			line = parts[2]
		}
		if line != "" {
			b.svc.AddMessage(line)
		}
	}
	return b.w.Write(p)
}

func setupLogging(dbDir string, logSvc *router.LogService) {
	if *logFile == "" {
		*logFile = filepath.Join(filepath.Dir(dbDir), "hyperwolf.log")
	}
	if err := os.MkdirAll(filepath.Dir(*logFile), 0755); err != nil {
		log.Printf("Warning: cannot create log dir: %v", err)
		return
	}
	f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Warning: cannot open log file %s: %v", *logFile, err)
		return
	}
	log.SetOutput(&logBridge{svc: logSvc, w: f})
	log.Printf("HyperWolf started, logging to %s", *logFile)
}
