/* ============================================================
   TagChip — small removable tag/label chip (e.g. hidden extensions).
   Pure builder, no DOM side effects. Populates window.UI.
   Styling: TagChip.css (mirrors style.css until visual baseline).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {string} text — chip label
   * @param {Object=} handlers
   * @param {(text: string) => void} handlers.onRemove — wired to the ✕ button
   * @returns {HTMLElement} span.tag-chip
   */
  UI.TagChip = function TagChip(text, handlers = {}) {
    const chip = document.createElement("span");
    chip.className = "tag-chip";
    chip.textContent = text;

    if (handlers.onRemove) {
      const remove = document.createElement("button");
      remove.className = "tag-remove";
      remove.textContent = "✕";
      remove.title = "Remove " + text;
      remove.onclick = () => handlers.onRemove(text);
      chip.appendChild(remove);
    }

    return chip;
  };
})();