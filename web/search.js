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

  if (!searchBox || !resultsEl) return;

  let apiBase = "http://127.0.0.1:18082/api";

  (async function initApiBase() {
    try {
      const resp = await fetch("/api/config");
      const data = await resp.json();
      if (data?.ok && data?.result?.gnomon_api_port) {
        apiBase = "http://127.0.0.1:" + data.result.gnomon_api_port + "/api";
      }
    } catch (e) {}
  })();

  let allResults = [];
  let fuse       = null;
  let minRating  = 30;
  let loadToken  = 0;
  let resultsLoaded = false;
  let metadataRetries = 0;

  const searchSuggestions = document.getElementById("searchSuggestions");

  let suggestionResults = [];
  let suggestionIndex = -1;

  searchBox.parentElement.style.position = "relative";

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

  async function loadSearchSCIDs() {
    const token = ++loadToken;
    resultsLoaded = false;
    try {
      statusEl.textContent = "⏳ Loading apps...";
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
          renderResults(allResults);
          statusEl.textContent = `✅ Loaded ${allResults.length} apps (direct)`;
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
        statusEl.textContent = `✅ Loaded ${allResults.length} apps`;
        renderResults(allResults);
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
              statusEl.textContent = `Ratings ${Math.min(index, scids.length)} / ${scids.length}...`;
            }
          }
        }
        await Promise.all(Array(concurrency).fill().map(worker));
        fuse.setCollection(allResults);
        statusEl.textContent = `✅ Loaded ${allResults.length} apps`;
        renderResults(allResults);
      }

      if (token !== loadToken) return;

      const missing = allResults.filter(r => r.likes === 0 && r.dislikes === 0 && r.average === 0);
      if (missing.length > 0) {
        statusEl.textContent = `Fallback ratings (daemon) ${missing.length} apps...`;
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
            statusEl.textContent = `✅ Loaded ${allResults.length} apps`;
            renderResults(allResults);
          }
        } catch (e) {
          console.warn("Daemon RPC fallback failed", e);
        }
      }
      resultsLoaded = true;
    } catch (err) {
      if (token !== loadToken) return;
      console.error("Error loading SCIDs:", err);
      statusEl.textContent = "❌ Failed loading apps – is HyperGnomon running?";
      const retry = document.createElement("button");
      retry.className = "retry-btn";
      retry.textContent = "↻ Retry";
      retry.onclick = () => { retry.remove(); loadSearchSCIDs(); };
      statusEl.appendChild(retry);
    }
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
        renderResults(allResults);
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

  function createHexIcon() {
    const div = document.createElement("div");
    div.className = "scid-svg";
    div.innerHTML = `
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 867 1001">
        <polygon points="0.5,250.55 433.47,0.58 866.43,250.55 866.43,750.5 433.47,1000.47 0.5,750.5" fill="none" stroke="currentColor" stroke-width="6"/>
        <polygon points="209.17,371.63 209.17,628.97 433.69,759.79 657.39,630.28 657.39,374.85 433.26,241.71 209.17,371.63" fill="none" stroke="currentColor" stroke-width="6"/>
        <polygon points="239.64,389.3 239.64,611.21 348.32,675.69 366.79,580 331.72,558.65 331.72,442.81 433.41,384.91 533.88,443.47 533.88,559.84 498.24,579.73 515.93,678.24 626.31,612.4 626.31,392.17 433.26,277.45 239.64,389.3" fill="none" stroke="currentColor" stroke-width="6"/>
        <polygon points="432.54,420.32 502.73,461.22 502.73,542 464.66,563.58 485.09,694.51 433.7,724.39 378.96,692.5 400.62,564.62 362.1,541.19 362.1,461.28 432.54,420.32" fill="none" stroke="currentColor" stroke-width="6"/>
      </svg>`;
    return div;
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
      statusEl.textContent = `✅ ${totalCount} TELA apps · showing ${filteredCount}`;
    } else {
      statusEl.textContent = `✅ ${totalCount} TELA app${totalCount !== 1 ? 's' : ''}`;
    }
    if (!filtered.length) {
      const msg = document.createElement("div"); msg.className = "no-results"; msg.textContent = "No results found";
      resultsEl.appendChild(msg);
      return;
    }
    filtered.forEach(r => {
      const div = document.createElement("div"); div.className = "result";
      div.onclick = (e) => { div.classList.add("clicking"); setTimeout(() => div.classList.remove("clicking"), 500); handleSCIDClick(r.scid); };
      const iconSlot = document.createElement("div"); iconSlot.className = "icon-slot";
      if (r.iconURL) {
        const img = document.createElement("img"); img.className = "icon"; img.src = r.iconURL;
        img.onerror = () => iconSlot.replaceChildren(createHexIcon());
        iconSlot.appendChild(img);
      } else { iconSlot.appendChild(createHexIcon()); }
      const content = document.createElement("div"); content.className = "content";
      const urlEl = document.createElement("div"); urlEl.className = "url"; urlEl.textContent = r.dURL;
      const nameEl = document.createElement("div"); nameEl.className = "nameHdr"; nameEl.textContent = r.nameHdr;
      const scidEl = document.createElement("div"); scidEl.className = "scid"; scidEl.textContent = r.scid;
      const descrEl = document.createElement("div"); descrEl.className = "descr"; descrEl.textContent = r.descrHdr;
      const ratingEl = document.createElement("div"); ratingEl.className = "rating";
      ratingEl.textContent = r.ratingsLoaded ? `👍 ${r.likes} 👎 ${r.dislikes} ⭐ ${r.average}` : "—";
      [urlEl, nameEl, scidEl].forEach(el => {
        el.style.cursor = "pointer";
        el.onclick = (e) => { e.stopPropagation(); handleSCIDClick(r.scid); };
      });
      content.append(urlEl, nameEl, scidEl, descrEl, ratingEl);
      div.append(iconSlot, content);
      resultsEl.appendChild(div);
    });
  }

  function handleSCIDClick(scid) {
    if (scidInput) { scidInput.value = scid; scidInput.dispatchEvent(new Event("input")); }
    const directLoad = typeof window.getDirectLoadSetting === "function" ? window.getDirectLoadSetting() : true;
    if (directLoad && loadBtn) loadBtn.click();
  }

  function getDURLSuggestions(input) {
    const q = input.trim().toLowerCase();
    if (!q) return [];
    return allResults.filter(r => (r.dURL || "").toLowerCase().startsWith(q))
      .sort((a, b) => a.dURL.length - b.dURL.length).slice(0, 8);
  }

  function renderSuggestions(results) {
    if (!searchSuggestions) return;
    searchSuggestions.innerHTML = "";
    suggestionResults = results.slice(0, 8);
    suggestionIndex = -1;
    if (!suggestionResults.length) { searchSuggestions.classList.add("hidden"); return; }
    suggestionResults.forEach((r, index) => {
      const item = document.createElement("div"); item.className = "search-suggestion";
      const durlEl = document.createElement("div"); durlEl.className = "durl"; durlEl.textContent = r.dURL;
      const metaEl = document.createElement("div"); metaEl.className = "meta"; metaEl.textContent = r.nameHdr || r.scid;
      item.append(durlEl, metaEl);
      item.addEventListener("mousedown", (e) => {
        e.preventDefault();
        handleSCIDClick(r.scid);
        hideSuggestions();
      });
      searchSuggestions.appendChild(item);
    });
    searchSuggestions.classList.remove("hidden");
  }

  function hideSuggestions() {
    if (!searchSuggestions) return;
    searchSuggestions.classList.add("hidden"); suggestionIndex = -1;
  }

  function updateSuggestionSelection() {
    const items = searchSuggestions.querySelectorAll(".search-suggestion");
    items.forEach((el, i) => { el.classList.toggle("selected", i === suggestionIndex); if (i === suggestionIndex) el.scrollIntoView({ block: "nearest", behavior: "smooth" }); });
    if (suggestionIndex >= 0 && suggestionResults[suggestionIndex]) searchBox.value = suggestionResults[suggestionIndex].dURL;
  }

  function runSearch(value) {
    const query = value.trim();
    if (!query) { renderResults(allResults); hideSuggestions(); return; }
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
    renderResults(clamped);
    renderSuggestions(clamped.filter(r => (r.dURL || "").toLowerCase().includes(query.toLowerCase())));
  }

  searchBox.addEventListener("input", e => { suggestionIndex = -1; runSearch(e.target.value); });

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
    }
  });

  document.addEventListener("click", (e) => {
    if (!searchBox.contains(e.target) && !searchSuggestions.contains(e.target)) hideSuggestions();
  });
  searchBox.addEventListener("blur", () => { setTimeout(hideSuggestions, 150); });

  minRatingEl?.addEventListener("input", e => { minRating = Number(e.target.value); minRatingVal.textContent = minRating; renderResults(allResults); });

  initSortDropdown();

  document.addEventListener("pageChanged", async (e) => {
    if (e.detail.page === "search") {
      metadataRetries = 0;
      if (!resultsLoaded) { await loadSearchSCIDs(); }
      else { renderResults(allResults); if (searchBox.value) runSearch(searchBox.value); }
    }
  });

  document.addEventListener("nodeConnected", async () => {
    if (statusEl) statusEl.textContent = "⏳ Loading apps...";
    searchBox.disabled = false;
    metadataRetries = 0;
    await loadSearchSCIDs();
    retryIfIncomplete();
  });

  (async () => {
    try {
      const resp = await fetch(`${apiBase}/tela`);
      if (!resp.ok) return;
      const data = await resp.json();
      if (data.count > 0) { searchBox.disabled = false; await loadSearchSCIDs(); }
    } catch {}
  })();

  // Listen for new SCIDs discovered during Gnomon sync (via WS)
  document.addEventListener("wsEvent", (e) => {
    const msg = e.detail;
    if (msg.event === "tip_synced") { loadSearchSCIDs(); return; }
    if (msg.event === "new_tela_app" && msg.scid) {
      (async () => {
        try {
          const disc = await fetch("/api/tela/discover");
          const data = await disc.json();
          if (!data?.ok || !data?.result?.apps) return;
          const match = data.result.apps.find(a => a.scid === msg.scid);
          if (!match) return;
          const entry = {
            scid: msg.scid, dURL: match.durl || msg.scid, nameHdr: match.name || msg.scid,
            descrHdr: match.descrHdr || "", iconURL: match.iconURL || "",
            likes: 0, dislikes: 0, average: 0, createdHeight: match.install_height || 0, ratingsLoaded: false, from_api: match.from_api
          };
          if (allResults.some(r => r.scid === msg.scid)) return;
          allResults.push(entry);
          fuse.setCollection(allResults);
          renderResults(allResults);
          if (statusEl) { const cnt = allResults.length; statusEl.textContent = `✅ Loaded ${cnt} app${cnt !== 1 ? "s" : ""} (direct)`; }
        } catch (e) { console.warn("Failed to add newly discovered SCID:", e); }
      })();
    }
  });
})();
