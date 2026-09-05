# PROJECT STATUS — HyperWolf UI / LLM-vriendelijkheid

**Snapshot datum:** 2026-09-05 (guardrails-sessie: canonical-ownership checker live; daarvoor Fase 3 + commit `db55563`)
**Versie:** 0.11.0 (single source: `internal/buildinfo/version.go`)
**Module:** `hyperwolf` (Go 1.26.0), `//go:embed web/*` in `main.go:28`

---

## 1. Doelstelling (4 fasen)

| Fase | Inhoud | Status |
|------|--------|--------|
| 1 | Codebase in kaart brengen (html/js/css/registry/component-pattern) | ✅ klaar |
| 2 | Inline styles verplaatsen naar component CSS / style.css | ✅ klaar |
| 3 | Compositie-componenten extraheren (pure builders + eigen CSS + stories) | ✅ 8 van 8 klaar |
| 4 | Visual regression baseline + guardrails + AGENTS.md | 🟡 guardrails ✅ · baseline/AGENTS.md open |

**Fase 3 extraheren (8 kandidaten — allen klaar):**
- ✅ `SearchCard` — bottom-panel kaart-shell (5 stories)
- ✅ `StatusCard` — server-statuskaart + grid + `.card-apps` variant (4 stories)
- ✅ `LogContainer` — log-viewer shell (controls + terminal panel) (2 stories)
- ✅ `BookmarkItem` — bookmark row; **dashboard.js `createBookmarkItem()` delegeert hiernaar**
- ✅ `SearchControlsRow` — `.controls-row`/`.control-group`/`.custom-select*` + compacte toggle-overlay (3 stories)
- ✅ `SettingItem` — `.setting-item`/`.setting-label`/`.setting-description`/`.setting-row` (3 stories)
- ✅ `SyncProgress` — `.progress-bar`/`.progress-fill`/`.sync-label` + GNOMON SYNCBAR (5 stories)
- ✅ `OnboardingFlow` — `#onboarding-popover`/`.onboarding-node-option`/`.onb-*` flow (3 stories)

---

## 2. Componentcatalogus (19 componenten)

Registry: `web/ui/registry.json` (19 uniek). Bron: `npm run registry` → leest `web/ui/components/*/<Name>.stories.js`, schrijft `registry.json`.

**Bestaand (11):** HexIcon, LatestFindItem, LogEntry, NoResults, Popover, ResultRow, SearchSuggestion, StatusDot, TagChip, Toast, ToggleSwitch

**Nieuw geëxtraheerd (8):**
- `web/ui/components/SearchCard/` — `UI.SearchCard` (5 stories)
- `web/ui/components/StatusCard/` — `UI.StatusCard` + `StatusCard.css` (4 stories)
- `web/ui/components/LogContainer/` — `UI.LogContainer` + `LogContainer.css` (2 stories)
- `web/ui/components/BookmarkItem/` — `UI.BookmarkItem` + `BookmarkItem.css` (3 stories)
- `web/ui/components/SearchControlsRow/` — `UI.SearchControlsRow` + `SearchControlsRow.css` (3 stories)
- `web/ui/components/SettingItem/` — `UI.SettingItem` + `SettingItem.css` (3 stories)
- `web/ui/components/SyncProgress/` — `UI.SyncProgress` + `SyncProgress.css` (5 stories)
- `web/ui/components/OnboardingFlow/` — `UI.OnboardingFlow` + `OnboardingFlow.css` (3 stories)

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

Alle verhuisde rules uit `style.css` zijn vervangen door pointer-comments ("canonical owner is …"); de componenten eigen de rules nu. `style.css` telt 241/241 braces, geen dubbele shell-rules meer.

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

### SearchControlsRow
- Uit style.css: CONTROLS ROW-sectie (`.controls-row`, orphan `.controls`, `.control-group`, `#minRating`/`#minRatingVal`, `.custom-select*`-familie incl. menu/li/open-states), de compacte `.controls-row`-toggle-overlay en de 768px responsive rules (`gap`, `#minRating`, `.custom-select` width) → `SearchControlsRow.css`.
- `.search-sticky .controls-row` context-override verhuisde mee. `custom-select`-regels blijken dus wél in style.css te zitten (in het verleden niet gevonden door een verkeerde zoekterm).
- Builder reproduceert de statische rij exact (ids `minRating`/`minRatingVal`/`sortMode`/`showAllToggle`/`search-status`) met werkende custom-select; search.js blijft de statische rij sturen.

