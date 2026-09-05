/* ============================================================
   SyncProgress — chain-sync progress block (percent label, thin
   progress bar, ETA label). Pure builder, no DOM side effects.
   Populates window.UI. The server page ships a static sync block
   (id="sync-info") inside the Sync status card in index.html;
   the builder is the migration target (and a canonical API for
   rendering the same block elsewhere).
   Styling: SyncProgress.css (single source of truth).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {Object=} opts
   * @param {string=} opts.id - id for the wrapper (default "sync-info")
   * @param {string=} opts.labelId - id for the state label (default "sync-label")
   * @param {string=} opts.barId - id for the fill element (default "sync-bar")
   * @param {string=} opts.etaId - id for the ETA label (default "sync-eta")
   * @param {string|Node=} opts.label - initial state text (default "Waiting...")
   * @param {string|Node=} opts.eta - initial ETA text
   * @param {number=} opts.percent - 0-100 fill; -1 starts in indeterminate mode
   * @returns {{element: HTMLElement, infoEl: HTMLElement, labelEl: HTMLElement, barEl: HTMLElement, etaEl: HTMLElement, setPercent: function(number): void, setLabel: function(string): void, setEta: function(string): void, setIndeterminate: function(boolean): void}}
   */
  UI.SyncProgress = function SyncProgress(opts = {}) {
    const infoEl = document.createElement("div");
    infoEl.id = opts.id || "sync-info";
    infoEl.className = "sync-info";

    const labelEl = document.createElement("div");
    labelEl.id = opts.labelId || "sync-label";
    labelEl.className = "sync-label";
    if (opts.label != null) {
      if (opts.label.nodeType) labelEl.appendChild(opts.label);
      else labelEl.textContent = String(opts.label);
    }

    const barWrap = document.createElement("div");
    barWrap.className = "progress-bar";
    const barEl = document.createElement("div");
    barEl.id = opts.barId || "sync-bar";
    barEl.className = "progress-fill";
    barEl.style.width = "0%";
    barWrap.appendChild(barEl);

    const etaEl = document.createElement("div");
    etaEl.id = opts.etaId || "sync-eta";
    etaEl.className = "sync-label";
    if (opts.eta != null) {
      if (opts.eta.nodeType) etaEl.appendChild(opts.eta);
      else etaEl.textContent = String(opts.eta);
    }

    infoEl.append(labelEl, barWrap, etaEl);

    const api = {
      element: infoEl,
      infoEl,
      labelEl,
      barEl,
      etaEl,
      setPercent(pct) {
        barEl.classList.remove("indeterminate");
        barEl.style.width = Math.max(0, Math.min(100, pct)).toFixed(1) + "%";
      },
      setLabel(text) { labelEl.textContent = text; },
      setEta(text) { etaEl.textContent = text; },
      setIndeterminate(on) {
        if (on) { barEl.classList.add("indeterminate"); barEl.style.width = ""; }
        else { barEl.classList.remove("indeterminate"); }
      },
    };

    if (typeof opts.percent === "number" && opts.percent === -1) api.setIndeterminate(true);
    else if (typeof opts.percent === "number") api.setPercent(opts.percent);

    return api;
  };
})();