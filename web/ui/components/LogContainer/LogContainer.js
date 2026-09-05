/* ============================================================
   LogContainer — the live log view: filter controls + action
   buttons + auto-follow checkbox + scrollable terminal panel.
   Pure builder, no DOM side effects. Populates window.UI.
   The log page ships a static shell in index.html with these exact
   ids; dashboard.js wires behaviour on DOMContentLoaded. This
   builder reproduces that shell for reuse.
   Styling: LogContainer.css (single source of truth).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {Object=} opts
   * @param {string=} opts.containerId - id for the terminal panel (default "log-container")
   * @param {boolean=} opts.follow - initial auto-follow state (default true)
   * @param {Array.<Node>=} opts.children - initial log rows (UI.LogEntry output)
   * @returns {{element: HTMLElement, containerEl: HTMLElement, controlsEl: HTMLElement, selectEl: HTMLElement}}
   */
  UI.LogContainer = function LogContainer(opts = {}) {
    const wrap = document.createElement("div");

    const controls = document.createElement("div");
    controls.className = "log-controls";

    const filter = document.createElement("div");
    filter.className = "log-filter-group";
    const label = document.createElement("label");
    label.htmlFor = "log-level-filter";
    label.textContent = "Filter by level:";
    const select = document.createElement("select");
    select.id = "log-level-filter";
    select.className = "select-mini";
    ["", "error", "warn", "info", "debug", "trace"].forEach((v) => {
      const option = document.createElement("option");
      option.value = v;
      option.textContent = v || "All levels";
      select.appendChild(option);
    });
    filter.append(label, select);

    const actions = document.createElement("div");
    actions.className = "log-action-group";
    const refresh = document.createElement("button");
    refresh.id = "log-refresh-btn";
    refresh.className = "small";
    refresh.textContent = "⟳ Refresh";
    const clear = document.createElement("button");
    clear.id = "log-clear-btn";
    clear.className = "small secondary";
    clear.textContent = "Clear";
    actions.append(refresh, clear);

    controls.append(filter, actions);

    const auto = document.createElement("div");
    auto.className = "log-auto-scroll";
    const check = document.createElement("input");
    check.type = "checkbox";
    check.id = "log-auto-scroll";
    check.checked = opts.follow !== false;
    const autoLabel = document.createElement("label");
    autoLabel.htmlFor = "log-auto-scroll";
    autoLabel.textContent = "Auto-follow latest";
    auto.append(check, autoLabel);

    const container = document.createElement("div");
    container.className = "log-container";
    container.id = opts.containerId || "log-container";
    (opts.children || []).forEach((c) => container.appendChild(c));

    wrap.append(controls, auto, container);
    return {
      element: wrap,
      containerEl: container,
      controlsEl: controls,
      selectEl: select,
    };
  };
})();