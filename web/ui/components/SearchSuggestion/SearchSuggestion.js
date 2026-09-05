/* ============================================================
   SearchSuggestion — one autocomplete row in the search dropdown.
   Pure builder, no DOM side effects. Populates window.UI.
   Styling: SearchSuggestion.css (mirrors style.css until visual baseline).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {Object} r — suggestion item
   * @param {string} r.dURL
   * @param {string=} r.nameHdr
   * @param {string=} r.scid
   * @param {Object=} handlers
   * @param {(r: Object) => void} handlers.onSelect — fired on mousedown (before click/blur)
   * @returns {HTMLElement} div.search-suggestion
   */
  UI.SearchSuggestion = function SearchSuggestion(r, handlers = {}) {
    const item = document.createElement("div");
    item.className = "search-suggestion";

    const durlEl = document.createElement("div");
    durlEl.className = "durl";
    durlEl.textContent = r.dURL;

    const metaEl = document.createElement("div");
    metaEl.className = "meta";
    metaEl.textContent = r.nameHdr || r.scid;

    item.append(durlEl, metaEl);

    item.addEventListener("mousedown", (e) => {
      if (!handlers.onSelect) return;
      e.preventDefault();
      handlers.onSelect(r);
    });

    return item;
  };
})();