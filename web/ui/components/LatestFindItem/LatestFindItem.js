/* ============================================================
   LatestFindItem — one row in the "latest finds" list.
   Pure builder, no DOM side effects. Populates window.UI.
   Styling: LatestFindItem.css (mirrors style.css until visual baseline).
   Depends on: UI.HexIcon.
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {Object} app — find item
   * @param {string} app.scid
   * @param {string=} app.name
   * @param {string=} app.durl
   * @param {string=} app.iconURL
   * @param {number=} app.install_height
   * @param {boolean=} app.isNew — renders the NEW tag
   * @param {Object=} handlers
   * @param {(app: Object) => void} handlers.onClick
   * @returns {HTMLElement} div.latest-find-item
   */
  UI.LatestFindItem = function LatestFindItem(app, handlers = {}) {
    const row = document.createElement("div");
    row.className = "latest-find-item";

    const iconSlot = document.createElement("div");
    iconSlot.className = "icon-slot";
    if (app.iconURL) {
      const img = document.createElement("img");
      img.className = "icon";
      img.src = app.iconURL;
      img.onerror = () => iconSlot.replaceChildren(UI.HexIcon());
      iconSlot.appendChild(img);
    } else {
      iconSlot.appendChild(UI.HexIcon());
    }

    const content = document.createElement("div");
    content.className = "content";
    const urlEl = document.createElement("div");
    urlEl.className = "url";
    urlEl.textContent = app.durl || app.scid;
    const nameEl = document.createElement("div");
    nameEl.className = "nameHdr";
    nameEl.textContent = app.name || app.scid;
    const scidEl = document.createElement("div");
    scidEl.className = "scid";
    scidEl.textContent = app.scid;
    content.append(urlEl, nameEl, scidEl);

    const meta = document.createElement("div");
    meta.className = "latest-find-meta";
    if (app.isNew) {
      const tag = document.createElement("span");
      tag.className = "latest-find-tag";
      tag.textContent = "NEW";
      meta.appendChild(tag);
    }
    if (app.install_height > 0) {
      const h = document.createElement("span");
      h.textContent = "h" + app.install_height.toLocaleString();
      meta.appendChild(h);
    }

    row.append(iconSlot, content, meta);
    row.onclick = () => handlers.onClick && handlers.onClick(app);
    return row;
  };
})();