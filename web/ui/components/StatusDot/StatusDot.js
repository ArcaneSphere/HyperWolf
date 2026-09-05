/* ============================================================
   StatusDot — colored status indicator.
   Pure builder, no DOM side effects. Populates window.UI.
   States: connected, error, warning, pending (default muted).
   Styling: StatusDot.css (mirrors style.css until visual baseline).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  UI.StatusDot = function StatusDot(state) {
    const span = document.createElement("span");
    span.className = "status-dot " + state;
    return span;
  };
})();