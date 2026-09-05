# PROJECT STATUS — HyperWolf UI / LLM-vriendelijkheid

**Snapshot datum:** 2026-09-05 (continuatiesessie, na StatusCard/LogContainer/BookmarkItem)
**Versie:** 0.10.0 (single source: `internal/buildinfo/version.go`)
**Module:** `hyperwolf` (Go 1.26.0), `//go:embed web/*` in `main.go:28`

---

## 1. Doelstelling (4 fasen)

| Fase | Inhoud | Status |
|------|--------|--------|
| 1 | Codebase in kaart brengen (html/js/css/registry/component-pattern) | ✅ klaar |
| 2 | Inline styles verplaatsen naar component CSS / style.css | ✅ klaar |
| 3 | Compositie-componenten extraheren (pure builders + eigen CSS + stories) | 🔶 4 van 8 klaar |
| 4 | Visual regression baseline + guardrails + AGENTS.md | ⬜ open |

**Fase 3 extraheren (8 kandidaten):**
- ✅ `SearchCard` — bottom-panel kaart-shell (5 stories)
- ✅ `StatusCard` — server-statuskaart + grid + `.card-apps` variant (4 stories)
- ✅ `LogContainer` — log-viewer shell (controls + terminal panel) (2 stories)
- ✅ `BookmarkItem` — bookmark row; **dashboard.js `createBookmarkItem()` delegeert hiernaar**
- ⬜ `SearchControlsRow` — `.controls-row`/`.search-controls` + custom-select markup
- ⬜ `SettingItem` — `.setting-item`/`.setting-label`/`.setting-description`/`.setting-row`
- ⬜ `SyncProgress` — `.progress-bar`/`.progress-fill`/`.sync-label` + GNOMON SYNCBAR compositie
- ⬜ `OnboardingFlow` — `#onboarding-popover`/`.onboarding-node-option`/`.onb-*` rules

---

## 2. Componentcatalogus (15 componenten)

Registry: `web/ui/registry.json` (15 uniek). Bron: `npm run registry` → leest `web/ui/components/*/<Name>.stories.js`, schrijft `registry.json`.

**Bestaand (11):** HexIcon, LatestFindItem, LogEntry, NoResults, Popover, ResultRow, SearchSuggestion, StatusDot, TagChip, Toast, ToggleSwitch

**Nieuw geëxtraheerd (4):**
- `web/ui/components/SearchCard/` — `UI.SearchCard` (5 stories)
- `web/ui/components/StatusCard/` — `UI.StatusCard` + `StatusCard.css` (4 stories)
- `web/ui/components/LogContainer/` — `UI.LogContainer` + `LogContainer.css` (2 stories)
- `web/ui/components/BookmarkItem/` — `UI.BookmarkItem` + `BookmarkItem.css` (3 stories)

Elk component: `Name.js` (pure builder, IIFE, `window.UI.Name`, geen DOM-side-effects) + `Name.css` (canoniek) + `Name.stories.js` (titel + `parameters.llm` metadata: description/useWhen/avoidWhen/related).

---

## 3. Fase 2 — Inline styles opgeruimd (klaar)

**Resultaat:** 30 inline `style=""` → 1 over (**`#sync-bar` width:0%**, runtime door JS gezet). Geverifieerd:
```bash
curl -s http://127.0.0.1:18080/ | grep -c 'style="'   # → 1
```
Nieuw in `web/style.css`: `.setting-row .setting-description`, `.setting-section-separator`, `.select-compact`, `.select-mini`, `#reset-settings-btn`, `.card-apps .card-value`, `.card-apps .card-row`, `#sv-tela-status`, `#sync-info`, `#sync-eta`, log-page rules, `.info-box .resource-group`. In `Popover.css`: `#confirm-popover .popover-content { max-width: 360px; }`.

---

## 4. Fase 3 — Geëxtraheerde componenten + CSS-ownership

Alle verhuisde rules uit `style.css` zijn vervangen door pointer-comments ("canonical owner is …"); de componenten eigen de rules nu. `style.css` telt 287/287 braces, geen dubbele shell-rules meer.

### StatusCard
- Uit style.css "STATUS CARD GRID" (incl. `#sv-tela-status`, `.card-expand`-familie) → `StatusCard.css`.
- Responsive `.status-grid`/`.card-row`/`.card-label` (max-width:640) → `StatusCard.css`.
- Static server-HTML blijft staan (gebruikt selfde classes); stories + builder als migratietarget.

### LogContainer
- Uit style.css "LOG PAGE" (`.log-container`, `.log-controls`-familie, `#log-auto-scroll`, dark variant) → `LogContainer.css`.
- Builder reproduceert de shell met exact de ids die dashboard.js verwacht (`#log-level-filter`, `#log-refresh-btn`, `#log-clear-btn`, `#log-auto-scroll`, `#log-container`).

