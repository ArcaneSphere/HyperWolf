package tela

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/civilware/tela"
)

type ProxyManager struct {
	mu        sync.RWMutex
	proxies   map[string]*httputil.ReverseProxy
	baseURLs  map[string]string
	entries   map[string]string
	sharded   map[string]bool
	scidLocks map[string]*sync.Mutex // per-SCID mutexes for TOCTOU protection
	port      int
	once      sync.Once
	server    *http.Server

	nodeFn func() string
}

func NewProxyManager(port int, nodeFn func() string) *ProxyManager {
	return &ProxyManager{
		proxies:   map[string]*httputil.ReverseProxy{},
		baseURLs:  map[string]string{},
		entries:   map[string]string{},
		sharded:   map[string]bool{},
		scidLocks: map[string]*sync.Mutex{},
		port:      port,
		nodeFn:    nodeFn,
	}
}

func (pm *ProxyManager) Start() {
	pm.once.Do(func() {
		tela.AllowUpdates(true)

		mux := http.NewServeMux()
		mux.HandleFunc("/add/", pm.handleAddSCID)
		mux.HandleFunc("/tela/", pm.handleTELA)
		srv := &http.Server{
			Addr:    fmt.Sprintf("127.0.0.1:%d", pm.port),
			Handler: mux,
		}
		pm.mu.Lock()
		pm.server = srv
		pm.mu.Unlock()

		go func() {
			log.Printf("TELA proxy listening on :%d", pm.port)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("TELA proxy: %v", err)
			}
		}()
	})
}

func (pm *ProxyManager) GetProxy(scid string) *httputil.ReverseProxy {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.proxies[scid]
}

func (pm *ProxyManager) GetProxiedSCIDs() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	scids := make([]string, 0, len(pm.proxies))
	for scid := range pm.proxies {
		scids = append(scids, scid)
	}
	return scids
}

func (pm *ProxyManager) Shutdown() {
	tela.ShutdownTELA()
	pm.mu.Lock()
	srv := pm.server
	pm.server = nil
	pm.mu.Unlock()
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		_ = srv.Close()
	}
}

func (pm *ProxyManager) Reset() {
	pm.mu.Lock()
	pm.proxies = map[string]*httputil.ReverseProxy{}
	pm.baseURLs = map[string]string{}
	pm.entries = map[string]string{}
	pm.sharded = map[string]bool{}
	pm.scidLocks = map[string]*sync.Mutex{}
	pm.mu.Unlock()
}

func (pm *ProxyManager) getSCIDLock(scid string) *sync.Mutex {
	pm.mu.Lock()
	if pm.scidLocks[scid] == nil {
		pm.scidLocks[scid] = &sync.Mutex{}
	}
	lock := pm.scidLocks[scid]
	pm.mu.Unlock()
	return lock
}

// writeJSONError sends a JSON error response consistent with writeJSON's shape
// so that upstream callers (e.g. handleLoadSCID in router.go) always get
// parseable JSON regardless of success or failure.
func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": msg,
	})
}

