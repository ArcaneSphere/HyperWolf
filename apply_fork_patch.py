#!/usr/bin/env python3
"""Apply the graceful Stop() patch to HyperGnomon fork's api/http.go + api/tela_content.go."""
import sys

def patch_file(path, replacements, must_all_match=True):
    with open(path, "r", encoding="utf-8") as f:
        src = f.read()
    for old, new, label in replacements:
        if old not in src:
            print(f"  [MISS] {label}: pattern not found in {path}")
            if must_all_match:
                sys.exit(1)
            continue
        if src.count(old) != 1:
            print(f"  [AMBIG] {label}: pattern appears {src.count(old)}x in {path}; skipping")
            if must_all_match:
                sys.exit(1)
            continue
        src = src.replace(old, new, 1)
        print(f"  [OK] {label}")
    with open(path, "w", encoding="utf-8") as f:
        f.write(src)

# ---------------- api/http.go ----------------
http_path = "api/http.go"

http_edits = [
    # Edit 1: imports — add context
    (
        'import (\n\t"encoding/json"\n\t"net/http"',
        'import (\n\t"context"\n\t"encoding/json"\n\t"net/http"',
        "import context",
    ),
    # Edit 2: struct fields — add stopCh + stopOnce after cachedInfo
    (
        "\tmu         sync.RWMutex\n\tcachedInfo *structures.GetInfoResult\n\n\tassetCatalogMu",
        "\tmu         sync.RWMutex\n\tcachedInfo *structures.GetInfoResult\n\n\t// stopCh is closed by Stop to signal the background loops to exit.\n\tstopCh chan struct{}\n\t// stopOnce makes Stop idempotent: the first call shuts down, later\n\t// calls are no-ops.\n\tstopOnce sync.Once\n\n\tassetCatalogMu",
        "struct stopCh/stopOnce",
    ),
    # Edit 3: Start() — wire stopCh into background loops + retain srv
    (
        "\t// Start background info caching + TELA cache invalidator\n\tgo s.refreshInfoLoop()\n\tgo s.runTELAInvalidator()\n\n\tlogger.Infof(\"HTTP API listening on %s\", s.listenAddr)\n\tsrv := &http.Server{\n\t\tAddr:         s.listenAddr,\n\t\tHandler:      r,\n\t\tReadTimeout:  10 * time.Second,\n\t\tWriteTimeout: 30 * time.Second,\n\t\tIdleTimeout:  60 * time.Second,\n\t}\n\treturn srv.ListenAndServe()\n}",
        "\t// Start background info caching + TELA cache invalidator. Both exit\n\t// when stopCh is closed by Stop.\n\ts.mu.Lock()\n\tif s.stopCh == nil {\n\t\ts.stopCh = make(chan struct{})\n\t}\n\tstopCh := s.stopCh\n\ts.mu.Unlock()\n\n\tgo s.refreshInfoLoop(stopCh)\n\tgo s.runTELAInvalidator(stopCh)\n\n\tlogger.Infof(\"HTTP API listening on %s\", s.listenAddr)\n\tsrv := &http.Server{\n\t\tAddr:         s.listenAddr,\n\t\tHandler:      r,\n\t\tReadTimeout:  10 * time.Second,\n\t\tWriteTimeout: 30 * time.Second,\n\t\tIdleTimeout:  60 * time.Second,\n\t}\n\ts.mu.Lock()\n\ts.srv = srv\n\ts.mu.Unlock()\n\treturn srv.ListenAndServe()\n}",
        "Start wires stopCh + retains srv",
    ),
    # Edit 4a: refreshInfoLoop signature + select
    (
        "func (s *Server) refreshInfoLoop() {\n\ts.refreshInfo()\n\tticker := time.NewTicker(10 * time.Second)\n\tdefer ticker.Stop()\n\tfor range ticker.C {\n\t\ts.refreshInfo()\n\t}\n}",
        "func (s *Server) refreshInfoLoop(stopCh <-chan struct{}) {\n\ts.refreshInfo()\n\tticker := time.NewTicker(10 * time.Second)\n\tdefer ticker.Stop()\n\tfor {\n\t\tselect {\n\t\tcase <-stopCh:\n\t\t\treturn\n\t\tcase <-ticker.C:\n\t\t\ts.refreshInfo()\n\t\t}\n\t}\n}",
        "refreshInfoLoop stoppable",
    ),
    # Edit 4b: add graceful Stop() after Start()
    (
        "\ts.mu.Lock()\n\ts.srv = srv\n\ts.mu.Unlock()\n\treturn srv.ListenAndServe()\n}\n\n// refreshInfoLoop periodically fetches daemon info and caches it.",
        "\ts.mu.Lock()\n\ts.srv = srv\n\ts.mu.Unlock()\n\treturn srv.ListenAndServe()\n}\n\n// Stop gracefully shuts down the HTTP server and its background loops.\n// Safe to call before Start (no-op) and safe to call multiple times.\nfunc (s *Server) Stop() {\n\ts.stopOnce.Do(func() {\n\t\t// Stop the background loops first so they don't wake up mid-shutdown.\n\t\ts.mu.Lock()\n\t\tif s.stopCh != nil {\n\t\t\tclose(s.stopCh)\n\t\t}\n\t\tsrv := s.srv\n\t\ts.srv = nil\n\t\ts.mu.Unlock()\n\t\tif srv == nil {\n\t\t\treturn\n\t\t}\n\n\t\t// Graceful: stop accepting new connections and let in-flight requests\n\t\t// finish, with a deadline as a backstop. If the deadline expires,\n\t\t// force-close whatever remains.\n\t\tctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)\n\t\tdefer cancel()\n\t\tif err := srv.Shutdown(ctx); err != nil {\n\t\t\t_ = srv.Close()\n\t\t}\n\t})\n}\n\n// refreshInfoLoop periodically fetches daemon info and caches it.",
        "add graceful Stop()",
    ),
]