### SettingItem
- Uit style.css: CARDS-shared-regel werd gesplitst (`.setting-item,.info-box` → alleen `.info-box` blijft; `.setting-item` + hover + dark-hover + `.setting-label` + `.setting-description` + `.setting-row` + `.setting-row .setting-description` → `SettingItem.css`) + de 640px responsive `.setting-item`/`.setting-row` rules.
- `.setting-section*`/`.setting-section-separator` blijven in style.css (page-chrome).
- Builder: layout "stack" (label + content + description) of "row" (label+description links, controle rechts, bv. toggle).

### SyncProgress
- Uit style.css: PROGRESS BAR (`.progress-bar`, `.progress-fill`) + GNOMON SYNCBAR (`#sync-bar.indeterminate`, `@keyframes indeterminate-slide`) → `SyncProgress.css`.
- `.sync-label` had GEEN overlevende rules in style.css (gap uit eerdere opschoning) — canoniek hier (her)gedefinieerd.
- Builder reproduceert de sync-block (ids `sync-info`/`sync-label`/`sync-bar`/`sync-eta`) + API `setPercent/setLabel/setEta/setIndeterminate`; dashboard.js blijft de statische block sturen.

### OnboardingFlow
- Uit style.css: ONBOARDING POPOVER (`#onboarding-popover`, `.onboarding-nodes`, `.onboarding-node-option`+hover+selected, `.onb-label`, `.onb-addr`, `.onboarding-empty`) → `OnboardingFlow.css`.
- `.onboarding-content`/`.onboarding-desc`/`.popover*` shell leven al in `Popover.css` (niet verhuisd).
- Builder reproduceert de statische shell (ids `onboarding-popover`, `onboarding-node-list`, `onboarding-node-input`, `onboarding-skip`, `onboarding-connect`) met de dashboard.js-gedragingen: selectie wist input, typen wist selectie, Enter connect. dashboard.js blijft de flow sturen.

---

## 5. Verificatie-commands (exact, allen groen op 2026-09-05)

```bash
# JS syntax
node --check web/dashboard.js
node --check web/ui/components/SearchControlsRow/SearchControlsRow.js   # + de 7 andere nieuwe per bestand
# CSS braces balans
python3 -c "s=open('web/style.css').read(); print(s.count('{'), s.count('}'))"  # → 241 241
# Registry hergenereren
npm run registry            # → "registry.json: 19 components"
# Storybook
npm run build-storybook     # → ✓ built, storybook-static/
# Geserveerde pagina (app draait lokaal op :18080)
curl -s http://127.0.0.1:18080/ | grep -c 'style="'                            # → 1
curl -s http://127.0.0.1:18080/ | grep -c 'ui/components/.*\.css'              # → 19
curl -s http://127.0.0.1:18080/ | grep -c 'ui/components/.*\.js'               # → 19
curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/ui/components/SearchControlsRow/SearchControlsRow.css  # → 200
curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18080/ui/components/OnboardingFlow/OnboardingFlow.js         # → 200
curl -s http://127.0.0.1:18080/dashboard.js | grep -c 'UI.BookmarkItem({'        # → 1
curl -s http://127.0.0.1:18080/style.css | grep -cE '^\.status-card|^\.bookmark-item |^\.log-container|^\.card-row |^\.controls-row|^\.setting-item |^\.progress-bar|^\.custom-select|^#onboarding-popover'  # → 0
```

## 6. Build / install / restart (exact)

```bash
go build -o hyperwolf .                                          # repo root, embedt web/
PID=$(pgrep -x hyperwolf); kill $PID; sleep 1                     # eerst stoppen
cp hyperwolf ~/.local/bin/hyperwolf                              # anders "Text file busy"
nohup ~/.local/bin/hyperwolf > /tmp/hyperwolf.log 2>&1 & disown  # start
ps aux | grep '[h]yperwolf' | grep -v bash                        # PID check
```
**Huidig:** PID 325539 actief, dashboard `http://127.0.0.1:18080/` → 200 OK (HERBOUDEN: guardrails-commit `ea27c63`, embedding huidige `web/`).

---

## 7. Incidenten (BELANGRIJK)

### 7a. index.html-corruptie (hersteld)
Script-sectie van `web/index.html` raakte gedupliceerd/verknipt tijdens een edit. Herstel:
```bash
cp web/index.html /tmp/index.html.corrupted   # bewijsstuk
git checkout -- web/index.html                # pre-sessie staat (729 regels)
```
Daarna alle sessie-wijzigingen atomair opnieuw toegepast (één ge-asserted Python-pass). Verificatie: 738 regels, 15 css-links, 15 scripts, 1 inline style.

