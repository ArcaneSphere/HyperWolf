/* ============================================================
   StatusCard — dashboard status card with a header, label/value
   rows and an optional expandable <details> section.
   Pure builder, no DOM side effects. Populates window.UI.
   The server page ships a static .status-grid in index.html; this
   builder is the canonical way to create new cards (and the
   migration target for the static grid).
   Styling: StatusCard.css (single source of truth).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  function makeRow(r) {
    const row = document.createElement("div");
    row.className = "card-row";
    const label = document.createElement("span");
    label.className = "card-label";
    label.textContent = r.label;
    const value = document.createElement("span");
    value.className = "card-value" + (r.lg ? " card-value-lg" : "");
    if (r.valueId) value.id = r.valueId;
    value.textContent = r.value == null ? "—" : String(r.value);
    row.append(label, value);
    return row;
  }

  /**
   * @param {Object=} opts
   * @param {string=} opts.id - id for the card root
   * @param {string=} opts.className - extra modifier(s), e.g. "card-apps"
   * @param {string|Node} opts.header - card title
   * @param {Array.<{label: string, value?: string|number, lg?: boolean, valueId?: string}>=} opts.rows - label/value rows
   * @param {{summary: string, rows: Array.<{label: string, value?: string|number, lg?: boolean, valueId?: string}>}=} opts.expand - expandable details section
   * @param {Array.<Node>=} opts.children - extra nodes appended after rows (e.g. a sync progress block)
   * @returns {{element: HTMLElement, headerEl: HTMLElement, bodyEl: HTMLElement}}
   */
  UI.StatusCard = function StatusCard(opts = {}) {
    const el = document.createElement("div");
    el.className = "status-card" + (opts.className ? " " + opts.className : "");
    if (opts.id) el.id = opts.id;

    const header = document.createElement("div");
    header.className = "card-header";
    if (opts.header && opts.header.nodeType) header.appendChild(opts.header);
    else header.textContent = opts.header || "";
    el.appendChild(header);

    const body = document.createElement("div");
    body.className = "card-body";
    (opts.rows || []).forEach((r) => body.appendChild(makeRow(r)));
    if (opts.expand) {
      const details = document.createElement("details");
      details.className = "card-expand";
      const summary = document.createElement("summary");
      summary.textContent = opts.expand.summary;
      details.appendChild(summary);
      (opts.expand.rows || []).forEach((r) => details.appendChild(makeRow(r)));
      body.appendChild(details);
    }
    (opts.children || []).forEach((c) => body.appendChild(c));
    el.appendChild(body);

    return { element: el, headerEl: header, bodyEl: body };
  };
})();