# ---------------- api/tela_content.go ----------------
tela_path = "api/tela_content.go"

tela_edits = [
    # Edit 5: runTELAInvalidator signature + select
    (
        "func (s *Server) runTELAInvalidator() {\n\tif s.bus == nil || s.telaCache == nil {\n\t\treturn\n\t}\n\t_, ch, cancel := s.bus.Subscribe(eventbus.Filter{\n\t\tEvents: map[eventbus.EventType]struct{}{\n\t\t\teventbus.EventInstall:   {},\n\t\t\teventbus.EventVarChange: {},\n\t\t},\n\t})\n\tdefer cancel()\n\tfor e := range ch {\n\t\tif e.SCID == \"\" {\n\t\t\tcontinue\n\t\t}\n\t\ts.telaCache.InvalidatePrefix(e.SCID)\n\t\tif err := s.store.DeleteTELAContentForSCID(e.SCID); err != nil {\n\t\t\tlogger.Debugf(\"tela content invalidate %s: %v\", e.SCID, err)\n\t\t}\n\t}\n}",
        "func (s *Server) runTELAInvalidator(stopCh <-chan struct{}) {\n\tif s.bus == nil || s.telaCache == nil {\n\t\treturn\n\t}\n\t_, ch, cancel := s.bus.Subscribe(eventbus.Filter{\n\t\tEvents: map[eventbus.EventType]struct{}{\n\t\t\teventbus.EventInstall:   {},\n\t\t\teventbus.EventVarChange: {},\n\t\t},\n\t})\n\tdefer cancel()\n\tfor {\n\t\tselect {\n\t\tcase <-stopCh:\n\t\t\treturn\n\t\tcase e, ok := <-ch:\n\t\t\tif !ok {\n\t\t\t\treturn\n\t\t\t}\n\t\t\tif e.SCID == \"\" {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\ts.telaCache.InvalidatePrefix(e.SCID)\n\t\t\tif err := s.store.DeleteTELAContentForSCID(e.SCID); err != nil {\n\t\t\t\tlogger.Debugf(\"tela content invalidate %s: %v\", e.SCID, err)\n\t\t\t}\n\t\t}\n\t}\n}",
        "runTELAInvalidator stoppable",
    ),
]

print("== api/http.go ==")
patch_file(http_path, http_edits)
print("== api/tela_content.go ==")
patch_file(tela_path, tela_edits)
print("\nAll edits applied.")
