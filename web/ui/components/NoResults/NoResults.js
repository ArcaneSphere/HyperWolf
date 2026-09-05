/* ============================================================
   NoResults — empty-state placeholder.
   Pure builder, no DOM side effects. Populates window.UI.
   Styling: NoResults.css (mirrors style.css until visual baseline).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  UI.NoResults = function NoResults(text) {
    const div = document.createElement("div");
    div.className = "no-results";
    div.textContent = text;
    return div;
  };
})();