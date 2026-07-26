# HyperWolf — Technical Documentation

> **Version:** 1.0.0
> **Language:** Go 1.26 | Vanilla JavaScript (ES2022)
> **License:** See LICENSE

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Architecture](#2-architecture)
3. [Directory Layout](#3-directory-layout)
4. [Go Modules & Packages](#4-go-modules--packages)
   - [main (main.go)](#5-main-maingo)
   - [internal/state](#6-internalstate)
   - [internal/daemon](#7-internaldaemon)
   - [internal/indexer](#8-internalindexer)
   - [internal/router](#9-internalrouter)
   - [internal/tela](#10-internaltela)
   - [internal/desktop](#11-internaldesktop)
   - [tray](#12-tray)
5. [Web Frontend Modules](#13-web-frontend-modules)
   - [dashboard.js](#14-dashboardjs)
   - [search.js](#15-searchjs)
   - [tray-popup.html (inline script)](#16-tray-popuphtml-inline-script)
6. [Data Flow](#17-data-flow)
7. [API Reference](#18-api-reference)
8. [Configuration](#19-configuration)
9. [Build & Cross-Compilation](#20-build--cross-compilation)
10. [Usage Examples](#21-usage-examples)

---

## 1. Project Overview

**HyperWolf** is a standalone desktop application for browsing **TELA** decentralized applications and websites on the **DERO** blockchain. It bundles three core capabilities into a single binary:

| Capability | Technology |
|---|---|
| **TELA Server** | [Azylem Tela](https://github.com/Azylem/tela) — serves on-chain HTML/CSS/JS as real web pages |
| **Blockchain Indexer** | [HyperGnomon](https://github.com/Dirtybird99/HyperGnomon) — discovers and indexes TELA smart contracts |
| **System Tray** | [gogpu/systray](https://github.com/gogpu/systray) — zero-CGO cross-platform tray icon with controls |

Key design decisions:

- **Single binary, no extensions** — no browser extension or external dependencies required.
- **WebSocket push** — the dashboard receives real-time sync progress, discovery events, and health alerts via `gorilla/websocket`.
- **Reverse-proxy TELA apps** — loaded SCIDs are served as first-class web pages through per-SCID reverse proxies, making on-chain dApps behave like normal websites.
- **Embedded frontend** — all HTML/JS/CSS is embedded into the binary via `//go:embed`.

---

## 2. Architecture

```
┌──────────────────────────────────────────────────────────┐
│                       main.go                            │
│  Parses flags → starts services → runs tray on main      │
└──────┬──────────────┬───────────────┬────────────────────┘
       │              │               │
       ▼              ▼               ▼
┌─────────────┐ ┌───────────┐ ┌──────────────────────────┐
│   state/    │ │  router/  │ │         tray/             │
│  AppState   │ │  Hub+HTTP │ │  SystemTray + Menu        │
│  (sync.RWM) │ │  Server   │ │  (blocks main goroutine)  │
└──────┬──────┘ └─────┬─────┘ └──────────────────────────┘
       │              │
       │         ┌────┴────┬──────────────┐
       ▼         ▼         ▼              ▼
┌───────────┐ ┌───────┐ ┌──────┐ ┌──────────────┐
│  indexer/ │ │daemon/│ │tela/ │ │  desktop/    │
│  SyncMgr  │ │Client │ │Proxy │ │ Install/     │
│ HyperGnom │ │ RPC   │ │Mux   │ │ Uninstall    │
└───────────┘ └───────┘ └──────┘ └──────────────┘
```

### Lifecycle

1. `main()` parses CLI flags, checks `--install` / `--uninstall`.
2. Creates `state.AppState`, `router.Hub`, `indexer.SyncManager`, `tela.ProxyManager`.
3. Starts the HTTP dashboard server on `:18080`.
4. Loads embedded icon, wires tray callbacks (`startNode`, `stopNode`, `quitFn`).
5. `tray.Run()` blocks the main goroutine until quit.
6. On quit signal: shuts down TELA proxy, stops tray, exits.

---

## 3. Directory Layout

```
HyperWolf/
├── main.go                  # Entry point
├── go.mod / go.sum          # Go module definition
├── internal/
│   ├── state/state.go       # Thread-safe application state
│   ├── daemon/client.go     # DERO daemon JSON-RPC client
│   ├── indexer/
│   │   ├── sync.go          # HyperGnomon sync manager
│   │   └── catalog.go       # Bundled TELA SCID registry
│   ├── router/
│   │   ├── hub.go           # WebSocket broadcast hub
│   │   └── router.go        # HTTP server + all API handlers
│   ├── tela/proxy.go        # TELA reverse-proxy manager
│   └── desktop/install.go   # Cross-platform desktop integration
├── tray/
│   └── tray.go              # System tray (systray) implementation
├── web/
│   ├── index.html           # Dashboard SPA shell
│   ├── dashboard.js         # Core UI logic, WS, bookmarks, settings
│   ├── search.js            # TELA app search (Fuse.js + HyperGnomon)
│   ├── style.css            # Dashboard styles (dark/light themes)
│   ├── fuse.min.js          # Fuse.js fuzzy-search library (vendored)
│   ├── tray-popup.html      # Tray click popup (inline JS)
│   └── icons/               # App icons (48×48, 96×96 PNG)
├── datashards/              # Reserved for DocShard data (empty)
├── assets/                  # Project assets
├── hyperwolf                # Pre-built binary (if present)
├── README.md
├── LICENSE
└── DOCS.md                  # This file
```

---

## 4. Go Modules & Packages

### Module: `hyperwolf`

Defined in `go.mod`. Key dependencies:

| Dependency | Purpose |
|---|---|
| `github.com/Azylem/tela` | TELA server — serves on-chain apps locally |
| `github.com/Dirtybird99/HyperGnomon` | Blockchain indexer for SCID discovery |
| `github.com/gogpu/systray` | Zero-CGO system tray |
| `github.com/gorilla/websocket` | WebSocket server for dashboard push |
| `github.com/sirupsen/logrus` | Structured logging (used by HyperGnomon) |

---

## 5. main (main.go)

**File:** `main.go`

### Purpose

Application entry point. Parses CLI flags, initializes all subsystems, wires callbacks, and runs the system tray on the main goroutine.

### CLI Flags

| Flag | Default | Description |
|---|---|---|
| `--dashboard-port` | `18080` | HTTP dashboard server port |
| `--tela-port` | `18081` | TELA proxy listen port |
| `--gnomon-api` | `18082` | HyperGnomon API port |
| `--log-file` | `~/.hyperwolf/hyperwolf.log` | Log file path |
| `--install` | `false` | Install desktop entries + autostart, then exit |
| `--uninstall` | `false` | Remove desktop entries and binary, then exit |

### Key Functions

```go
func main()
```

Entry point. Handles install/uninstall mode or starts the full application.

```go
func readDefaultNode(hyperwolfDir string) string
```

Reads `config.json` from the data directory to extract the `defaultNode` setting.

```go
func setupLogging(dbDir string)
```

Redirects `log` output to the configured log file, creating directories as needed.

### Lifecycle

```
flag.Parse()
  ├─ --install  → desktop.Install() → return
  ├─ --uninstall → desktop.Uninstall() → return
  └─ else:
       state.New()
       router.NewHub()
       indexer.NewSyncManager()
       syncMgr.InitStorage()
       tela.NewProxyManager()
       router.NewServer() → go srv.ListenAndServe()
       tray.Run()         → blocks until quit
       telaProxy.Shutdown()
       tray.Stop()
```

---

## 6. internal/state

**Package:** `hyperwolf/internal/state`
**File:** `internal/state/state.go`

### Purpose

Thread-safe application state holder. Stores the currently connected DERO node address and connection timestamp. Used by multiple goroutines (HTTP handlers, tray, sync manager).

### Types

#### `AppState`

```go
type AppState struct {
    mu          sync.RWMutex
    Node        string
    ConnectedAt time.Time
}
```

Concurrent-safe container for the active node connection state.

### Functions

```go
func New() *AppState
```

**@returns** `*AppState` — A new, disconnected `AppState`.

```go
func (s *AppState) SetNode(node string)
```

Sets the active node address and records the connection timestamp.

| Param | Description |
|---|---|
| `node` | DERO node address (e.g. `"127.0.0.1:10102"`) |

```go
func (s *AppState) ClearNode()
```

Clears the node address and resets the connection timestamp.

```go
func (s *AppState) GetNode() string
```

**@returns** `string` — The current node address, or `""` if disconnected.

```go
func (s *AppState) GetConnectedAt() time.Time
```

**@returns** `time.Time` — When the current connection was established, or zero time if disconnected.

```go
func (s *AppState) IsConnected() bool
```

**@returns** `bool` — `true` if a node address is set.

### Thread Safety

All methods are safe for concurrent use. `SetNode`/`ClearNode` acquire a write lock; `Get*` methods acquire a read lock.

---

## 7. internal/daemon

**Package:** `hyperwolf/internal/daemon`
**File:** `internal/daemon/client.go`

### Purpose

Lightweight DERO daemon JSON-RPC client. Provides functions to query chain info and fetch smart contract variables. Used by the dashboard status endpoint and the indexer fallback path.

### Types

#### `Info`

```go
type Info struct {
    TopoHeight   int64  `json:"topoheight"`
    StableHeight int64  `json:"stableheight"`
    Difficulty   int64  `json:"difficulty"`
    Version      string `json:"version"`
    Network      string `json:"network"`
    MempoolSize  int    `json:"tx_pool_size"`
}
```

Parsed response from `DERO.GetInfo`.

#### `SCIDVarData`

```go
type SCIDVarData struct {
    SCID          string `json:"scid"`
    DURL          string `json:"dURL"`
    NameHdr       string `json:"nameHdr"`
    DescrHdr      string `json:"descrHdr"`
    IconURL       string `json:"iconURL"`
    Likes         int    `json:"likes"`
    Dislikes      int    `json:"dislikes"`
    Average       int    `json:"average"`
    CreatedHeight int64  `json:"createdHeight"`
}
```

Aggregated metadata extracted from a TELA INDEX smart contract's stored variables.

### Functions

```go
func GetInfo(node string) *Info
```

Queries the DERO daemon at the given node address for chain information.

| Param | Description |
|---|---|
| `node` | Full URL of the DERO daemon (e.g. `"http://127.0.0.1:10102"`) |

**@returns** `*Info` — Parsed chain info, or `nil` on error.
**@throws** Logs errors to stdout on RPC failure.

```go
func FetchSCIDVariables(node string, scids []string) []SCIDVarData
```

Fetches and parses smart contract variables for multiple SCIDs concurrently. Extracts TELA metadata (dURL, name, icon, ratings) from the on-chain `stringkeys`/`uint64keys`.

| Param | Description |
|---|---|
| `node` | Full URL of the DERO daemon |
| `scids` | Slice of 64-character hex SCIDs to query |

**@returns** `[]SCIDVarData` — Parsed metadata for each successfully queried SCID.

### Implementation Notes

- `GetInfo` uses a 5-second HTTP timeout.
- `FetchSCIDVariables` runs 8 concurrent worker goroutines with a 15-second timeout per request.
- Rating extraction handles both aggregated (`likes`/`dislikes` keys) and per-user (`dero1..._<score>_<height>`) formats.
- Failed SCIDs are logged but do not cause the entire batch to fail.

---

## 8. internal/indexer

**Package:** `hyperwolf/internal/indexer`
**Files:** `sync.go`, `catalog.go`

### Purpose

Manages the HyperGnomon blockchain indexer lifecycle: initialization, background sync, SCID discovery, catalog progress events, and daemon health monitoring. Also contains the bundled registry of known TELA SCIDs.

### Types

#### `SyncManager`

```go
type SyncManager struct {
    Indexer      *hgindexer.Indexer
    APIServer    *hgapi.Server
    DBDir        string
    syncCancel   chan struct{}
    tipSyncedSent bool
    sendEvent    EventSender
    gnomonPort   int
}
```

Central coordinator for blockchain indexing. Owns the HyperGnomon indexer instance, its API server, and sync lifecycle.

#### `EventSender`

```go
type EventSender func(msg map[string]any)
```

Callback type for pushing real-time events (sync progress, discovery, health alerts) to connected WebSocket clients.

#### `TelaAppInfo`

```go
type TelaAppInfo struct {
    SCID          string `json:"scid"`
    DURL          string `json:"durl"`
    Name          string `json:"name"`
    DescrHdr      string `json:"descrHdr"`
    IconURL       string `json:"iconURL"`
    InstallHeight int64  `json:"install_height"`
    FromAPI       bool   `json:"from_api"`
}
```

Discovered TELA app metadata returned by the discovery API.

### Functions

```go
func NewSyncManager(gnomonPort int, sendEvent EventSender) *SyncManager
```

| Param | Description |
|---|---|
| `gnomonPort` | Port for the HyperGnomon API server |
| `sendEvent` | Callback for broadcasting events to dashboard clients |

**@returns** `*SyncManager` — Uninitialized manager (call `InitStorage` then `StartSync`).

```go
func (sm *SyncManager) InitStorage() error
```

Creates the `~/.hyperwolf/gnomondb` directory.

**@returns** `error` — Non-nil if directory creation fails.

```go
func (sm *SyncManager) StartSync(node string)
```

Starts the full indexing pipeline:
1. Verifies daemon connectivity (retries until reachable).
2. Creates a HyperGnomon indexer with turbo mode.
3. Preloads bundled TELA SCIDs in parallel.
4. Runs FastSync, then starts daemon mode.
5. Starts the HyperGnomon API server.
6. Begins a 2-second ticker broadcasting `sync_progress`, `catalog_progress`, and `tip_synced` events.
7. Spawns a health watcher that pings the daemon every 10 seconds.

| Param | Description |
|---|---|
| `node` | DERO daemon address (with or without `http://` prefix) |

```go
func (sm *SyncManager) StopSync()
```

Stops all sync activity: cancels background goroutines, closes the indexer.

```go
func (sm *SyncManager) StopIndexer()
```

Closes the HyperGnomon indexer without stopping other sync infrastructure.

```go
func (sm *SyncManager) CloseStorage()
```

Alias for `StopIndexer()` — called on application shutdown via defer.

```go
func (sm *SyncManager) DiscoverTelaApps() []TelaAppInfo
```

Returns all known TELA apps: those discovered by HyperGnomon's `TELA-INDEX-1` class bucket, merged with the bundled `BundledTelaSCIDs` registry.

**@returns** `[]TelaAppInfo` — Deduplicated list of TELA applications.

```go
func (sm *SyncManager) ValidatedSCIDCount() int
```

**@returns** `int` — Number of SCIDs confirmed as `TELA-INDEX-1` by the indexer.

```go
func (sm *SyncManager) KnownSCIDs() []string
```

**@returns** `[]string` — Union of indexer-discovered and bundled SCIDs.

```go
func (sm *SyncManager) IndexSCIDNow(scid string)
```

On-demand indexes a single SCID. Used when the user loads a SCID that hasn't been indexed yet.

### Constants (`catalog.go`)

```go
var BundledTelaSCIDs []string
```

A curated list of ~88 known TELA INDEX SCIDs, preloaded on every sync to ensure fast initial catalog availability.

### Events Emitted

| Event | Payload | Trigger |
|---|---|---|
| `sync_progress` | `{indexed, chain}` | Every 2s during sync |
| `tip_synced` | `{height}` | Once when indexed height reaches chain tip |
| `catalog_progress` | `{total, filtered}` | Every 2s, after discovery counts update |
| `new_tela_app` | `{scid}` | When a new SCID appears in the catalog |
| `node_unreachable` | `{node}` | Daemon health check fails (after 2 retries) |
| `node_recovered` | `{node}` | Daemon comes back after being unreachable |

---

## 9. internal/router

**Package:** `hyperwolf/internal/router`
**Files:** `hub.go`, `router.go`

### Purpose

Implements the HTTP server, WebSocket hub, and all API endpoint handlers for the dashboard. Acts as the central coordination layer between the frontend, state, indexer, TELA proxy, and daemon.

### Types

#### `Hub`

```go
type Hub struct {
    mu      sync.Mutex
    clients map[*websocket.Conn]struct{}
}
```

WebSocket broadcast hub. Maintains a set of connected dashboard clients and broadcasts messages to all of them.

#### `Handlers`

```go
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
```

Dependency-injected handler group. All HTTP handler methods are methods on this struct.

### Hub Methods

```go
func NewHub() *Hub
```

**@returns** `*Hub` — New hub with empty client set.

```go
func (h *Hub) Add(c *websocket.Conn)
```

Registers a new WebSocket client.

```go
func (h *Hub) Remove(c *websocket.Conn)
```

Removes and closes a WebSocket client connection.

```go
func (h *Hub) Send(msg map[string]any)
```

Marshals `msg` as JSON and broadcasts to all connected clients. Silently removes clients that fail to receive.

```go
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request)
```

HTTP handler that upgrades to WebSocket, adds the client, and blocks on reads until disconnect.

### Server Constructor

```go
func NewServer(addr string, webFS fs.FS, h *Handlers) *http.Server
```

Creates the HTTP server with all route registrations.

| Param | Description |
|---|---|
| `addr` | Listen address (e.g. `"127.0.0.1:18080"`) |
| `webFS` | Embedded filesystem (`//go:embed web/*`) |
| `h` | Handler dependencies |

**@returns** `*http.Server` — Ready to `ListenAndServe`.

### API Endpoints

#### Control (POST)

| Endpoint | Description |
|---|---|
| `POST /api/set_node` | Connect to a DERO node. Body: `{node: string}` |
| `POST /api/disconnect_node` | Disconnect from current node |
| `POST /api/server_status` | Full status report (heights, services, daemon info) |
| `POST /api/load_scid` | Load a SCID via TELA proxy. Body: `{scid: string}` |
| `POST /api/settings` | Save app settings. Body: `AppConfig` JSON |
| `POST /api/tela/vars` | Fetch SCID variables from daemon. Body: `{scids: []string}` |
| `POST /api/shutdown` | Graceful application shutdown |

#### Query (GET)

| Endpoint | Description |
|---|---|
| `GET /api/config` | Returns `{gnomon_api_port, tela_port}` |
| `GET /api/tela/discover` | Returns all discovered TELA apps |
| `GET /api/scids` | Returns list of loaded (proxied) SCIDs |
| `GET /api/probe_xswd` | Probes XSWD at `127.0.0.1:44326` |
| `GET /api/settings` | Returns saved app settings |
| `GET /ws` | WebSocket upgrade endpoint |

#### TELA Routes

| Endpoint | Description |
|---|---|
| `GET /add/{scid}` | Load and register a SCID with the TELA proxy |
| `GET /tela/{scid}/...` | Reverse-proxy to a loaded TELA app |
| `GET /gnomon/...` | Reverse-proxied HyperGnomon API |

#### Static Files

| Path | Description |
|---|---|
| `GET /` | Serves `web/index.html` and all static assets |

### Response Format

All API endpoints return:

```json
{
  "ok": true,
  "result": { ... },
  "error": "optional error message"
}
```

---

## 10. internal/tela

**Package:** `hyperwolf/internal/tela`
**File:** `internal/tela/proxy.go`

### Purpose

Manages TELA application reverse proxies. Each loaded SCID gets its own `httputil.ReverseProxy` pointing at a local file server where the TELA app's files are served. Handles both standard and DocShard-based (fragmented) apps.

### Types

#### `ProxyManager`

```go
type ProxyManager struct {
    mu       sync.RWMutex
    proxies  map[string]*httputil.ReverseProxy
    baseURLs map[string]string
    entries  map[string]string
    sharded  map[string]bool
    port     int
    once     sync.Once
    nodeFn   func() string
}
```

Manages per-SCID reverse proxies and the underlying TELA HTTP server.

### Functions

```go
func NewProxyManager(port int, nodeFn func() string) *ProxyManager
```

| Param | Description |
|---|---|
| `port` | Port for the TELA proxy HTTP server |
| `nodeFn` | Callback returning the current DERO node address |

**@returns** `*ProxyManager` — Uninitialized manager (call `Start` to begin listening).

```go
func (pm *ProxyManager) Start()
```

Starts the TELA proxy HTTP server (once). Registers `/add/` and `/tela/` routes. Uses `sync.Once` to ensure single initialization.

```go
func (pm *ProxyManager) GetProxy(scid string) *httputil.ReverseProxy
```

**@returns** `*httputil.ReverseProxy` — The proxy for the given SCID, or `nil` if not loaded.

```go
func (pm *ProxyManager) GetProxiedSCIDs() []string
```

**@returns** `[]string` — List of all currently loaded (proxied) SCIDs.

```go
func (pm *ProxyManager) Shutdown()
```

Shuts down the TELA server via `tela.ShutdownTELA()`.

```go
func (pm *ProxyManager) Reset()
```

Clears all proxies, base URLs, and shard state. Used on disconnect.

### SCID Loading Flow

1. `GET /add/{scid}` → `handleAddSCID`
2. Checks if SCID is already loaded (returns cached URL if so).
3. Calls `tela.GetINDEXInfo` to check if it's a DocShard app.
4. If **sharded**: calls `downloadAndReconstructShards` — downloads all DOC fragments, reassembles them, starts a temporary file server.
5. If **standard**: calls `tela.ServeTELA` — extracts and serves from a local clone directory.
6. Creates a `httputil.ReverseProxy` targeting the local file server.
7. Returns `{ok: true, result: {scid, url}}` for the dashboard to open.

---

## 11. internal/desktop

**Package:** `hyperwolf/internal/desktop`
**File:** `internal/desktop/install.go`

### Purpose

Cross-platform desktop integration: creates launcher entries, icons, and autostart configurations. Supports Linux (XDG), macOS (.app bundle + LaunchAgent), and Windows (Start Menu + Registry).

### Functions

```go
func Install(exePath string, iconData []byte) error
```

Installs HyperWolf as a desktop application on the current platform.

| Param | Description |
|---|---|
| `exePath` | Absolute path to the running binary |
| `iconData` | Raw PNG icon data (96×96) |

**@returns** `error` — Non-nil on platform-specific failure.

```go
func Uninstall() error
```

Removes all installed desktop entries and the copied binary.

**@returns** `error` — Non-nil on platform-specific failure.

### Platform Behavior

| Platform | Install | Uninstall |
|---|---|---|
| **Linux** | Copies binary to `~/.local/bin/`, creates `.desktop` file + autostart entry, installs icon | Removes all created files |
| **macOS** | Creates `~/Applications/HyperWolf.app` bundle, installs LaunchAgent | Removes app bundle + LaunchAgent |
| **Windows** | Copies to `%LOCALAPPDATA%/HyperWolf/`, creates Start Menu shortcut via PowerShell, sets registry autostart | Removes registry key, shortcut, and install directory |

---

## 12. tray

**Package:** `hyperwolf/tray`
**File:** `tray/tray.go`

### Purpose

System tray integration. Displays an icon with a connection status LED (green = connected, red = disconnected). Provides a context menu for node control and a click action that opens the tray popup HTML page.

### Types

The package uses package-level state (not exported types) since it manages a single global tray instance.

### Exported Functions

```go
func SetIcon(png []byte)
```

Sets the tray icon from raw PNG data. Must be called before `Run`.

| Param | Description |
|---|---|
| `png` | Raw PNG bytes (typically 48×48) |

```go
func Run(dashboardAddr string, startNode, stopNode, quit func())
```

Initializes and runs the system tray. **Blocks the calling goroutine** until `Stop()` is called.

| Param | Description |
|---|---|
| `dashboardAddr` | Dashboard listen address (e.g. `"127.0.0.1:18080"`) |
| `startNode` | Callback when "Start Node" is clicked |
| `stopNode` | Callback when "Stop Node" is clicked |
| `quit` | Callback when "Quit" is clicked |

```go
func SetConnected(connected bool)
```

Updates the tray icon LED color and rebuilds the menu to reflect connection state. Safe to call from any goroutine.

```go
func Stop()
```

Removes the tray icon and signals `Run` to return.

### Icon Rendering

The tray uses a layered icon system:

1. **Base icon** — loaded from embedded PNG or generated as a blue 32×32 placeholder.
2. **Status LED** — a 3px-radius circle drawn at the bottom-right corner:
   - **Green** (`#00cc00`) when connected
   - **Red** (`#cc0000`) when disconnected

### Menu Items

| Item | Action |
|---|---|
| **Open Dashboard** | Opens `http://{addr}/` in the default browser |
| **Start Node / Stop Node** | Toggles node connection (label changes based on state) |
| **Quit** | Calls the quit callback |

### Click Behavior

Clicking the tray icon opens `http://{addr}/tray-popup.html` in the default browser — a lightweight popup with status cards and live data polling.

---

## 13. Web Frontend Modules

### 14. dashboard.js

**File:** `web/dashboard.js`
**Type:** IIFE (immediately-invoked function expression), global scope
**Dependencies:** None (vanilla JS)

#### Purpose

Core dashboard logic. Manages the SPA navigation, WebSocket connection, node connection/disconnection, SCID loading, bookmarks, settings persistence, sync progress visualization, and toast notifications.

#### Module-Level State

```javascript
let bookmarks = { scids: {}, nodes: {} };
let settings = { defaultNode: "", autoConnect: true, directLoad: true, hiddenExtensions: "" };
let appConfig = { gnomon_api_port: 18082, tela_port: 18081 };
let wasConnected = false;
let connectTime = null;
let syncStartTime = null;
let lastChainHeight = 0;
let lastBlockTime = null;
```

#### Key Functions

```javascript
/**
 * Sends a POST request to a dashboard API endpoint.
 * @param {string} method - API endpoint name (e.g. "set_node", "server_status")
 * @param {Object} [params={}] - JSON body parameters
 * @returns {Promise<Object>} Parsed JSON response with {ok, result, error}
 */
async function send(method, params = {})
```

```javascript
/**
 * Renders the sidebar status indicators for Node, TELA, Gnomon, and XSWD.
 * Polls server_status every 5 seconds.
 * @returns {Promise<void>}
 */
async function updateStatusIndicators()
```

```javascript
/**
 * Updates the sync progress bar, percentage label, and ETA.
 * @param {number} indexed - Current indexed height
 * @param {number} chain - Current chain height
 * @returns {void}
 */
function updateSyncProgress(indexed, chain)
```

```javascript
/**
 * Marks the chain as synced and fills the progress bar to 100%.
 * @returns {void}
 */
function markTipSynced()
```

```javascript
/**
 * Connects to the WebSocket endpoint and handles incoming events.
 * Auto-reconnects after 3 seconds on disconnect.
 * @returns {void}
 */
function connectWebSocket()
```

```javascript
/**
 * Dispatches WebSocket events to the DOM and handles known event types.
 * @param {Object} msg - Parsed JSON message from the server
 * @returns {void}
 */
function handleEvent(msg)
```

```javascript
/**
 * Displays a toast notification in the status area.
 * @param {"connected"|"error"|"warning"|"pending"} state - Toast type
 * @param {string} message - Display text
 * @returns {HTMLElement|undefined} The toast DOM element
 */
function pushToast(state, message)
```

```javascript
/**
 * Removes a toast with a fade-out animation.
 * @param {HTMLElement} toast - The toast element to dismiss
 * @returns {void}
 */
function dismissToast(toast)
```

```javascript
/**
 * Updates the connect/disconnect button and input state.
 * @param {boolean} connected - Whether currently connected
 * @param {string} [node] - The connected node address
 * @returns {void}
 */
function setNodeConnected(connected, node)
```

```javascript
/**
 * Saves bookmarks to localStorage and re-renders.
 * @returns {void}
 */
function saveBookmarks()
```

```javascript
/**
 * Renders bookmarked nodes and SCIDs in the bookmarks page.
 * @returns {void}
 */
function renderBookmarks()
```

```javascript
/**
 * Persists settings to localStorage and POSTs to the server.
 * @returns {void}
 */
function saveSettings()
```

```javascript
/**
 * Loads settings from localStorage, merging with server-side config.
 * @returns {Promise<void>}
 */
async function loadSettings()
```

```javascript
/**
 * Connects to the default node on startup if autoConnect is enabled.
 * @returns {Promise<void>}
 */
async function autoConnect()
```

```javascript
/**
 * Navigates to a named page in the SPA.
 * @param {string} page - Page identifier (e.g. "server", "search", "bookmarks")
 * @returns {void}
 */
function navigateTo(page)
```

```javascript
/**
 * Creates a status dot span element.
 * @param {"connected"|"error"|"pending"|"warning"} state - Dot color class
 * @returns {HTMLElement} A <span> with class "status-dot {state}"
 */
function createDot(state)
```

```javascript
/**
 * Formats milliseconds into a human-readable age string.
 * @param {number} ms - Milliseconds elapsed
 * @returns {string} e.g. "just now", "5m ago", "2h ago"
 */
function formatAge(ms)
```

```javascript
/**
 * Formats seconds into a human-readable duration string.
 * @param {number} secs - Seconds elapsed
 * @returns {string} e.g. "45s", "3m 12s", "1h 30m"
 */
function formatDuration(secs)
```

```javascript
/**
 * Formats a difficulty value into a human-readable hashrate.
 * @param {number} diff - Difficulty / hashrate value
 * @returns {string} e.g. "1.23 TH/s", "456 MH/s"
 */
function formatHashrate(diff)
```

#### Events Listened

| Event | Source | Action |
|---|---|---|
| `DOMContentLoaded` | Browser | Initializes bookmarks, settings, auto-connect, WebSocket |
| `pageChanged` | Custom | Updates navigation highlight |
| `nodeConnected` | Custom | Triggers search.js reload |
| `nodeDisconnected` | Custom | Resets search state |

#### Global Exports

```javascript
/**
 * Returns the current "direct load" setting.
 * @returns {boolean} True if SCIDs should auto-load on click
 */
window.getDirectLoadSetting()

/**
 * Returns the list of file extensions to hide from search results.
 * @returns {string[]} Array of lowercase extensions (e.g. [".jpg", ".shards"])
 */
window.getHiddenExtensions()

/**
 * Re-renders the tag input chips in settings.
 * Defined dynamically by initTagInput().
 * @returns {void}
 */
window.renderTags()
```

---

### 15. search.js

**File:** `web/search.js`
**Type:** IIFE (immediately-invoked function expression)
**Dependencies:** `fuse.min.js` (Fuse.js fuzzy search), `dashboard.js` (global helpers)

#### Purpose

TELA application search engine. Loads all discovered TELA apps from the HyperGnomon indexer + bundled registry, enriches them with ratings data, and provides fuzzy search with filtering, sorting, and autocomplete suggestions.

#### Module-Level State

```javascript
let allResults = [];        // All loaded TELA apps
let fuse = null;            // Fuse.js instance for fuzzy search
let minRating = 30;         // Minimum rating filter threshold
let loadToken = 0;          // Cancellation token for async loads
let resultsLoaded = false;  // Whether initial load has completed
let suggestionResults = []; // Current autocomplete suggestions
let suggestionIndex = -1;   // Keyboard-navigated suggestion index
let apiBase = "http://127.0.0.1:18082/api"; // HyperGnomon API base URL
```

#### Key Functions

```javascript
/**
 * Loads all TELA apps from discovery endpoint + HyperGnomon API,
 * enriches with ratings (concurrent workers), and renders results.
 * Uses a loadToken to cancel stale requests.
 * @returns {Promise<void>}
 */
async function loadSearchSCIDs()
```

```javascript
/**
 * Fetches rating data for a single SCID from the HyperGnomon API.
 * @param {string} scid - 64-character hex SCID
 * @returns {Promise<{scid: string, likes: number, dislikes: number, average: number, createdHeight: number}|null>}
 */
async function fetchSCIDData(scid)
```

```javascript
/**
 * Renders the filtered and sorted result list into the DOM.
 * @param {Array<{scid: string, dURL: string, nameHdr: string, ...}>} results - Apps to render
 * @returns {void}
 */
function renderResults(results)
```

```javascript
/**
 * Sorts an array of app results by the selected sort mode.
 * @param {Array} list - Results to sort
 * @param {"top_rated"|"name_asc"|"name_desc"|"newest"|"oldest"} mode - Sort key
 * @returns {Array} New sorted array (does not mutate)
 */
function sortResults(list, mode)
```

```javascript
/**
 * Executes a fuzzy search query via Fuse.js and renders matching results.
 * @param {string} value - User search input
 * @returns {void}
 */
function runSearch(value)
```

```javascript
/**
 * Populates the autocomplete dropdown with dURL suggestions.
 * @param {string} input - Current search box value
 * @returns {Array<{scid: string, dURL: string, nameHdr: string}>} Matching apps
 */
function getDURLSuggestions(input)
```

```javascript
/**
 * Renders the autocomplete suggestion dropdown.
 * @param {Array} results - Suggestion items to display
 * @returns {void}
 */
function renderSuggestions(results)
```

```javascript
/**
 * Handles clicking a search result — fills the SCID input and triggers load.
 * @param {string} scid - The clicked result's SCID
 * @returns {void}
 */
function handleSCIDClick(scid)
```

```javascript
/**
 * Returns the current sort mode from the custom dropdown.
 * @returns {"top_rated"|"name_asc"|"name_desc"|"newest"|"oldest"}
 */
function getSortValue()
```

```javascript
/**
 * Creates the default hexagonal SVG icon for apps without a custom icon.
 * @returns {HTMLElement} A div containing an inline SVG
 */
function createHexIcon()
```

```javascript
/**
 * Checks if a URL should be hidden based on the hiddenExtensions setting.
 * @param {string} url - The file URL to check
 * @returns {boolean} True if the URL matches any hidden extension
 */
function isHiddenByExtension(url)
```

#### Events Listened

| Event | Source | Action |
|---|---|---|
| `pageChanged` | Custom | Loads results when navigating to search page |
| `nodeConnected` | Custom | Triggers full app reload |
| `wsEvent` | Custom | Reloads on `tip_synced`, adds new apps on `new_tela_app` |

#### Initialization

1. On page load: fetches `apiBase` from `/api/config`.
2. On `pageChanged` to "search": calls `loadSearchSCIDs()` if not already loaded.
3. On `nodeConnected`: resets and reloads all apps.
4. On `wsEvent` with `tip_synced`: triggers a full reload to pick up new apps.

---

### 16. tray-popup.html (Inline Script)

**File:** `web/tray-popup.html`
**Type:** Inline `<script>` within HTML
**Dependencies:** None

#### Purpose

Lightweight status popup opened when clicking the system tray icon. Polls the dashboard API every 3 seconds to display live connection status, daemon info, and sync progress. Provides a Start/Stop toggle button and an "Open Dashboard" link.

#### Key Logic

```javascript
/**
 * Polls /api/server_status and updates all DOM elements.
 * Handles status dots, live data rows, sync progress, and button state.
 * @returns {Promise<void>}
 */
async function poll()
```

```javascript
/**
 * Toggles node connection via the API.
 * Sends POST to /api/disconnect_node or /api/set_node.
 * @returns {Promise<void>}
 */
// Attached to #btn-toggle-node click
```

```javascript
/**
 * Formats milliseconds into a relative age string.
 * @param {number} ms - Milliseconds
 * @returns {string} e.g. "just now", "5m ago"
 */
function formatAge(ms)
```

```javascript
/**
 * Formats seconds into a duration string.
 * @param {number} s - Seconds
 * @returns {string} e.g. "3m 12s"
 */
function formatDuration(s)
```

```javascript
/**
 * Formats a difficulty number into a human-readable hashrate.
 * @param {number} diff - Raw difficulty value
 * @returns {string} e.g. "1.23 TH/s"
 */
function formatDiff(diff)
```

---

## 17. Data Flow

### Node Connection

```
User clicks "Connect"
  → dashboard.js: send("set_node", {node})
    → router.go: handleSetNode
      → state.SetNode(node)
      → tela.Start()
      → syncMgr.StartSync(node)
        → HyperGnomon indexer starts
        → 2s ticker → Hub.Send(sync_progress)
          → WebSocket → dashboard.js: updateSyncProgress()
      → OnConnected(true) → tray.SetConnected(true)
        → Tray icon turns green
```

### SCID Loading

```
User clicks search result
  → dashboard.js: loadBtn.onclick
    → send("load_scid", {scid})
      → router.go: handleLoadSCID
        → http.Get(/add/{scid}) → tela proxy
          → ProxyManager.handleAddSCID
            → tela.ServeTELA() or downloadAndReconstructShards()
            → Creates httputil.ReverseProxy
        → syncMgr.IndexSCIDNow(scid) [async]
        → Returns {url: "http://127.0.0.1:18081/tela/{scid}/"}
      → Returns to dashboard.js
    → window.open(url, "_blank")
      → Browser opens TELA app as a real web page
```

### Event Broadcasting

```
indexer sync ticker (2s)
  → sendEvent({event: "sync_progress", indexed, chain})
    → Hub.Send()
      → WebSocket JSON to all clients
        → dashboard.js: handleEvent()
          → CustomEvent("wsEvent") for search.js
          → updateSyncProgress() for UI
```

---

## 18. API Reference

### POST /api/set_node

Connect to a DERO daemon.

**Request:**
```json
{ "node": "127.0.0.1:10102" }
```

**Response:**
```json
{ "ok": true }
```

### POST /api/disconnect_node

Disconnect and stop all services.

**Response:**
```json
{ "ok": true }
```

### POST /api/server_status

Get full application status.

**Response:**
```json
{
  "ok": true,
  "result": {
    "tela": true,
    "gnomon": true,
    "connected": true,
    "node": "http://127.0.0.1:10102",
    "connected_at": 1700000000000,
    "tela_apps_count": 88,
    "daemon": {
      "version": "7.x.x",
      "network": "Mainnet",
      "difficulty": 123456789,
      "mempool_size": 2
    },
    "heights": {
      "indexed": 500000,
      "chain": 500001,
      "stable": 499980
    }
  }
}
```

### POST /api/load_scid

Load a TELA application.

**Request:**
```json
{ "scid": "a6832a5a09b82dc4b1034fd726b118da1df8ca9ad33e76bee4563e3f69d1d99a" }
```

**Response:**
```json
{
  "ok": true,
  "result": {
    "url": "http://127.0.0.1:18081/tela/a6832.../"
  }
}
```

### GET /api/config

**Response:**
```json
{
  "ok": true,
  "result": {
    "gnomon_api_port": 18082,
    "tela_port": 18081
  }
}
```

### GET /api/tela/discover

**Response:**
```json
{
  "ok": true,
  "result": {
    "apps": [
      {
        "scid": "a6832a5a...",
        "durl": "tela-demo.tela",
        "name": "TELA Demo",
        "descrHdr": "A demo application",
        "iconURL": "",
        "install_height": 123456,
        "from_api": true
      }
    ]
  }
}
```

### POST /api/tela/vars

Fetch SCID variables from the daemon (fallback ratings).

**Request:**
```json
{ "scids": ["a6832a5a..."] }
```

**Response:**
```json
{
  "ok": true,
  "result": {
    "vars": [
      {
        "scid": "a6832a5a...",
        "dURL": "tela-demo.tela",
        "nameHdr": "TELA Demo",
        "likes": 12,
        "dislikes": 3,
        "average": 72,
        "createdHeight": 123456
      }
    ]
  }
}
```

### GET /api/probe_xswd

**Response:**
```json
{ "xswd": true }
```

### GET /api/settings / POST /api/settings

Read/write application configuration. Body/Response is an `AppConfig` JSON object.

### WebSocket /ws

Server pushes JSON messages on the following events:

| Event | Description |
|---|---|
| `sync_progress` | `{indexed: number, chain: number}` |
| `tip_synced` | `{height: number}` |
| `catalog_progress` | `{total: number, filtered: number}` |
| `new_tela_app` | `{scid: string}` |
| `node_unreachable` | `{node: string}` |
| `node_recovered` | `{node: string}` |

---

## 19. Configuration

### File Locations

| File | Purpose |
|---|---|
| `~/.hyperwolf/config.json` | App settings, bookmarks, default node |
| `~/.hyperwolf/hyperwolf.log` | Application log |
| `~/.hyperwolf/gnomondb/` | HyperGnomon index database |

### config.json Schema

```json
{
  "bookmarks": {
    "scids": {
      "<hex-scid>": { "scid": "...", "label": "..." }
    },
    "nodes": {
      "<address>": { "node": "...", "label": "..." }
    }
  },
  "settings": {
    "defaultNode": "127.0.0.1:10102",
    "autoConnect": true,
    "directLoad": true,
    "hiddenExtensions": ".jpg,.png"
  },
  "theme": "dark"
}
```

### Settings Fields

| Field | Type | Default | Description |
|---|---|---|---|
| `defaultNode` | `string` | `""` | Auto-connect on startup |
| `autoConnect` | `bool` | `true` | Whether to auto-connect to defaultNode |
| `directLoad` | `bool` | `true` | Open SCIDs in new tab when clicking search results |
| `hiddenExtensions` | `string` | `""` | Comma-separated extensions to hide (e.g. `.jpg,.shards`) |

---

## 20. Build & Cross-Compilation

### Local Build

```bash
CGO_ENABLED=0 go build -o hyperwolf .
```

### Cross-Compile

```bash
# Linux (amd64)
GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -o hyperwolf-linux .

# macOS (amd64)
GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -o hyperwolf-darwin .

# Windows (amd64)
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o hyperwolf.exe .

# macOS (arm64 / Apple Silicon)
GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -o hyperwolf-darwin-arm64 .
```

### Install Desktop Entries

```bash
./hyperwolf --install
```

### Uninstall

```bash
./hyperwolf --uninstall
```

---

## 21. Usage Examples

### Start the Application

```bash
# Default ports
./hyperwolf
# Dashboard: http://127.0.0.1:18080/

# Custom ports
./hyperwolf --dashboard-port 8080 --tela-port 8081 --gnomon-api 8082
```

### Programmatic Node Connection (via API)

```bash
# Connect to a node
curl -X POST http://127.0.0.1:18080/api/set_node \
  -H "Content-Type: application/json" \
  -d '{"node": "127.0.0.1:10102"}'

# Check status
curl -X POST http://127.0.0.1:18080/api/server_status

# Load a SCID
curl -X POST http://127.0.0.1:18080/api/load_scid \
  -H "Content-Type: application/json" \
  -d '{"scid": "a6832a5a09b82dc4b1034fd726b118da1df8ca9ad33e76bee4563e3f69d1d99a"}'

# Disconnect
curl -X POST http://127.0.0.1:18080/api/disconnect_node
```

### WebSocket Event Stream

```javascript
const ws = new WebSocket("ws://127.0.0.1:18080/ws");
ws.onmessage = (e) => {
  const msg = JSON.parse(e.data);
  console.log(msg.event, msg);
};
// Outputs: sync_progress {indexed: 500, chain: 500000}
//          tip_synced    {height: 500000}
//          catalog_progress {total: 88, filtered: 45}
```

### Access a Loaded TELA App

After loading a SCID via the dashboard or API, the app is accessible at:

```
http://127.0.0.1:18080/tela/{scid}/
http://127.0.0.1:18081/tela/{scid}/
```

Both the dashboard proxy and the direct TELA proxy port serve the same content.

---

*Generated for HyperWolf v1.0.0 — DERO TELA Browser*
