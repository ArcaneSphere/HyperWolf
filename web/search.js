(function () {
  const page = document.getElementById("page-search");
  if (!page) return;

  const searchBox    = document.getElementById("searchBox");
  const statusEl     = document.getElementById("search-status");
  const resultsEl    = document.getElementById("results");
  const minRatingEl  = document.getElementById("minRating");
  const minRatingVal = document.getElementById("minRatingVal");
  const scidInput    = document.getElementById("scid");
  const loadBtn      = document.getElementById("load");
  const searchClear  = document.getElementById("searchClear");
  const showAllToggle = document.getElementById("showAllToggle");
  const searchCardsEl = document.getElementById("search-cards");

  if (!searchBox || !resultsEl) return;

  // Central status writer — toggles a live spinner for loading phases.
  function setStatus(text) {
    statusEl.textContent = text;
    const busy = /\u23f3|Loading|Ratings|probing|Waiting/i.test(text || "");
    statusEl.classList.toggle("is-loading", busy);
  }

  // Move suggestions dropdown outside #content to body so it's never clipped
  const searchSuggestions = document.getElementById("searchSuggestions");
  if (searchSuggestions) {
    document.body.appendChild(searchSuggestions);
  }

  function positionDropdown() {
    if (!searchSuggestions || searchSuggestions.classList.contains("hidden")) return;
    const rect = searchBox.getBoundingClientRect();
    searchSuggestions.style.top = (rect.bottom + 4) + "px";
    searchSuggestions.style.left = rect.left + "px";
    searchSuggestions.style.width = rect.width + "px";
  }

  let apiBase = "http://127.0.0.1:18082/api";
  window.UI = window.UI || {};
  window.UI.apiBase = apiBase;

  (async function initApiBase() {
    try {
      const resp = await fetch("/api/config");
      const data = await resp.json();
      if (data?.ok && data?.result?.gnomon_api_port) {
        apiBase = "http://127.0.0.1:" + data.result.gnomon_api_port + "/api";
        window.UI.apiBase = apiBase;
      }
    } catch (e) {}
  })();

  let allResults = [];
  let fuse       = null;
  let minRating  = 30;
  let showAllSCIDs = false;
  let loadToken  = 0;
  let resultsLoaded = false;
  let fullLoadInProgress = false; // guards against catalog refresh racing a full load
  let lastFullLoadCount = 0;      // catalog size at the last full ratings load
  let catalogRefreshTimer = null; // throttle for lightweight catalog refreshes
  let metadataRetries = 0;

  // New-SCID detection: remembers the last-seen catalog so the user gets a
  // "N new app(s) found" notification + latest-finds suggestions on open.
  const SEEN_KEY = "hyperwolf.seenSCIDs";
  const newBadgeEl = document.getElementById("search-new-badge");
  const latestFindsEl = document.getElementById("latest-finds");
  const latestFindsNewEl = document.getElementById("latest-finds-new");
  const latestFindsListEl = document.getElementById("latest-finds-list");
  const newSCIDs = new Set();       // scids discovered since the stored baseline
  let baselineSeen = new Set();     // last known catalog (persisted)
  let newNotified = false;          // avoid spamming toasts

  function loadSeenBaseline() {
    try {
      const raw = JSON.parse(localStorage.getItem(SEEN_KEY) || "[]");
      baselineSeen = new Set(Array.isArray(raw) ? raw : []);
    } catch (e) {
      baselineSeen = new Set();
    }
  }

  function persistSeenBaseline() {
    try {
      localStorage.setItem(SEEN_KEY, JSON.stringify([...baselineSeen]));
    } catch (e) {}
  }

  function setNewBadge(count) {
    if (!newBadgeEl) return;
    if (count > 0) {
      newBadgeEl.textContent = count;
      newBadgeEl.title = count + " new TELA app" + (count !== 1 ? "s" : "") + " found";
    } else {
      newBadgeEl.textContent = "";
      newBadgeEl.title = "";
    }
  }

  function showLatestFinds(apps) {
    if (!latestFindsEl || !latestFindsListEl) return;
    // Always render the card so the 3-card layout stays stable — even with no
    // data yet. A placeholder replaces the list until apps are discovered.
    latestFindsEl.classList.remove("hidden");
    // "Latest" = newest DISCOVERED, not highest install height. In this DB all
    // installs share one re-index height, so height alone is useless — order by
    // (newly discovered first), then install height desc, then SCID.
    // Filter to only entries with real names (not raw SCID or filenames)
    const valid = apps.filter(a => {
      const name = (a.name || "").trim();
      const durl = (a.durl || "").trim();
      if (!name || name === a.scid) return false;
      if (durl && durl.endsWith(".js")) return false;
      return true;
    });
    const newest = [...valid]
      .sort((a, b) => {
        const aNew = newSCIDs.has(a.scid) ? 1 : 0;
        const bNew = newSCIDs.has(b.scid) ? 1 : 0;
        if (aNew !== bNew) return bNew - aNew;
        // Fall back to install height (highest = newest)
        return (b.install_height || 0) - (a.install_height || 0);
      })
      .slice(0, 3);

    if (!newest.length) {
      latestFindsListEl.replaceChildren();
      const empty = document.createElement("div");
      empty.className = "latest-find-empty";
      empty.textContent = waitingStatusText();
      latestFindsListEl.appendChild(empty);
      if (latestFindsNewEl) latestFindsNewEl.classList.add("hidden");
      return;
    }

    latestFindsListEl.replaceChildren();
    const isNew = (scid) => newSCIDs.has(scid);
    const anyNew = newest.some(r => isNew(r.scid));
    if (latestFindsNewEl) {
      latestFindsNewEl.classList.toggle("hidden", !anyNew);
      latestFindsNewEl.textContent = anyNew ? "✨ " + newest.filter(r => isNew(r.scid)).length + " new" : "";
    }

    newest.forEach(app => {
      const row = UI.LatestFindItem({ ...app, isNew: isNew(app.scid) }, {
        onClick: (selected) => handleSCIDClick(selected.scid),
      });
      latestFindsListEl.appendChild(row);
    });

    latestFindsEl.classList.remove("hidden");
  }

  function detectNewSCIDs(apps) {
    loadSeenBaseline();
    const current = new Set(apps.map(a => a.scid));
    newSCIDs.clear();
    // First run (empty baseline) just seeds — no false "new" spam.
    if (baselineSeen.size > 0) {
      for (const scid of current) {
        if (!baselineSeen.has(scid)) newSCIDs.add(scid);
      }
    }
    // Merge current into baseline and persist (old entries kept so re-finds
    // after a DB wipe don't re-alert as "new").
    for (const scid of current) baselineSeen.add(scid);
    persistSeenBaseline();
    setNewBadge(newSCIDs.size);
    if (newSCIDs.size > 0 && !newNotified && typeof window.pushToast === "function") {
      newNotified = true;
      const n = newSCIDs.size;
      window.pushToast("connected", `🎉 ${n} new TELA app${n !== 1 ? "s" : ""} found`);
    }
  }

  function updateLatestFinds() {
    const withHeight = allResults.map((r, i) => ({ scid: r.scid, durl: r.dURL, name: r.nameHdr, descrHdr: r.descrHdr, iconURL: r.iconURL, install_height: r.createdHeight, _idx: i }));
    showLatestFinds(withHeight);
  }

  let suggestionResults = [];
  let suggestionIndex = -1;

  if (minRatingEl && minRatingVal) {
    minRatingEl.value        = minRating;
    minRatingVal.textContent = minRating;
  }

  function isHiddenByExtension(url) {
    if (!url) return false;
    const hidden = window.getHiddenExtensions?.() || [];
    const lower = url.toLowerCase();
    return hidden.some(ext => lower.endsWith(ext));
  }

  async function fetchSCIDData(scid) {
    try {
      const resp = await fetch(`${apiBase}/tela/${scid}/ratings`);
      if (!resp.ok) return null;
      const data = await resp.json();
      if (!data.ratings || data.count === 0) return null;
      const likes = data.summary?.likes ?? data.ratings.filter(r => r.score >= 50).length;
      const dislikes = data.summary?.dislikes ?? data.ratings.filter(r => r.score < 50).length;
      const average = Math.round(data.avg ?? 0);
      const createdHeight = data.ratings.reduce((min, r) => Math.min(min, r.height), Infinity) || 0;
      return { scid, likes, dislikes, average, createdHeight };
    } catch (err) {
      console.warn("SCID fetch error", err);
      return null;
    }
  }

  function waitingStatusText() {
    const nodeStatus = document.getElementById("sc-node-status");
    const connected = nodeStatus && /Connected/i.test(nodeStatus.textContent || "");
    return connected ? "⏳ Waiting for sync…" : "No TELA apps discovered yet — connect a node";
  }

  async function loadSearchSCIDs() {
    const token = ++loadToken;
    fullLoadInProgress = true;
    resultsLoaded = false;
    try {
      setStatus("⏳ Loading apps...");
      resultsEl.replaceChildren();

      try {
        const disc = await fetch("/api/tela/discover");
        const data = await disc.json();
        if (token !== loadToken) return;
        if (data?.ok && data?.result?.apps) {
          allResults = data.result.apps.map(app => ({
            scid: app.scid,
            dURL: app.durl,
            nameHdr: app.name,
            descrHdr: app.descrHdr || "",
            iconURL: app.iconURL || "",
            likes: 0, dislikes: 0, average: 0, createdHeight: app.install_height || 0,
            ratingsLoaded: false,
            from_api: app.from_api
          }));
          fuse = new Fuse(allResults, {
            keys: ["scid", "dURL", "nameHdr", "descrHdr"],
            threshold: 0.25,
            ignoreLocation: true
          });
          refreshResults();
          setStatus(`✅ Loaded ${allResults.length} apps (direct)`);
        }
      } catch (e) {
        console.warn("Direct discovery failed", e);
      }

      if (token !== loadToken) return;

      let telaApps = [];
      try {
        const resp = await fetch(`${apiBase}/tela`);
        if (resp.ok) {
          const data = await resp.json();
          telaApps = data.tela_apps || [];
        }
      } catch (e) {
        console.warn("API metadata fetch failed", e);
      }

      if (token !== loadToken) return;

      if (telaApps.length > 0) {
        const metaMap = new Map(telaApps.map(a => [a.scid, a]));
        allResults.forEach(r => {
          const meta = metaMap.get(r.scid);
          if (meta) {
            r.nameHdr = meta.name || r.nameHdr;
            r.descrHdr = meta.description || r.descrHdr;
            r.dURL = meta.durl || r.dURL;
            r.from_api = true;
          }
        });
        for (const a of telaApps) {
          if (!allResults.some(r => r.scid === a.scid)) {
            allResults.push({
              scid: a.scid, dURL: a.durl, nameHdr: a.name,
              descrHdr: a.description || "", iconURL: "",
              likes: 0, dislikes: 0, average: 0, createdHeight: a.install_height || 0,
              ratingsLoaded: false, from_api: true
            });
          }
        }
        fuse.setCollection(allResults);
        setStatus(`✅ Loaded ${allResults.length} apps`);
        refreshResults();
      }

      if (token !== loadToken) return;

      const scids = allResults.map(r => r.scid);
      if (scids.length > 0) {
        let index = 0;
        const concurrency = 5;
        async function worker() {
          while (index < scids.length) {
            const scid = scids[index++];
            if (token !== loadToken) return;
            const res = await fetchSCIDData(scid);
            if (res) {
              const idx = allResults.findIndex(r => r.scid === scid);
              if (idx >= 0) {
                Object.assign(allResults[idx], res);
                allResults[idx].ratingsLoaded = true;
              }
            }
            if (token === loadToken) {
              setStatus(`Ratings ${Math.min(index, scids.length)} / ${scids.length}...`);
            }
          }
        }
        await Promise.all(Array(concurrency).fill().map(worker));
        fuse.setCollection(allResults);
        setStatus(`✅ Loaded ${allResults.length} apps`);
        refreshResults();
      }

      if (token !== loadToken) return;

      const missing = allResults.filter(r => r.likes === 0 && r.dislikes === 0 && r.average === 0);
      if (missing.length > 0) {
        setStatus(`Fallback ratings (daemon) ${missing.length} apps...`);
        try {
          const resp = await fetch("/api/tela/vars", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ scids: missing.map(r => r.scid) })
          });
          const data = await resp.json();
          if (data?.ok && data?.result?.vars) {
            for (const v of data.result.vars) {
              const idx = allResults.findIndex(r => r.scid === v.scid);
              if (idx >= 0) {
                if (v.likes > 0 || v.dislikes > 0 || v.average > 0) {
                  Object.assign(allResults[idx], {
                    likes: v.likes, dislikes: v.dislikes,
                    average: v.average, createdHeight: v.createdHeight
                  });
                }
                allResults[idx].ratingsLoaded = true;
              }
            }
            fuse.setCollection(allResults);
            setStatus(`✅ Loaded ${allResults.length} apps`);
            refreshResults();
          }
        } catch (e) {
          console.warn("Daemon RPC fallback failed", e);
        }
      }
      resultsLoaded = true;
      lastFullLoadCount = allResults.length;

      // Surface newly-found SCIDs since the last visit + refresh latest finds.
      detectNewSCIDs(allResults.map(r => ({ scid: r.scid })));
      updateLatestFinds();

      // Honest status: an empty catalog usually means "still syncing", not done.
      if (statusEl) {
        setStatus(allResults.length > 0
          ? `✅ Loaded ${allResults.length} apps`
          : waitingStatusText());
      }
    } catch (err) {
      if (token !== loadToken) return;
      console.error("Error loading SCIDs:", err);
      setStatus("❌ Failed loading apps – is HyperGnomon running?");
      const retry = document.createElement("button");
      retry.className = "retry-btn";
      retry.textContent = "↻ Retry";
      retry.onclick = () => { retry.remove(); loadSearchSCIDs(); };
      statusEl.appendChild(retry);
    } finally {
      fullLoadInProgress = false;
    }
  }

  // Lightweight catalog refresh: re-reads the discovered app list (+ metadata)
  // WITHOUT the per-app ratings fetch. Keeps the Search page live during first
  // sync / live discovery so the user never has to refresh to see content.
  async function refreshFromCatalog() {
    if (fullLoadInProgress) return; // a full load is already doing this work
    const token = ++loadToken;
    try {
      const disc = await fetch("/api/tela/discover");
      if (token !== loadToken) return;
      const data = await disc.json();
      if (!data?.ok || !data?.result?.apps) return;
      if (token !== loadToken) return;

      const apps = data.result.apps;
      const current = new Map(allResults.map(r => [r.scid, r]));

      // Early exit: the catalog set is unchanged — nothing new to show.
      if (resultsLoaded && apps.length === current.size && apps.every(a => current.has(a.scid))) {
        return;
      }

      let metaMap = new Map();
      try {
        const resp = await fetch(`${apiBase}/tela`);
        if (resp.ok) {
          const md = await resp.json();
          (md.tela_apps || []).forEach(a => metaMap.set(a.scid, a));
        }
      } catch (e) { /* metadata is optional */ }
      if (token !== loadToken) return;

      const merged = new Map();
      apps.forEach(app => {
        const prev = current.get(app.scid);
        const meta = metaMap.get(app.scid);
        const entry = prev ? { ...prev } : {
          scid: app.scid, dURL: app.durl, nameHdr: app.name,
          descrHdr: app.descrHdr || "", iconURL: app.iconURL || "",
          likes: 0, dislikes: 0, average: 0,
          createdHeight: app.install_height || 0,
          ratingsLoaded: false, from_api: app.from_api
        };
        if (meta) {
          entry.nameHdr = meta.name || entry.nameHdr;
          entry.descrHdr = meta.description || entry.descrHdr;
          entry.dURL = meta.durl || entry.dURL;
          entry.from_api = true;
        }
        merged.set(app.scid, entry);
      });
      // Merge API-only entries that discovery may have missed.
      metaMap.forEach((meta, scid) => {
        if (merged.has(scid)) return;
        merged.set(scid, {
          scid, dURL: meta.durl || scid, nameHdr: meta.name || scid,
          descrHdr: meta.description || "", iconURL: "",
          likes: 0, dislikes: 0, average: 0,
          createdHeight: meta.install_height || 0,
          ratingsLoaded: false, from_api: true
        });
      });

      if (token !== loadToken) return;
      allResults = [...merged.values()];
      fuse.setCollection(allResults);
      refreshResults();
      detectNewSCIDs(allResults.map(r => ({ scid: r.scid })));
      updateLatestFinds();
      if (statusEl) {
        setStatus(allResults.length > 0
          ? `✅ ${allResults.length} TELA app${allResults.length !== 1 ? "s" : ""}`
          : waitingStatusText());
      }
    } catch (e) {
      console.warn("Catalog refresh failed", e);
    }
  }

  function scheduleCatalogRefresh() {
    if (catalogRefreshTimer) return;
    catalogRefreshTimer = setTimeout(() => {
      catalogRefreshTimer = null;
      refreshFromCatalog();
    }, 2500);
  }

  function retryIfIncomplete() {
    const missing = allResults.filter(r => !r.dURL || r.dURL === r.scid);
    if (missing.length > allResults.length * 0.3 && metadataRetries < 3) {
      metadataRetries++;
      setTimeout(async () => {
        await loadSearchSCIDs();
        retryIfIncomplete();
      }, 2000 * metadataRetries);
    }
  }

  window.loadSearchSCIDs = loadSearchSCIDs;

  function getSortValue() {
    const sel = document.querySelector("#sortMode .selected");
    return sel?.getAttribute("data-value") || "top_rated";
  }

  function initSortDropdown() {
    const select = document.getElementById("sortMode");
    if (!select) return;
    const trigger = select.querySelector(".custom-select-trigger");
    const menu = select.querySelector(".custom-select-menu");
    const textEl = select.querySelector(".custom-select-text");
    if (!trigger || !menu || !textEl) return;
    trigger.addEventListener("click", (e) => {
      e.stopPropagation();
      select.classList.toggle("open");
    });
    menu.querySelectorAll("li").forEach((li) => {
      li.addEventListener("click", () => {
        menu.querySelectorAll("li").forEach((l) => l.classList.remove("selected"));
        li.classList.add("selected");
        textEl.textContent = li.textContent;
        select.classList.remove("open");
        refreshResults();
      });
    });
    document.addEventListener("click", () => { select.classList.remove("open"); });
  }

  function sortResults(list, mode) {
    const arr = [...list];
    switch (mode) {
      case "top_rated": return arr.sort((a, b) => { if (b.average !== a.average) return b.average - a.average; return b.likes - a.likes; });
      case "name_asc": return arr.sort((a, b) => a.nameHdr.localeCompare(b.nameHdr, undefined, { sensitivity: "base" }));
      case "name_desc": return arr.sort((a, b) => b.nameHdr.localeCompare(a.nameHdr, undefined, { sensitivity: "base" }));
      case "newest": return arr.sort((a, b) => b.createdHeight - a.createdHeight);
      case "oldest": return arr.sort((a, b) => a.createdHeight - b.createdHeight);
      default: return arr;
    }
  }

  function showCleanState() {
    resultsEl.replaceChildren();
    setStatus("");
  }

  function refreshResults() {
    const query = searchBox.value.trim();
    if (query) {
      runSearch(query);
    } else if (showAllSCIDs) {
      renderResults(allResults);
    } else {
      showCleanState();
    }
    updateCardsVisibility();
  }

  function renderResults(results) {
    resultsEl.replaceChildren();
    const hiddenExt = window.getHiddenExtensions?.() || [];
    const filtered = sortResults(results, getSortValue()).filter(r => {
      if (r.ratingsLoaded && r.average < minRating) return false;
      if (!r.dURL) return true;
      const url = r.dURL.toLowerCase();
      return !hiddenExt.some(ext => ext && url.endsWith(ext));
    });
    const totalCount = results.length;
    const filteredCount = filtered.length;
    // Always update status so the user sees the true total and filtered count,
    // never a stale value from an earlier loading phase.
    if (filteredCount < totalCount) {
      setStatus(`✅ ${totalCount} TELA apps · showing ${filteredCount}`);
    } else {
      setStatus(`✅ ${totalCount} TELA app${totalCount !== 1 ? 's' : ''}`);
    }
    if (!filtered.length) {
      const q = (searchBox.value || "").trim();
      const msg = document.createElement("div");
      msg.className = "no-results";
      const title = document.createElement("div");
      title.className = "no-results-title";
      title.textContent = q ? "No matches for “" + q + "” in the catalog" : "No results found";
      const hint = document.createElement("div");
      hint.className = "no-results-hint";
      hint.textContent = "try another query — scan the full SCID catalog";
      msg.append(title, hint);
      resultsEl.appendChild(msg);
      return;
    }
    filtered.forEach(r => {
      const row = UI.ResultRow(r, {
        onClick: handleSCIDClick,
        onBookmark: (scid) => {
          if (typeof window.toggleSearchBookmark === "function") window.toggleSearchBookmark(scid);
        },
        bookmarked: typeof window.isBookmarked === "function" && window.isBookmarked(r.scid)
      });
      resultsEl.appendChild(row);
      const descrEl = row.querySelector(".descr");
      if (descrEl) {
        const lh = parseFloat(getComputedStyle(descrEl).lineHeight) || 0;
        if (lh && Math.round(descrEl.scrollHeight / lh) > 1) {
          row.style.setProperty("--descr-hue", "var(--c-fuschia)");
        }
      }
    });
  }

  function handleSCIDClick(scid) {
    if (scidInput) { scidInput.value = scid; scidInput.dispatchEvent(new Event("input")); }
    const directLoad = typeof window.getDirectLoadSetting === "function" ? window.getDirectLoadSetting() : true;
    if (directLoad && loadBtn) loadBtn.click();
  }

  function renderSuggestions(results) {
    if (!searchSuggestions) return;
    searchSuggestions.innerHTML = "";
    suggestionResults = results.slice(0, 8);
    suggestionIndex = -1;
    if (!suggestionResults.length) { searchSuggestions.classList.add("hidden"); return; }
    suggestionResults.forEach((r) => {
      const item = UI.SearchSuggestion(r, {
        onSelect: (sel) => {
          handleSCIDClick(sel.scid);
          hideSuggestions();
        },
      });
      searchSuggestions.appendChild(item);
    });
    searchSuggestions.classList.remove("hidden");
    positionDropdown();
  }

  function hideSuggestions() {
    if (!searchSuggestions) return;
    searchSuggestions.classList.add("hidden"); suggestionIndex = -1;
  }

  function updateSuggestionSelection() {
    const items = searchSuggestions.querySelectorAll(".search-suggestion");
    items.forEach((el, i) => { el.classList.toggle("selected", i === suggestionIndex); if (i === suggestionIndex) el.scrollIntoView({ block: "nearest", behavior: "smooth" }); });
    if (suggestionIndex >= 0 && suggestionResults[suggestionIndex]) searchBox.value = suggestionResults[suggestionIndex].dURL;
    updateSearchClear();
  }

  function runSearch(value, { showResults = true } = {}) {
    const query = value.trim();
    if (!query) {
      if (showAllSCIDs) renderResults(allResults);
      else showCleanState();
      hideSuggestions();
      return;
    }
    if (!fuse) return;
    // Search always operates on the current allResults.  Because allResults can
    // grow asynchronously (metadata merge, WS events), we snapshot scids before
    // searching so we never show MORE results than the unfiltered allResults set
    // — a search/filter MUST reduce or equal the visible count, never increase it.
    const snapshotScids = new Set(allResults.map(r => r.scid));
    const results = fuse.search(query).map(r => r.item);
    // Deduplicate and clamp: only items that were in allResults at search time.
    const seen = new Set();
    const clamped = [];
    for (const r of results) {
      if (seen.has(r.scid)) continue;
      seen.add(r.scid);
      if (snapshotScids.has(r.scid)) clamped.push(r);
    }
    if (showResults) renderResults(clamped);
    renderSuggestions(getSuggestions(clamped, query));
  }

  /**
   * Orders autocomplete suggestions from search results.
   * The Fuse search matches scid, dURL, name AND description, so suggestions
   * now surface apps whose description (or name/scid) matches the query —
   * consistent with the full search. Exact dURL-prefix matches are kept on
   * top so typing a partial dURL still autocompletes it first.
   * @param {Array} results - Fuse-ranked search results
   * @param {string} query - User search input
   * @returns {Array} Reordered suggestion items
   */
  function getSuggestions(results, query) {
    const q = query.toLowerCase();
    const durlFirst = [];
    const rest = [];
    for (const r of results) {
      const d = (r.dURL || "").toLowerCase();
      if (d.startsWith(q)) durlFirst.push(r);
      else rest.push(r);
    }
    return durlFirst.concat(rest);
  }

  searchBox.addEventListener("input", e => { suggestionIndex = -1; updateSearchClear(); runSearch(e.target.value, { showResults: showAllSCIDs }); });

  function updateSearchClear() {
    if (!searchClear) return;
    const hasText = searchBox.value.length > 0;
    searchClear.classList.toggle("active", hasText);
    searchBox.classList.toggle("has-clear", hasText);
  }

  if (searchClear) {
    searchClear.addEventListener("click", (e) => {
      e.preventDefault();
      searchBox.value = "";
      updateSearchClear();
      runSearch("");
      hideSuggestions();
      searchBox.focus();
    });
  }

  searchBox.addEventListener("keydown", e => {
    const items = searchSuggestions.querySelectorAll(".search-suggestion");
    if (e.key === "ArrowDown") {
      if (items.length) { e.preventDefault(); suggestionIndex = suggestionIndex < items.length - 1 ? suggestionIndex + 1 : 0; updateSuggestionSelection(); }
    }
    if (e.key === "ArrowUp") {
      if (items.length) { e.preventDefault(); suggestionIndex = suggestionIndex > 0 ? suggestionIndex - 1 : items.length - 1; updateSuggestionSelection(); }
    }
    if (e.key === "Enter") {
      const selected = suggestionResults[suggestionIndex];
      if (selected) { e.preventDefault(); handleSCIDClick(selected.scid); hideSuggestions(); return; }
      // No suggestion selected: run search and show results
      runSearch(searchBox.value, { showResults: true });
      hideSuggestions();
    }
  });

  document.addEventListener("click", (e) => {
    if (!searchBox.contains(e.target) && !searchSuggestions.contains(e.target)) hideSuggestions();
  });
  searchBox.addEventListener("blur", () => { setTimeout(hideSuggestions, 150); });

  // Show/hide bottom cards on search focus
  function updateCardsVisibility() {
    if (!searchCardsEl) return;
    const enabled = typeof window.getShowSearchCardsSetting === "function" ? window.getShowSearchCardsSetting() : true;
    if (!enabled) { searchCardsEl.classList.add("hidden-cards"); return; }
    const hasFocus = document.activeElement === searchBox;
    const hasResults = resultsEl.children.length > 0;
    searchCardsEl.classList.toggle("hidden-cards", hasFocus || showAllSCIDs || hasResults);
  }

  searchBox.addEventListener("focus", () => { updateCardsVisibility(); positionDropdown(); });
  searchBox.addEventListener("blur", () => setTimeout(updateCardsVisibility, 150));
  searchBox.addEventListener("input", () => { updateCardsVisibility(); positionDropdown(); });
  window.addEventListener("resize", positionDropdown);
  document.getElementById("content")?.addEventListener("scroll", positionDropdown);

  minRatingEl?.addEventListener("input", e => { minRating = Number(e.target.value); minRatingVal.textContent = minRating; refreshResults(); });

  initSortDropdown();

  // Render the Latest Finds card immediately (placeholder until data arrives)
  // so the 3-card layout is stable from the very first paint.
  updateLatestFinds();

  // Refresh result bookmark stars when bookmarks change
  document.addEventListener("bookmarksChanged", () => { refreshResults(); });

  if (showAllToggle) {
    showAllToggle.addEventListener("change", () => {
      showAllSCIDs = showAllToggle.checked;
      refreshResults();
    });
  }

  document.addEventListener("pageChanged", async (e) => {
    if (e.detail.page === "search") {
      metadataRetries = 0;
      updateLatestFinds();
      // Reload whenever the catalog is empty or grew since the last full load
      // — never show a stale (e.g. empty-on-install) cached state on activation.
      if (!resultsLoaded || allResults.length === 0 || allResults.length !== lastFullLoadCount) {
        await loadSearchSCIDs();
      } else {
        refreshResults();
      }
    }
  });

  document.addEventListener("nodeConnected", async () => {
    if (statusEl) setStatus("⏳ Loading apps...");
    searchBox.disabled = false;
    metadataRetries = 0;
    await loadSearchSCIDs();
    retryIfIncomplete();
  });

  (async () => {
    // Probe the router's own discovery endpoint (works offline via the on-disk
    // store/cache, and has no gnomon-port race). If apps already exist from a
    // previous session, load them immediately so the Search page is never
    // blank on first open.
    try {
      const resp = await fetch("/api/tela/discover");
      if (!resp.ok) return;
      const data = await resp.json();
      if (data?.ok && data?.result?.apps?.length > 0) {
        searchBox.disabled = false;
        await loadSearchSCIDs();
      }
    } catch {}
  })();

  // Listen for new SCIDs discovered during Gnomon sync (via WS)
  document.addEventListener("wsEvent", (e) => {
    const msg = e.detail;
    if (msg.event === "tip_synced") {
      // Full reload (incl. ratings) once sync catches the tip. Re-armed on
      // every connect/reconnect by the backend, so this is a reliable signal.
      loadSearchSCIDs();
      return;
    }
    if (msg.event === "catalog_progress") {
      // Fires every ~2s while the indexer discovers apps: refresh the visible
      // catalog live (throttled) so Search fills in during first sync without
      // a manual refresh. Skip when nothing is happening yet.
      if (resultsLoaded || allResults.length > 0 || msg.total > 0) {
        scheduleCatalogRefresh();
      }
      return;
    }
    if (msg.event === "new_tela_app" && msg.scid) {
      // Lightweight: let the throttled catalog refresh pick up the new app —
      // no per-event discover scan (avoids O(N²) churn during FastSync bursts).
      scheduleCatalogRefresh();
    }
  });

  // ===================== LIVE INFO CARD (Updates + RSS) =====================
  
  // Live info card elements
  const liveInfoCard = document.getElementById("live-info-card");
  const updateBanner = document.getElementById("update-banner");
  const updateCurrentMsg = document.getElementById("update-current-message");
  const updateLatestVersionEl = document.getElementById("update-latest-version");
  const updateCurrentVersionEl = document.getElementById("update-current-version");
  const currentVersionDisplayEl = document.getElementById("current-version-display");
  const updateViewBtn = document.getElementById("update-view-btn");
  const updateDismissBtn = document.getElementById("update-dismiss-btn");
  const liveInfoRefreshBtn = document.getElementById("live-info-refresh");
  const rssFeedList = document.getElementById("rss-feed-list");
  const rssFeedTitle = document.getElementById("rss-feed-title");
  const rssFeedUpdated = document.getElementById("rss-feed-updated");
  
  // State
  let currentAppVersion = "0.13.0";
  let rssFeedUrl = "https://dero.world/anotherworld/feed/";
  let updateCheckEnabled = true;
  let rssRefreshInterval = null;
  let dismissedUpdateVersion = null;
  
  // Load settings from global settings (synced with config.json via settings page)
  function loadLiveInfoSettings() {
    if (typeof window.getRSSFeedUrlSetting === "function") {
      rssFeedUrl = window.getRSSFeedUrlSetting();
    }
    if (typeof window.getCheckUpdatesSetting === "function") {
      updateCheckEnabled = window.getCheckUpdatesSetting();
    }
    // Load dismissed version from localStorage (persists across sessions)
    try {
      const stored = JSON.parse(localStorage.getItem("hyperwolf.liveInfoSettings") || "{}");
      dismissedUpdateVersion = stored.dismissedUpdateVersion || null;
    } catch (e) {}
  }
  
  function saveLiveInfoSettings() {
    try {
      localStorage.setItem("hyperwolf.liveInfoSettings", JSON.stringify({
        rssFeedUrl,
        updateCheckEnabled,
        dismissedUpdateVersion
      }));
    } catch (e) {}
  }
  
  // Format relative time (e.g., "2h ago", "1d ago")
  function formatRelativeTime(dateStr) {
    if (!dateStr) return "";
    const date = new Date(dateStr);
    if (isNaN(date.getTime())) return dateStr;
    const now = new Date();
    const diffMs = now - date;
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    
    if (diffMins < 1) return "just now";
    if (diffMins < 60) return `${diffMins}m ago`;
    if (diffHours < 24) return `${diffHours}h ago`;
    if (diffDays < 7) return `${diffDays}d ago`;
    return date.toLocaleDateString();
  }
  
  // Sanitize HTML to prevent XSS
  function sanitizeHtml(text) {
    const div = document.createElement("div");
    div.textContent = text;
    return div.innerHTML;
  }
  
  // Render RSS feed items
  function renderRSSFeed(items, feedTitle, feedUpdated) {
    if (!rssFeedList) return;
    
    if (!items || items.length === 0) {
      rssFeedList.innerHTML = '<div class="rss-empty">No feed items found</div>';
      return;
    }
    
    // Show up to 5 items (scrollable inside the one-item window)
    const displayItems = items.slice(0, 5);
    
    rssFeedList.innerHTML = "";
    displayItems.forEach(item => {
      const div = document.createElement("div");
      div.className = "rss-item";
      div.innerHTML = `
        <span class="rss-item-title">${sanitizeHtml(item.title)}</span>
        <div class="rss-item-meta">
          <span class="rss-item-source">${sanitizeHtml(item.source || feedTitle)}</span>
          <span class="rss-item-date">${formatRelativeTime(item.pub_date)}</span>
        </div>
      `;
      div.onclick = () => {
        if (item.link) {
          window.open(item.link, "_blank", "noopener,noreferrer");
        }
      };
      div.style.cursor = "pointer";
      rssFeedList.appendChild(div);
    });
    
    if (rssFeedTitle) rssFeedTitle.textContent = feedTitle || "📰 Dero.World AnotherWorld";
    if (rssFeedUpdated) rssFeedUpdated.textContent = feedUpdated ? formatRelativeTime(feedUpdated) : "";
  }
  
  // Load RSS feed
  async function loadRSSFeed() {
    if (!rssFeedList) return;
    
    rssFeedList.innerHTML = '<div class="rss-loading">Loading feed...</div>';
    
    try {
      const resp = await fetch(`/api/rss?url=${encodeURIComponent(rssFeedUrl)}`);
      const data = await resp.json();
      
      if (data.ok && data.result) {
        renderRSSFeed(data.result.items, data.result.title, data.result.updated);
      } else {
        rssFeedList.innerHTML = `<div class="rss-error">Failed to load: ${data.error || "Unknown error"}</div>`;
      }
    } catch (err) {
      console.error("RSS feed load error:", err);
      rssFeedList.innerHTML = '<div class="rss-error">Failed to load feed</div>';
    }
  }
  
  // Check for application updates
  async function checkForUpdates() {
    if (!updateCheckEnabled) {
      showUpdateCurrent();
      return;
    }
    
    try {
      const resp = await fetch("/api/update-check");
      const data = await resp.json();
      
      if (data.ok && data.result) {
        const result = data.result;
        currentAppVersion = result.current_version;
        
        if (result.update_available && result.latest_version !== dismissedUpdateVersion) {
          showUpdateBanner(result);
        } else {
          showUpdateCurrent();
        }
      } else {
        // Failed to check - silently show current version
        showUpdateCurrent();
      }
    } catch (err) {
      console.error("Update check error:", err);
      showUpdateCurrent();
    }
  }
  
  function showUpdateBanner(result) {
    if (updateBanner) updateBanner.classList.remove("hidden");
    if (updateCurrentMsg) updateCurrentMsg.classList.add("hidden");
    if (updateLatestVersionEl) updateLatestVersionEl.textContent = result.latest_version;
    if (updateCurrentVersionEl) updateCurrentVersionEl.textContent = result.current_version;
    
    // Store current version for dismissal comparison
    currentAppVersion = result.current_version;
  }
  
  function showUpdateCurrent() {
    if (updateBanner) updateBanner.classList.add("hidden");
    if (updateCurrentMsg) updateCurrentMsg.classList.remove("hidden");
    if (currentVersionDisplayEl) currentVersionDisplayEl.textContent = currentAppVersion;
  }
  
  // Dismiss update notification
  function dismissUpdate() {
    dismissedUpdateVersion = currentAppVersion;
    saveLiveInfoSettings();
    showUpdateCurrent();
  }
  
  // Initialize live info card
  function initLiveInfoCard() {
    loadLiveInfoSettings();
    
    // Set up event listeners
    if (updateDismissBtn) {
      updateDismissBtn.onclick = dismissUpdate;
    }
    
    if (updateViewBtn) {
      updateViewBtn.onclick = () => {
        window.open("https://github.com/ArcaneSphere/HyperWolf/releases", "_blank", "noopener,noreferrer");
      };
    }
    
    if (liveInfoRefreshBtn) {
      liveInfoRefreshBtn.onclick = () => {
        checkForUpdates();
        loadRSSFeed();
      };
    }
    
    // Initial load
    checkForUpdates();
    loadRSSFeed();
    
    // Set up periodic refresh (every 10 minutes)
    rssRefreshInterval = setInterval(() => {
      if (document.visibilityState === "visible") {
        loadRSSFeed();
        checkForUpdates();
      }
    }, 10 * 60 * 1000);
    
    // Refresh on page visibility change
    document.addEventListener("visibilitychange", () => {
      if (document.visibilityState === "visible") {
        loadRSSFeed();
        checkForUpdates();
      }
    });
    
    // Refresh when search page becomes active
    document.addEventListener("pageChanged", (e) => {
      if (e.detail.page === "search") {
        loadRSSFeed();
        checkForUpdates();
      }
    });
  }
  
  // Initialize when DOM is ready
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initLiveInfoCard);
  } else {
    initLiveInfoCard();
  }
  
  // Expose for settings page to update
  window.updateLiveInfoSettings = function(settings) {
    if (settings.rssFeedUrl !== undefined) rssFeedUrl = settings.rssFeedUrl;
    if (settings.updateCheckEnabled !== undefined) updateCheckEnabled = settings.updateCheckEnabled;
    saveLiveInfoSettings();
    loadRSSFeed();
    checkForUpdates();
  };

})();