func (pm *ProxyManager) handleAddSCID(w http.ResponseWriter, r *http.Request) {
	scid := strings.TrimPrefix(r.URL.Path, "/add/")
	scid = strings.Split(scid, "/")[0]

	log.Printf("[ADD] %s", scid)

	node := pm.nodeFn()
	if node == "" {
		writeJSONError(w, "node not set", http.StatusBadRequest)
		return
	}

	// Fast path: check if already loaded (read lock)
	pm.mu.RLock()
	if base, ok := pm.baseURLs[scid]; ok {
		pm.mu.RUnlock()
		writeJSON(w, scid, base)
		return
	}
	pm.mu.RUnlock()

	// Slow path: acquire per-SCID lock to prevent duplicate ServeTELA calls
	lock := pm.getSCIDLock(scid)
	lock.Lock()
	defer lock.Unlock()

	// Re-check under lock (double-checked locking)
	pm.mu.RLock()
	if base, ok := pm.baseURLs[scid]; ok {
		pm.mu.RUnlock()
		writeJSON(w, scid, base)
		return
	}
	pm.mu.RUnlock()

	telaNode := strings.TrimPrefix(node, "http://")
	var rawURL string
	isShardedSCID := false

	index, err := tela.GetINDEXInfo(scid, telaNode)
	if err == nil && strings.HasSuffix(index.DURL, tela.TAG_DOC_SHARDS) {
		rawURL, err = downloadAndReconstructShards(scid, index, telaNode)
		isShardedSCID = true
	} else {
		rawURL, err = tela.ServeTELA(scid, telaNode)
	}

	if err != nil {
		log.Printf("[ADD] %s — ServeTELA error: %v", scid, err)
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if rawURL == "" {
		log.Printf("[ADD] %s — ServeTELA returned empty URL (no error)", scid)
		writeJSONError(w, "SCID returned empty URL", http.StatusInternalServerError)
		return
	}

	var base, entry string
	if isShardedSCID && strings.HasSuffix(rawURL, ".html") {
		base = rawURL[:strings.LastIndex(rawURL, "/")+1]
		entry = rawURL[strings.LastIndex(rawURL, "/")+1:]
	} else {
		base = strings.TrimSuffix(rawURL, "/index.html")
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
	}

	log.Printf("[MAP] base=%s entry=%s sharded=%v", base, entry, isShardedSCID)

	target, _ := url.Parse(base)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
	}
	// Instead of stripping CSP entirely, set a permissive but explicit policy
	// that allows inline scripts/styles (needed by many TELA apps) while
	// still providing a security baseline.
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("Content-Security-Policy",
			"default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; "+
				"script-src 'self' 'unsafe-inline' 'unsafe-eval'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; "+
				"connect-src 'self' ws: wss:;")
		return nil
	}

	pm.mu.Lock()
	pm.proxies[scid] = proxy
	pm.baseURLs[scid] = base
	pm.entries[scid] = entry
	pm.sharded[scid] = isShardedSCID
	pm.mu.Unlock()

	writeJSON(w, scid, base)
}

func (pm *ProxyManager) handleTELA(w http.ResponseWriter, r *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/tela/"), "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	scid := parts[0]

	pm.mu.RLock()
	proxy := pm.proxies[scid]
	entry := pm.entries[scid]
	isSharded := pm.sharded[scid]
	pm.mu.RUnlock()

	if proxy == nil {
		http.Error(w, "SCID not loaded", 404)
		return
	}

	subPath := ""
	if len(parts) == 2 {
		subPath = parts[1]
	}

	if isSharded && subPath == "" && entry != "" {
		http.Redirect(w, r, "/tela/"+scid+"/"+url.PathEscape(entry), http.StatusFound)
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

func writeJSON(w http.ResponseWriter, scid, base string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok": true,
		"result": map[string]any{
			"scid": scid,
			"url":  base,
		},
	})
}

func findEntrypoint(folder string) (string, error) {
	ents, err := os.ReadDir(folder)
	if err != nil {
		return "", err
	}

	var htmlFiles []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".html") {
			htmlFiles = append(htmlFiles, e.Name())
		}
	}

	for _, f := range htmlFiles {
		if f == "index.html" {
			return f, nil
		}
	}

	if len(htmlFiles) > 0 {
		return htmlFiles[0], nil
	}

	return "", fmt.Errorf("no HTML entrypoint found")
}

func detectShard(nameHdr, compression string) (int, string) {
	name := strings.TrimSuffix(nameHdr, compression)
	lastDash := strings.LastIndex(name, "-")
	if lastDash < 0 {
		return 0, tela.TrimCompressedExt(nameHdr)
	}

	var idx int
	if _, err := fmt.Sscanf(name[lastDash+1:], "%d", &idx); err != nil || idx < 1 {
		return 0, tela.TrimCompressedExt(nameHdr)
	}

	return idx, name[:lastDash]
}

