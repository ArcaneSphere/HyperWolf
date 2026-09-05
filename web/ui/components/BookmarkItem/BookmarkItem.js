/* ============================================================
   BookmarkItem — a saved node/SCID bookmark row: info block plus
   Load + inline label-edit (✏) + Remove actions.
   Pure builder, no DOM side effects. Populates window.UI.
   dashboard.js's createBookmarkItem() delegates here, so the app
   and the component stay in sync.
   Styling: BookmarkItem.css (single source of truth).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {Object} bm
   * @param {string} bm.label - display label (default for new bookmarks)
   * @param {string} bm.value - node address or SCID
   * @param {() => void=} bm.onLoad - Load button handler
   * @param {() => void=} bm.onRemove - Remove button handler
   * @param {(newLabel: string) => void=} bm.onCommit - fires on blur/Enter after inline label edit
   * @returns {{element: HTMLElement, labelEl: HTMLElement, valueEl: HTMLElement}}
   */
  UI.BookmarkItem = function BookmarkItem(bm = {}) {
    const root = document.createElement("div");
    root.className = "bookmark-item";

    const info = document.createElement("div");
    info.className = "bookmark-info";
    const labelEl = document.createElement("div");
    labelEl.className = "bookmark-label";
    labelEl.textContent = bm.label;
    const valueEl = document.createElement("div");
    valueEl.className = "bookmark-value";
    valueEl.textContent = bm.value;
    info.append(labelEl, valueEl);

    const actions = document.createElement("div");
    actions.className = "bookmark-actions";
    const load = document.createElement("button");
    load.className = "small";
    load.textContent = "Load";
    load.onclick = () => bm.onLoad && bm.onLoad();
    const edit = document.createElement("button");
    edit.className = "small";
    edit.textContent = "✏";
    edit.title = "Edit label";
    edit.onclick = (e) => {
      e.stopPropagation();
      labelEl.contentEditable = "true";
      labelEl.focus();
      const range = document.createRange();
      range.selectNodeContents(labelEl);
      const sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
    };
    const remove = document.createElement("button");
    remove.className = "small danger";
    remove.textContent = "Remove";
    remove.onclick = () => bm.onRemove && bm.onRemove();
    actions.append(load, edit, remove);

    labelEl.addEventListener("blur", () => {
      labelEl.contentEditable = "false";
      if (bm.onCommit) bm.onCommit(labelEl.textContent || bm.value.slice(0, 8));
    });
    labelEl.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        labelEl.blur();
      }
    });

    root.append(info, actions);
    return { element: root, labelEl: labelEl, valueEl: valueEl };
  };
})();