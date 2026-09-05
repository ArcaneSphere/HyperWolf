/* ============================================================
   HexIcon — DERO/SCID hexagon placeholder icon
   Pure builder, no DOM side effects. Populates window.UI.
   Styling: HexIcon.css (mirrors style.css until visual baseline).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  UI.HexIcon = function HexIcon() {
    const div = document.createElement("div");
    div.className = "scid-svg";
    div.innerHTML = `
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 867 1001">
        <polygon points="0.5,250.55 433.47,0.58 866.43,250.55 866.43,750.5 433.47,1000.47 0.5,750.5" fill="none" stroke="currentColor" stroke-width="6"/>
        <polygon points="209.17,371.63 209.17,628.97 433.69,759.79 657.39,630.28 657.39,374.85 433.26,241.71 209.17,371.63" fill="none" stroke="currentColor" stroke-width="6"/>
        <polygon points="239.64,389.3 239.64,611.21 348.32,675.69 366.79,580 331.72,558.65 331.72,442.81 433.41,384.91 533.88,443.47 533.88,559.84 498.24,579.73 515.93,678.24 626.31,612.4 626.31,392.17 433.26,277.45 239.64,389.3" fill="none" stroke="currentColor" stroke-width="6"/>
        <polygon points="432.54,420.32 502.73,461.22 502.73,542 464.66,563.58 485.09,694.51 433.7,724.39 378.96,692.5 400.62,564.62 362.1,541.19 362.1,461.28 432.54,420.32" fill="none" stroke="currentColor" stroke-width="6"/>
      </svg>`;
    return div;
  };
})();