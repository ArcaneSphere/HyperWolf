/* ============================================================
   SearchCard — bottom-panel card shell with header + body.
   Pure builder, no DOM side effects. Populates window.UI.
   The app currently ships three static .search-card shells in
   index.html; this builder is the canonical way to create new
   ones (and the migration target for the static shells).
   Styling: SearchCard.css (single source of truth).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {Object} opts
   * @param {string=} opts.className - extra modifier(s), e.g. "latest-finds"
   * @param {string=} opts.id - element id for the card root
   * @param {string} opts.title - header label
   * @param {(Node|Array.<Node>)=} opts.headerExtra - trailing header controls (badge, refresh button)
   * @param {string=} opts.bodyClass - extra modifier(s) for the body, e.g. "live-info-body"
   * @param {string=} opts.bodyId - element id for the body (e.g. latest-finds-list)
   * @param {(Node|Array.<Node>)=} opts.children - body content
   * @param {boolean=} opts.empty - render the empty-state "Coming soon" variant when no children
   * @returns {{element: HTMLElement, headerEl: HTMLElement, bodyEl: HTMLElement}}
   */
  UI.SearchCard = function SearchCard(opts = {}) {
    const el = document.createElement("div");
    el.className = "search-card" + (opts.className ? " " + opts.className : "");
    if (opts.id) el.id = opts.id;

    const header = document.createElement("div");
    header.className = "search-card-header";

    const title = document.createElement("span");
    title.textContent = opts.title || "";
    header.appendChild(title);
    if (opts.headerExtra) {
      const extras = Array.isArray(opts.headerExtra) ? opts.headerExtra : [opts.headerExtra];
      extras.forEach((x) => header.appendChild(x));
    }
    el.appendChild(header);

    const body = document.createElement("div");
    body.className = "search-card-body" + (opts.bodyClass ? " " + opts.bodyClass : "");
    if (opts.bodyId) body.id = opts.bodyId;

    const children = opts.children
      ? Array.isArray(opts.children) ? opts.children : [opts.children]
      : [];
    if (opts.empty && children.length === 0) {
      body.classList.add("search-card-empty");
    } else {
      children.forEach((c) => body.appendChild(c));
    }
    el.appendChild(body);

    return { element: el, headerEl: header, bodyEl: body };
  };
})();