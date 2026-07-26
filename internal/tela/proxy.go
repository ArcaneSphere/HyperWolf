package tela

import (
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

	"github.com/civilware/tela"
)

type ProxyManager struct {
	mu       sync.RWMutex
	proxies  map[string]*httputil.ReverseProxy
	baseURLs map[string]string
	entries  map[string]string
	sharded  map[string]bool
	port     int
	once     sync.Once

	nodeFn func() string
}

func NewProxyManager(port int, nodeFn func() string) *ProxyManager {
	return &ProxyManager{
		proxies:  map[string]*httputil.ReverseProxy{},
		baseURLs: map[string]string{},
		entries:  map[string]string{},
		sharded:  map[string]bool{},
		port:     port,
		nodeFn:   nodeFn,
	}
}

func (pm *ProxyManager) Start() {
	pm.once.Do(func() {
		tela.AllowUpdates(true)

		mux := http.NewServeMux()
		mux.HandleFunc("/add/", pm.handleAddSCID)
		mux.HandleFunc("/tela/", pm.handleTELA)

		go func() {
			log.Printf("TELA proxy listening on :%d", pm.port)
			if err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", pm.port), mux); err != nil {
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
}

func (pm *ProxyManager) Reset() {
	pm.mu.Lock()
	pm.proxies = map[string]*httputil.ReverseProxy{}
	pm.baseURLs = map[string]string{}
	pm.entries = map[string]string{}
	pm.sharded = map[string]bool{}
	pm.mu.Unlock()
}

func (pm *ProxyManager) handleAddSCID(w http.ResponseWriter, r *http.Request) {
	scid := strings.TrimPrefix(r.URL.Path, "/add/")
	scid = strings.Split(scid, "/")[0]

	log.Printf("[ADD] %s", scid)

	node := pm.nodeFn()
	if node == "" {
		http.Error(w, "node not set", 400)
		return
	}

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
		http.Error(w, err.Error(), 500)
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
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Content-Security-Policy")
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

func downloadAndReconstructShards(scid string, index tela.INDEX, telaNode string) (string, error) {
	log.Printf("[SHARDS] Reconstructing SCID: %s", scid)

	baseName := strings.TrimSuffix(index.DURL, tela.TAG_DOC_SHARDS)
	appDir := filepath.Join(tela.GetClonePath(), baseName)
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
		dst := filepath.Join(appDir, key)
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