### 7b. DOCS/PROJECT_STATUS.md verdween ("rollback-achtig")
Het eerste snapshot-bestand verdween na diverse build/restart-stappen zonder tussenkomst. Alle code-wijzigingen bleven intact (opnieuw geverifieerd). Dit bestand is de (wederom) opnieuw geschreven versie — sindsdien opnieuw bevestigd aanwezig na elke sessie.

### 7c. Anchor mismatch bij sectie-splitsing (geleerd)
Sectie-headers in style.css wisselen in `=`-lengte van sluitregel (`=========== */`). Les: ankeren op de openingsregel (`/* ===============================\n   NAME`) i.p.v. het volledige header-blok; eerste run faalde netjes documenteersbaar door `assert` (niets is geschreven) — de atomic-python-werkwijse betaalt zich uit.

### 7d. Guardrail wees resterende owned-selectors aan (opgelost)
De `subjectTokenRules`-scan (rule-subject i.p.v. elk klasse-voorkomen) toonde 11 nog-losgekoppelde rules in style.css — alle 640px-residuen die de frequentie-heuristiek eerder miste: `.bookmark-item`/`.bookmark-actions` → BookmarkItem.css, `.controls`/`.controls button` → SearchControlsRow.css, `.search-card .latest-find-item` compact-blok → SearchCard.css, `.toggle-switch` align-self → ToggleSwitch.css. ALLOW-lijst empirisch uitgekamd (lege-lijst probe): alleen `search-sticky` + `sidebar` blijken échte page-level share-tokens; 9 overtollige entries verwijderd. Eindtoestand: `guardrails.cjs --discover` = 0 warnings.

**Werkwijze:** bulk-bewerkingen via één ge-asserted Python-script; outputs van bash/grep/read kunnen in deze omgeving gedupliceerd of verknipt **weergegeven** worden (bestandsinhoud is betrouwbaar geverifieerd via braces/node/curl; vertrouw niet op visuele output van lange blokken).

---

## 8. Next steps (todo)

1. **Fase 4 (rest):** visual regression baseline (Storybook) rond pixelmatch/playwright-core + AGENTS.md.
2. **Onderhoud registered:** regelrechtende loop aan nieuwe `npm run check` binding; een nieuwe component moet de canonical-ownership-checks sluitend houden.
3. Doorlooptijd/onderhoud: versie bumps via `internal/buildinfo/version.go` + web-fallbacks (`web/index.html #hw-version`, `web/search.js currentAppVersion`, SearchCard-story-tekst, README).

**Roadmap — backend LLM-readiness (opgenomen, niet nu):**
- Root-`AGENTS.md`: verken-volgorde van backend-modules, build/check/fix-workflow, relatie tot `hypergnomon_src` + `apply_fork_patch.py`.
- Architectuurdoc voor `internal/` (daemon/handler/router/hub, state, tela, tray): dataflow verbindings- en start-ups.
- Indexer-domeindoc: heights/SCIDs/classify-probe/FastSync, gnomon_ws, postscan-vars, DB-bewaarformaat (offline reads).
- Router/update-check + versie-flow (buildinfo single source) expliciet maken.
- Overweging: `CLAUDE.md`-afgeleide (zie `hypergnomon_src/`) óók voor de HyperWolf-wraplaag; dan is de hele app in één vocabulaire beschreven.

---

## 9. Belangrijke bestanden (huidige staat)

| Pad | Opmerking |
|-----|-----------|
| `web/index.html` | 19 css-links + 19 scripts; 1 inline style (`#sync-bar`); 754 regels, 38 `ui/components`-referenties |
| `web/style.css` | 230/230 braces, 1678 regels; compositie-rules + pointer-comments |
| `web/ui/guardrails.cjs` | canonical-ownership-checker; `npm run check` (`--discover` voor empirische ALLOW-onderhoud) |
| `web/dashboard.js` | `createBookmarkItem` → `UI.BookmarkItem`; sync/onboarding/log door ids gestuurd |
| `web/search.js` | stuurt de statische controls-row/custom-select (ongewijzigd in deze sessie) |
| `web/ui/components/` | 19 dirs (11 bestaand + 8 nieuw) |
| `web/ui/registry.js` + `registry.json` | 19 componenten; `npm run registry` |
| `web/tokens.css` | design tokens |
| `internal/buildinfo/version.go` | versie 0.11.0 |
| `~/.local/bin/hyperwolf` | geïnstalleerde binary (PID 325539) |
| `DOCS/PROJECT_STATUS.md` | dit bestand |