### BookmarkItem
- Uit style.css: BOOKMARKS-sectie + de `.bookmark-item` vermeldingen in de gedeelde CARDS-regel (`.setting-item,.bookmark-item,.info-box`) → `BookmarkItem.css`.
- `web/dashboard.js`: `createBookmarkItem()` is nu een dunne delegatie naar `UI.BookmarkItem({ label, value, onLoad, onRemove, onCommit })`; de scid-vs-node-classificatie (op `value.length === 64`) en opslag blijven in de app.

---

## 5. Verificatie-commands (exact, allen groen op 2026-09-05)

```bash
# JS syntax
node --check web/dashboard.js
node --check web/ui/components/{StatusCard,LogContainer,BookmarkItem}/*.js     # per bestand
# CSS braces balans
python3 -c "s=open('web/style.css').read(); print(s.count('{'), s.count('}'))"  # → 287 287
# Registry hergenereren
npm run registry            # → "registry.json: 15 components"
# Storybook
npm run build-storybook     # → ✓ built, storybook-static/
# Geserveerde pagina (app draait lokaal op :18080)
curl -s http://127.0.0.1:18080/ | grep -c 'style="'                            # → 1
curl -s http://127.0.0.1:18080/ | grep -c 'ui/components/.*\.css'              # → 15
curl -s http://127.0.0.1:18080/ | grep -c 'ui/components/.*\.js'               # → 15
curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/ui/components/StatusCard/StatusCard.css   # → 200
curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/ui/components/LogContainer/LogContainer.js  # → 200
curl -s http://127.0.0.1:18080/dashboard.js | grep -c 'UI.BookmarkItem({'        # → 1
curl -s http://127.0.0.1:18080/style.css | grep -cE '^\.status-card|^\.bookmark-item |^\.log-container|^\.card-row '  # → 0
```

## 6. Build / install / restart (exact)

```bash
go build -o hyperwolf .                                          # repo root, embedt web/
PID=$(pgrep -x hyperwolf); kill $PID; sleep 1                     # eerst stoppen
cp hyperwolf ~/.local/bin/hyperwolf                              # anders "Text file busy"
nohup ~/.local/bin/hyperwolf > /tmp/hyperwolf.log 2>&1 & disown  # start
ps aux | grep '[h]yperwolf' | grep -v bash                        # PID check
```
**Huidig:** PID 283264 actief, dashboard `http://127.0.0.1:18080/` (200 OK).

---

## 7. Incidenten (BELANGRIJK)

### 7a. index.html-corruptie (hersteld)
Script-sectie van `web/index.html` raakte gedupliceerd/verknipt tijdens een edit. Herstel:
```bash
cp web/index.html /tmp/index.html.corrupted   # bewijsstuk
git checkout -- web/index.html                # pre-sessie staat (729 regels)
```
Daarna alle sessie-wijzigingen atomair opnieuw toegepast (één ge-asserted Python-pass). Verificatie nu: 738 regels, 15 css-links, 15 scripts, 1 inline style, normaal gedupliceerd rondom journals.

### 7b. DOCS/PROJECT_STATUS.md verdween ("rollback-achtig")
Het eerste snapshot-bestand verdween na diverse build/restart-stappen zonder tussenkomst. Alle code-wijzigingen bleven intact (opnieuw geverifieerd: componenten, braces, registry 15, dashboard-delegatie). Dit bestand is de heropnieuwde versie.

**Werkwijze:** bulk-bewerkingen via één ge-asserted Python-script; outputs van bash/grep/read kunnen in deze omgeving gedupliceerd of verknipt **weergegeven** worden (bestandsinhoud is betrouwbaar geverifieerd via braces/node/curl; vertrouw niet op visuele output van lange blokken).

---

## 8. Next steps (todo)

1. **Fase 3 rest (4):** `SearchControlsRow`, `SettingItem`, `SyncProgress`, `OnboardingFlow`.
2. **Fase 4:** visual regression baseline, guardrails, AGENTS.md.
3. **Commit** huidige staat (index.html, style.css, dashboard.js, Popover.css, registry.json + 4 nieuwe componentdirs + dit doc).

---

## 9. Belangrijke bestanden (huidige staat)

| Pad | Opmerking |
|-----|-----------|
| `web/index.html` | 738 regels; 15 css-links + 15 scripts; 1 inline style (`#sync-bar`) |
| `web/style.css` | 287/287 braces, 1997 regels; compositie-rules + pointer-comments |
| `web/dashboard.js` | 1464+ regels; `createBookmarkItem` → `UI.BookmarkItem` |
| `web/search.js` | 1042 regels; ongewijzigd in deze sessie |
| `web/ui/components/` | 15 dirs (11 + SearchCard, StatusCard, LogContainer, BookmarkItem) |
| `web/ui/registry.js` + `registry.json` | 15 componenten; `npm run registry` |
| `web/tokens.css` | design tokens |
| `internal/buildinfo/version.go` | versie 0.10.0 |
| `~/.local/bin/hyperwolf` | geïnstalleerde binary (PID 283264) |
| `DOCS/PROJECT_STATUS.md` | dit bestand |