func parseShardRawBytes(doc tela.DOC) ([]byte, error) {
	code := doc.Code
	start := strings.Index(code, "/*")
	end := strings.LastIndex(code, "*/")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no shard data found")
	}
	return []byte(code[start+3 : end]), nil
}

// pathWithin returns a path rooted under root and rejects lexical traversal
// outside that root. TELA shard metadata is chain-provided and must not be
// allowed to choose arbitrary filesystem destinations.
func pathWithin(root string, parts ...string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := rootAbs
	for _, part := range parts {
		if filepath.IsAbs(part) || filepath.VolumeName(part) != "" {
			return "", fmt.Errorf("absolute path component: %q", part)
		}
		candidate = filepath.Join(candidate, part)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root: %q", filepath.Join(parts...))
	}
	current := rootAbs
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink path component: %q", component)
		}
	}
	return candidate, nil
}

// normalizeTELAPath converts a TELA virtual-root path such as "/index.html"
// into a path relative to the reconstructed app directory. The leading slash
// is not a host filesystem root; it is part of TELA's virtual path format.
func normalizeTELAPath(path string) string {
	return strings.TrimLeft(path, "/\\")
}

func downloadAndReconstructShards(scid string, index tela.INDEX, telaNode string) (string, error) {
	log.Printf("[SHARDS] Reconstructing SCID: %s", scid)

	baseName := normalizeTELAPath(strings.TrimSuffix(index.DURL, tela.TAG_DOC_SHARDS))
	appDir, err := pathWithin(tela.GetClonePath(), baseName)
	if err != nil {
		return "", fmt.Errorf("unsafe TELA app path: %w", err)
	}
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return "", err
	}

	type shard struct {
		idx  int
		data []byte
	}
	type group struct {
		shards      []shard
		compression string
		isSharded   bool
	}

	groups := map[string]*group{}

	for _, docSCID := range index.DOCs {
		doc, err := tela.GetDOCInfo(docSCID, telaNode)
		if err != nil {
			return "", err
		}

		raw, err := parseShardRawBytes(doc)
		if err != nil {
			return "", err
		}

		idx, base := detectShard(doc.Headers.NameHdr, doc.Compression)
		key := base
		if doc.SubDir != "" {
			key = filepath.Join(doc.SubDir, base)
		}

		if _, ok := groups[key]; !ok {
			groups[key] = &group{}
		}
		g := groups[key]
		g.compression = doc.Compression
		if idx > 0 {
			g.isSharded = true
		}
		g.shards = append(g.shards, shard{idx, raw})
	}

	for key, g := range groups {
		dst, err := pathWithin(appDir, normalizeTELAPath(key))
		if err != nil {
			return "", fmt.Errorf("unsafe TELA shard path: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return "", err
		}

		var data []byte
		if g.isSharded {
			sort.Slice(g.shards, func(i, j int) bool { return g.shards[i].idx < g.shards[j].idx })
			buf := []byte{}
			for _, s := range g.shards {
				buf = append(buf, s.data...)
			}
			if g.compression != "" {
				var err error
				data, err = tela.Decompress(buf, g.compression)
				if err != nil {
					return "", err
				}
			} else {
				data = buf
			}
		} else {
			data = g.shards[0].data
			if g.compression != "" {
				var err error
				data, err = tela.Decompress(data, g.compression)
				if err != nil {
					return "", err
				}
			}
		}

		if err := os.WriteFile(dst, data, 0644); err != nil {
			return "", err
		}
	}

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	go http.ListenAndServe(
		fmt.Sprintf("127.0.0.1:%d", port),
		http.FileServer(http.Dir(appDir)),
	)

	entry, err := findEntrypoint(appDir)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("http://127.0.0.1:%d/%s", port, entry), nil
}
