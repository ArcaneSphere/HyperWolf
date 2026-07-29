package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/civilware/tela"

	"hyperwolf/internal/desktop"
	"hyperwolf/internal/indexer"
	"hyperwolf/internal/router"
	"hyperwolf/internal/state"
	telapkg "hyperwolf/internal/tela"
	"hyperwolf/tray"
)

//go:embed web/*
var webFS embed.FS

var (
	dashboardPort = flag.Int("dashboard-port", 18080, "dashboard HTTP server port")
	telaPort      = flag.Int("tela-port", 18081, "TELA proxy port")
	gnomonPort    = flag.Int("gnomon-api", 18082, "HyperGnomon API port")
	gnomonWSPort  = flag.Int("gnomon-ws", 40403, "Gnomon WebSocket JSON-RPC port (TELA apps use this as fallback when XSWD is unavailable)")
	logFile       = flag.String("log-file", "", "log file path (default: ~/.hyperwolf/hyperwolf.log)")
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

	syncMgr := indexer.NewSyncManager(*gnomonPort, *gnomonWSPort, func(msg map[string]any) {
		hub.Send(msg)
	})

	if err := syncMgr.InitStorage(); err != nil {
		log.Fatalf("Failed to init storage: %v", err)
	}
	defer syncMgr.CloseStorage()

	setupLogging(syncMgr.DBDir)

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
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *dashboardPort)
	srv := router.NewServer(addr, webFS, handlers)

	// Start HTTP server in background
	go func() {
		log.Printf("HyperWolf dashboard: http://%s/", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server: %v", err)
		}
	}()

	fmt.Printf("\n  HyperWolf started — open http://%s/ in your browser\n\n", addr)

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
		time.Sleep(100 * time.Millisecond)
		go syncMgr.StartSync(node)
	}

	stopNode := func() {
		syncMgr.StopSync()
		telaProxy.Reset()
		appState.ClearNode()
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
		case <-shutdownCh:
		}
		telaProxy.Shutdown()
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

func setupLogging(dbDir string) {
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
	log.SetOutput(f)
	log.Printf("HyperWolf started, logging to %s", *logFile)
}
