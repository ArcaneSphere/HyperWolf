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
   * @param {(scid: string) => void} handlers.onCopy
   * @param {(scid: string) => void} handlers.onExplore
   * @param {boolean=} handlers.bookmarked — resolved bookmark state (overrides r.bookmarked)
   * @returns {HTMLElement} div.result
   */
  UI.ResultRow = function ResultRow(r, handlers = {}) {
    const onClick = handlers.onClick || (() => {});
    const onBookmark = handlers.onBookmark || (() => {});
    const onCopy = handlers.onCopy || (() => {});
    const onExplore = handlers.onExplore || (() => {});
    const saved = handlers.bookmarked !== undefined ? handlers.bookmarked : !!(r.bookmarked);

    const div = document.createElement("div");
    div.className = "result";
    if (r.scid) div.dataset.scid = r.scid;
    const mfpHues = [
      "var(--c-green)", "var(--c-blue)", "var(--c-purple)",
      "var(--c-fuschia)", "var(--c-orange)", "var(--c-yellow)",
    ];
    const hue = mfpHues[(UI._resultHue = (UI._resultHue || -1) + 1) % mfpHues.length];
    div.style.setProperty("--self", hue);
    div.onclick = () => {
      div.classList.add("clicking");
      setTimeout(() => div.classList.remove("clicking"), 500);
      onClick(r.scid);
    };

    const iconSlot = document.createElement("div");
    iconSlot.className = "icon-slot";
    // --- Icon loading logic ---
    // 1. Bepaal de bron van het icoon
    let imgSrc = r.iconURL;

    // Helper: een kale DERO SCID (64 hex) die verwijst naar een TELA-STATIC
    // doc dat de echte iconafbeelding host (bv. "derotary" -> icon.svg).
    function isScidRef(src) {
      return typeof src === "string" && /^[0-9a-f]{64}$/.test(src);
    }

    // Basis van de TELA-content server (zonder /api suffix). Gelezen uit
    // window.UI.apiBase (gevuld door search.js via /api/config) met fallback
    // naar de standaard gnomon-poort.
    function telaContentBase() {
      const ab = (window.UI && window.UI.apiBase) || "http://127.0.0.1:18082/api";
      return String(ab).replace(/\/api\/?$/, "");
    }

    // TELA-bestanden worden soms on-gecomprimeerd op de chain gezet:
    // de on-wire payload is dan base64(gzip(raw)) maar het bestand heet niet
    // *.gz, dus de content server serveert de ruwe bytes. Sniff en decode
    // hier (raw gzip-bytes of base64("H4sI"...) > gzip) -> opgehaalde tekst.
    function b64ToBytes(b64) {
      const bin = atob(String(b64).replace(/\s+/g, ""));
      const bytes = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i) & 0xff;
      return bytes;
    }
    function gunzipToText(bytes) {
      if (typeof DecompressionStream === "undefined") {
        return Promise.reject(new Error("DecompressionStream niet beschikbaar"));
      }
      return new Response(new Blob([bytes]).stream().pipeThrough(new DecompressionStream("gzip"))).text();
    }
    function isGzip(bytes) {
      return bytes.length >= 2 && bytes[0] === 0x1f && bytes[1] === 0x8b;
    }
    async function decodeTelaIconPayload(buf) {
      const bytes = new Uint8Array(buf);
      if (isGzip(bytes)) return { text: await gunzipToText(bytes) };
      const text = new TextDecoder().decode(bytes);
      const t = text.trim();
      if (t.startsWith("H4sI") && /^[A-Za-z0-9+/]+={0,2}$/.test(t)) {
        const raw = b64ToBytes(t);
        if (isGzip(raw)) return { text: await gunzipToText(raw) };
      }
      return { text };
    }

    // Icon SVGs can come from on-chain contract data. Keep presentation and
    // internal SVG references, but remove executable content and external
    // resource references before inserting anything into the dashboard DOM.
    function sanitizeSvg(svg) {
      const blockedElements = new Set([
        "script", "foreignobject", "iframe", "object", "embed", "audio", "video", "base", "link",
      ]);
      const blockedAttributes = new Set([
        "src", "srcset", "action", "formaction", "poster", "xml:base",
      ]);
      const elements = [svg, ...svg.querySelectorAll("*")];

      for (const element of elements) {
        if (blockedElements.has(element.localName.toLowerCase())) {
          element.remove();
          continue;
        }

        for (const attribute of [...element.attributes]) {
          const name = attribute.name.toLowerCase();
          const value = attribute.value.trim();
          const lowerValue = value.toLowerCase();
          const externalURL = /^(https?:|data:|blob:|javascript:)/i.test(value);

          if (name.startsWith("on") || blockedAttributes.has(name)) {
            element.removeAttribute(attribute.name);
          } else if ((name === "href" || name === "xlink:href") && !value.startsWith("#")) {
            element.removeAttribute(attribute.name);
          } else if (
            name === "style" &&
            (/expression\s*\(/i.test(lowerValue) ||
              /javascript\s*:/i.test(lowerValue) ||
              /url\s*\(\s*(?!#)/i.test(lowerValue) ||
              externalURL)
          ) {
            element.removeAttribute(attribute.name);
          }
        }
      }
      return svg;
    }

    // Raster een SVG-tekst inline in de slot (zonder <img> omzeilt dit ook
    // de base64/gzip-blob-problematiek).
    function renderSvgText(svgText, slot) {
      if (!svgText || typeof svgText !== "string") return false;
      const host = document.createElement("div");
      host.innerHTML = svgText;
      const svg = host.querySelector("svg");
      if (svg) {
        sanitizeSvg(svg);
        // width/height zijn read-only getters op SVGSVGElement (strict mode
        // gooit TypeError), dus dimensioneren via inline CSS.
        svg.style.width = "48px";
        svg.style.height = "48px";
        svg.style.borderRadius = "var(--radius-lg)";
        slot.appendChild(svg);
        return true;
      }
      return false;
    }

    // Laad een kale SCID-referentie via de TELA-content server. Probeert
    // icon.svg (met gzip/base64-sniffing), dan icon.png als gewone <img>.
    async function fetchScidIcon(scidRef, slot) {
      const base = telaContentBase();
      const urls = [base + "/tela/" + scidRef + "/icon.svg", base + "/tela/" + scidRef + "/icon.png"];
      for (const u of urls) {
        try {
          const resp = await fetch(u, { mode: "cors" });
          if (!resp.ok) continue;
          if (u.endsWith(".svg")) {
            const { text } = await decodeTelaIconPayload(await resp.arrayBuffer());
            if (renderSvgText(text, slot)) return true;
          } else {
            const img = document.createElement("img");
            img.className = "icon";
            img.style.width = "48px";
            img.style.height = "48px";
            img.style.objectFit = "cover";
            img.style.borderRadius = "var(--radius-lg)";
            img.onerror = () => { img.style.display = "none"; };
            img.src = u;
            slot.appendChild(img);
            return true;
          }
        } catch (e) {
          // volgende kandidaat
        }
      }
      return false;
    }

    // Helper: probeer SVG-commentaarblok te extraheren en inline te tonen
    function tryInlineSvg(source) {
      if (!source || typeof source !== 'string') return false;
      const match = source.match(/\/\*([\s\S]*?)\*\//);
      if (!match) return false;
      const svgContent = match[1].trim();
      if (!svgContent.toLowerCase().includes('<svg') && !svgContent.includes('viewBox')) return false;
      const svgContainer = document.createElement('div');
      svgContainer.innerHTML = svgContent;
      const actualSvg = svgContainer.querySelector('svg');
      if (actualSvg) {
        sanitizeSvg(actualSvg);
        actualSvg.style.width = '48px';
        actualSvg.style.height = '48px';
        actualSvg.style.borderRadius = 'var(--radius-lg)';
        actualSvg.style.fill = 'currentColor';
        iconSlot.appendChild(actualSvg);
        return true;
      }
      return false;
    }

    // 1. Probeer direct uit r.iconURL als het een commentaarblok is
    if (tryInlineSvg(r.iconURL)) {
      // SVG inline getoond, klaar
    } else {
      // 2. Probeer via window.iconSC (wordt async ingesteld) met polling
      let retries = 0;
      const maxRetries = 10;
      const retryDelay = 200;
      function checkIconSC() {
        if (window.iconSC && window.iconSC.decoded && window.iconSC.decoded.C) {
          return tryInlineSvg(window.iconSC.decoded.C);
        }
        return false;
      }
      if (!window.iconSC || !window.iconSC.decoded || !window.iconSC.decoded.C) {
        // Pollen tot window.iconSC beschikbaar is
        let retries = 0;
        const interval = setInterval(() => {
          if (window.iconSC && window.iconSC.decoded && window.iconSC.decoded.C) {
            if (tryInlineSvg(window.iconSC.decoded.C)) {
              clearInterval(interval);
            }
          } else if (++retries >= 10) {
            clearInterval(interval);
          }
        }, 200);
      }

      // 3. Normale afbeelding URL (SVG-bestand, PNG, JPG, data-URL, etc.)
      // Alleen uitvoeren als nog geen inline SVG is geplaatst (geen <svg> in iconSlot)
      if (!iconSlot.querySelector('svg')) {
        const loadImg = (src) => {
          const img = document.createElement("img");
          img.className = "icon";
          img.style.width = "48px";
          img.style.height = "48px";
          img.style.objectFit = "cover";
          img.style.borderRadius = "var(--radius-lg)";
          // Geen crossOrigin: we rasteren icons nooit naar een canvas, dus CORS
          // eisen zou alleen maar icons breken van servers zonder CORS-headers.
          if (isScidRef(src)) {
            // Kale SCID -> TELA-content server. Fetch + sniff gzip/base64,
            // want de content server serveert gecomprimeerde iconen raw.
            fetchScidIcon(src, iconSlot);
          } else {
            // IconURL zonder scheme (bv. "www.loc.gov/...jpg") -> https://
            let finalSrc = src;
            if (!/^(https?:|data:|blob:|\.|\/|#)/i.test(finalSrc)) {
              finalSrc = "https://" + finalSrc;
            }
            img.onerror = () => { img.style.display = "none"; };
            img.src = finalSrc;
            iconSlot.appendChild(img);
          }
        };
        const imgSrc = r.iconURL;
        if (imgSrc) {
          loadImg(imgSrc);
        }
      }
    }
    // Als niets lukt, blijft de slot-div leeg (leeg = achtergrond #1a1a1a zichtbaar)

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

    const actions = document.createElement("div");
    actions.className = "result-actions";

    const bookmarkBtn = document.createElement("button");
    bookmarkBtn.className = "result-bookmark";
    bookmarkBtn.type = "button";
    bookmarkBtn.title = saved ? "Remove bookmark" : "Add bookmark";
    bookmarkBtn.setAttribute("aria-label", bookmarkBtn.title);
    bookmarkBtn.textContent = saved ? "★" : "☆";
    bookmarkBtn.classList.toggle("saved", saved);
    bookmarkBtn.onclick = (e) => {
      e.stopPropagation();
      onBookmark(r.scid);
    };

    const copyBtn = document.createElement("button");
    copyBtn.className = "result-copy";
    copyBtn.type = "button";
    copyBtn.title = "Copy SCID";
    copyBtn.textContent = "Copy";
    copyBtn.onclick = (e) => {
      e.stopPropagation();
      onCopy(r.scid);
    };

    const exploreBtn = document.createElement("button");
    exploreBtn.className = "result-explore";
    exploreBtn.type = "button";
    exploreBtn.title = "Open in explorer";
    exploreBtn.textContent = "Explorer";
    exploreBtn.onclick = (e) => {
      e.stopPropagation();
      onExplore(r.scid);
    };

    actions.append(copyBtn, exploreBtn, bookmarkBtn);

    div.append(iconSlot, content, actions);
    return div;
  };
})();
