/* ============================================================
   ResultRow — compact representation of one search result.
   Pure builder, no DOM side effects. Populates window.UI.
   Styling: ResultRow.css (mirrors style.css until visual baseline).
   Depends on: UI.HexIcon.
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {Object} r — result props
   * @param {string} r.scid
   * @param {string} r.nameHdr
   * @param {string} r.dURL
   * @param {string} r.descrHdr
   * @param {string=} r.iconURL
   * @param {number=} r.likes
   * @param {number=} r.dislikes
   * @param {number=} r.average
   * @param {boolean=} r.ratingsLoaded
   * @param {boolean=} r.bookmarked
   * @param {Object=} handlers
   * @param {(scid: string) => void} handlers.onClick
   * @param {(scid: string) => void} handlers.onBookmark
   * @param {boolean=} handlers.bookmarked — resolved bookmark state (overrides r.bookmarked)
   * @returns {HTMLElement} div.result
   */
  UI.ResultRow = function ResultRow(r, handlers = {}) {
    const onClick = handlers.onClick || (() => {});
    const onBookmark = handlers.onBookmark || (() => {});
    const saved = handlers.bookmarked !== undefined ? handlers.bookmarked : !!(r.bookmarked);

    const div = document.createElement("div");
    div.className = "result";
    div.onclick = () => {
      div.classList.add("clicking");
      setTimeout(() => div.classList.remove("clicking"), 500);
      onClick(r.scid);
    };

    const iconSlot = document.createElement("div");
    iconSlot.className = "icon-slot";
    if (r.iconURL) {
      const img = document.createElement("img");
      img.className = "icon";
      img.src = r.iconURL;
      img.onerror = () => iconSlot.replaceChildren(UI.HexIcon());
      iconSlot.appendChild(img);
    } else {
      iconSlot.appendChild(UI.HexIcon());
    }

    const content = document.createElement("div");
    content.className = "content";

    const urlEl = document.createElement("div");
    urlEl.className = "url";
    urlEl.textContent = r.dURL;
    const nameEl = document.createElement("div");
    nameEl.className = "nameHdr";
    nameEl.textContent = r.nameHdr;
    const scidEl = document.createElement("div");
    scidEl.className = "scid";
    scidEl.textContent = r.scid;
    const descrEl = document.createElement("div");
    descrEl.className = "descr";
    descrEl.textContent = r.descrHdr;
    const ratingEl = document.createElement("div");
    ratingEl.className = "rating";
    ratingEl.textContent = r.ratingsLoaded ? `👍 ${r.likes} 👎 ${r.dislikes} ⭐ ${r.average}` : "—";

    [urlEl, nameEl, scidEl].forEach((el) => {
      el.style.cursor = "pointer";
      el.onclick = (e) => {
        e.stopPropagation();
        onClick(r.scid);
      };
    });

    content.append(urlEl, nameEl, scidEl, descrEl, ratingEl);

    const bookmarkBtn = document.createElement("button");
    bookmarkBtn.className = "result-bookmark";
    bookmarkBtn.textContent = saved ? "★" : "☆";
    bookmarkBtn.classList.toggle("saved", saved);
    bookmarkBtn.onclick = (e) => {
      e.stopPropagation();
      onBookmark(r.scid);
    };

    div.append(iconSlot, content, bookmarkBtn);
    return div;
  };
})();