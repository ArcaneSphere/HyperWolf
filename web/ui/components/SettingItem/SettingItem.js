/* ============================================================
   SettingItem — one settings card (label, optional description,
   body content). Two layouts mirror the settings page markup:
   "stack" puts the label on top followed by body content and a
   trailing description; "row" renders label + description on the
   left with a control (typically a toggle) on the right.
   Pure builder, no DOM side effects. Populates window.UI.
   The server page ships static .setting-item cards in index.html;
   this builder is the canonical way to create new cards (and the
   migration target for the static ones).
   Styling: SettingItem.css (single source of truth).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {Object=} opts
   * @param {string=} opts.id - id for the card root
   * @param {string=} opts.className - extra modifier(s)
   * @param {string|Node=} opts.label - card title (uppercase accent label)
   * @param {string|Node=} opts.description - helper text under/next to the label
   * @param {Node|string=} opts.content - body content (stack layout)
   * @param {Node|string=} opts.control - trailing control, e.g. a toggle (row layout)
   * @param {string=} opts.layout - "auto" | "row" | "stack" (defaults to "row" when control is given)
   * @param {Array.<Node>=} opts.children - extra nodes appended after the body
   * @returns {{element: HTMLElement, labelEl: HTMLElement, descriptionEl: HTMLElement|null}}
   */
  UI.SettingItem = function SettingItem(opts = {}) {
    const layout = opts.layout === "row" || opts.layout === "stack" ? opts.layout
      : opts.control ? "row" : "stack";

    const el = document.createElement("div");
    el.className = "setting-item" + (opts.className ? " " + opts.className : "");
    if (opts.id) el.id = opts.id;

    let labelEl = null;
    if (opts.label != null) {
      labelEl = document.createElement("div");
      labelEl.className = "setting-label";
      if (opts.label.nodeType) labelEl.appendChild(opts.label);
      else labelEl.textContent = String(opts.label);
    }

    let descriptionEl = null;
    if (opts.description != null) {
      descriptionEl = document.createElement("div");
      descriptionEl.className = "setting-description";
      if (opts.description.nodeType) descriptionEl.appendChild(opts.description);
      else descriptionEl.textContent = String(opts.description);
    }

    if (layout === "row") {
      const row = document.createElement("div");
      row.className = "setting-row";
      const text = document.createElement("div");
      if (labelEl) text.appendChild(labelEl);
      if (descriptionEl) text.appendChild(descriptionEl);
      row.appendChild(text);
      if (opts.control != null) {
        if (opts.control.nodeType) row.appendChild(opts.control);
        else row.appendChild(document.createTextNode(String(opts.control)));
      }
      el.appendChild(row);
    } else {
      if (labelEl) el.appendChild(labelEl);
      if (opts.content != null) {
        if (opts.content.nodeType) el.appendChild(opts.content);
        else el.appendChild(document.createTextNode(String(opts.content)));
      }
      if (descriptionEl) el.appendChild(descriptionEl);
    }

    (opts.children || []).forEach((c) => el.appendChild(c));

    return { element: el, labelEl, descriptionEl };
  };
})();