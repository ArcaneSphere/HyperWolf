(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  // ExplorerStat renders a single stat tile (label + value + optional sub).
  UI.ExplorerStat = function ExplorerStat({ label, value, sub } = {}) {
    const el = document.createElement("div");
    el.className = "explorer-stat";
    el.innerHTML =
      '<div class="exp-stat-value"></div>' +
      '<div class="exp-stat-label"></div>' +
      '<div class="exp-stat-sub"></div>';
    el.querySelector(".exp-stat-label").textContent = label || "";
    el.querySelector(".exp-stat-value").textContent = value ?? "";
    el.querySelector(".exp-stat-sub").textContent = sub ?? "";
    return el;
  };
})();