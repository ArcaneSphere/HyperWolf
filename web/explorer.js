/* ============================================================
   explorer.js — in-app blockchain explorer page.
   Hash-driven SPA view; renders Overview / Blocks / Mempool /
   Block / Tx / SC / Address views into #exp-content.
   Depends on: UI.ExplorerStat, UI.ExplorerBlockRow,
   UI.ExplorerTxRow, UI.NoResults, UI.Toast.
   ============================================================ */
(function () {
  "use strict";
  const UI = window.UI || {};

  const els = {};
  let activeTimer = null;

  const PAGE_SIZE = 15;

  document.addEventListener("DOMContentLoaded", () => {
    els.content = document.getElementById("exp-content");
    els.search = document.getElementById("exp-search-input");
    els.tabs = document.getElementById("exp-tabs");

    els.tabs.addEventListener("click", (e) => {
      const tab = e.target.closest(".exp-tab");
      if (!tab) return;
      const name = tab.dataset.tab;
      if (name === "overview") setHash("");
      else if (name === "blocks") setHash("blocks");
      else if (name === "mempool") setHash("mempool");
      else setHash("");
    });

    els.search.addEventListener("keydown", (e) => {
      if (e.key === "Enter") doSearch();
    });
    els.search.addEventListener("search", (e) => {
      if (e.target.value === "") setHash("");
    });

    window.addEventListener("hashchange", onHash);
    document.addEventListener("pageChanged", (e) => {
      if (e.detail && e.detail.page === "explorer") onHash();
    });
    onHash();
  });

  function onHash() {
    const page = document.getElementById("page-explorer");
    if (!page || !page.classList.contains("active")) return;
    stopAutoRefresh();
    const parts = parseHash();
    render(parts);
  }

  function parseHash() {
    let raw = (location.hash || "").replace(/^#\/?/, "");
    if (raw === "" || raw === "exp") return { view: "overview" };
    if (raw.startsWith("exp/")) raw = raw.slice(4);
    const first = raw.split("/")[0];
    const map = {
      overview: { view: "overview" },
      blocks: { view: "blocks", page: 0 },
      block: { view: "block", id: raw.slice(6) },
      tx: { view: "tx", id: raw.slice(3) },
      sc: { view: "sc", id: raw.slice(3) },
      mempool: { view: "mempool" },
      address: { view: "address", id: raw.slice(8) },
    };
    if (first === "blocks") {
      const m = raw.split("/");
      map.blocks.page = parseInt(m[1], 10);
      if (!Number.isFinite(map.blocks.page) || map.blocks.page < 0) map.blocks.page = 0;
    }
    if (first === "address") {
      const m = raw.slice(8).split("/");
      map.address.id = m[0] || "";
      map.address.name = m[1] ? decodeURIComponent(m[1]) : "";
    }
    return map[first] || { view: "overview" };
  }

  function setHash(str) {
    const target = "#" + (str ? "exp/" + str : "");
    if (location.hash === target) onHash();
    else location.hash = target;
  }

  // ---- rendering dispatcher ----

  function render(parts) {
    els.content.scrollIntoView({ block: "start", behavior: "smooth" });
    setTab(parts.view);
    switch (parts.view) {
      case "blocks":
        renderBlocks(parts.page);
        break;
      case "block":
        fetchView("/api/explorer/block/" + encodeURIComponent(parts.id), renderBlock, renderError);
        break;
      case "tx":
        fetchView("/api/explorer/tx/" + encodeURIComponent(parts.id), renderTx, renderError);
        break;
      case "sc":
        fetchView("/api/explorer/sc/" + encodeURIComponent(parts.id), renderSC, renderError);
        break;
      case "address": {
        const p = parts.name ? "?name=" + encodeURIComponent(parts.name) : "";
        fetchView("/api/explorer/address/" + encodeURIComponent(parts.id) + p, renderAddress, renderError);
        break;
      }
      case "mempool":
        renderMempool();
        break;
      default:
        renderOverview();
    }
  }

  function setTab(view) {
    const name = { overview: "overview", blocks: "blocks", mempool: "mempool" }[view] || null;
    els.tabs.querySelectorAll(".exp-tab").forEach((t) => {
      t.classList.toggle("active", t.dataset.tab === name);
    });
  }

  // ---- loading / error ----

  function showLoading(msg) {
    els.content.replaceChildren(spinner());
    if (msg) {
      const note = document.createElement("div");
      note.className = "exp-note";
      note.textContent = msg;
      els.content.appendChild(note);
    }
  }

  function spinner() {
    if (UI.Spinner) return UI.Spinner("exp");
    const el = document.createElement("div");
    el.className = "exp-spinner";
    el.textContent = "…";
    return el;
  }

  const RETRY = "↻";

  function renderError() {
    const el = document.createElement("div");
    el.className = "exp-error";
    el.textContent = "Unable to reach the daemon. Check Node Status.";
    const btn = document.createElement("button");
    btn.className = "exp-retry";
    btn.textContent = RETRY + " Retry";
    btn.onclick = () => {
      if (parseHash().view === "block") {
        const p = parseHash();
        fetchView("/api/explorer/block/" + encodeURIComponent(p.id), renderBlock, renderError);
      }
    };
    el.append(btn);
    els.content.replaceChildren(el);
  }

  function note(msg) {
    const el = document.createElement("div");
    el.className = "exp-note";
    el.textContent = msg;
    return el;
  }

  // ---- fetch ----

  async function fetchView(url, ok, err) {
    showLoading("");
    try {
      const res = await fetch(url, { headers: { Accept: "application/json" } });
      const data = await res.json();
      if (data && (data.ok === undefined || data.ok === true)) {
        ok(data && data.result !== undefined ? data.result : data);
        return;
      }
      err();
    } catch (e) {
      err();
    }
  }

  // ---- Overview ----

  function renderOverview() {
    showLoading("");
    Promise.all([
      fetch("/api/explorer/stats", { headers: { Accept: "application/json" } }).then((r) => r.json()),
      fetch("/api/explorer/blocks?count=8", { headers: { Accept: "application/json" } }).then((r) => r.json()),
    ])
      .then(([d, b]) => {
        if (!d || d.ok === false) {
          renderNodeNotSet();
          return;
        }
        const s = d.result;
        const recent = b && b.ok && Array.isArray(b.result.headers) ? b.result.headers : [];
        els.content.replaceChildren();

        const grid = document.createElement("div");
        grid.className = "exp-stats-grid";
        grid.append(
          UI.ExplorerStat({ label: "Network", value: s.network, sub: "derod " + (s.version || "") }),
          UI.ExplorerStat({ label: "Topo height", value: fmtNum(s.topoheight), sub: "stable " + fmtNum(s.stableheight) }),
          UI.ExplorerStat({ label: "Block height", value: fmtNum(s.height), sub: s.last_block ? agoFromMs(s.last_block.timestamp) : "" }),
          UI.ExplorerStat({ label: "Difficulty", value: fmtNum(s.difficulty), sub: s.median_block_size ? "median block " + fmtNum(s.median_block_size) + " B" : "" }),
          UI.ExplorerStat({ label: "Block time", value: s.average_block_time_50 ? s.average_block_time_50.toFixed(1) + "s" : "—", sub: "50-block average" }),
          UI.ExplorerStat({ label: "Total supply", value: s.total_supply_dero, sub: "DERO" }),
          UI.ExplorerStat({ label: "Mempool", value: fmtNum(s.mempool_size) + " txs", sub: s.dynamic_fee_per_kb_dero ? "fee " + s.dynamic_fee_per_kb_dero + "/KB" : "" }),
          UI.ExplorerStat({ label: "Top block", value: s.top_block_hash ? s.top_block_hash.slice(0, 12) + "…" : "—", sub: s.topoheight ? "topo " + fmtNum(s.topoheight) : "" }),
          UI.ExplorerStat({ label: "Connections", value: fmtNum(s.connections), sub: s.peers ? "peers " + fmtNum(s.peers) : "" }),
        );
        els.content.appendChild(grid);

        const tableWrap = explorerTable("Recent blocks", ["TOPO", "HEIGHT", "HASH", "TXS", "REWARD", "SIZE", "AGE"], () => {
          setHash("blocks");
        }, "View all blocks");
        const tbody = tableWrap.querySelector("tbody");
        if (recent.length) {
          recent.forEach((h) => tbody.appendChild(UI.ExplorerBlockRow(h, { onOpen: (hd) => setHash("block/" + hd.hash) })));
        } else {
          tbody.appendChild(emptyRow("view all"));
        }
        els.content.appendChild(tableWrap);

        if (s.mempool_size > 0) {
          const poolWrap = explorerTable("Mempool", ["HASH", "TYPE", "FEE", "RING", "SIZE", "SIGNER", "BLOCK", "AGE"], () => {
            setHash("mempool");
          }, "Open mempool");
          const pt = poolWrap.querySelector("tbody");
          pt.appendChild(emptyRow("fetching " + s.mempool_size + " pooled txs…"));
          els.content.appendChild(poolWrap);
          loadMempoolInto(pt, 15);
        }

        scheduleOverviewRefresh();
      })
      .catch(() => renderNodeNotSet());
  }

  function scheduleOverviewRefresh() {
    stopAutoRefresh();
    const page = document.getElementById("page-explorer");
    if (page && page.classList.contains("active") && currentView() === "overview") {
      activeTimer = setTimeout(() => {
        if (currentView() === "overview") renderOverview();
      }, 10000);
    }
  }

  function currentView() {
    return parseHash().view;
  }

  function stopAutoRefresh() {
    if (activeTimer) {
      clearTimeout(activeTimer);
      activeTimer = null;
    }
  }

  function renderNodeNotSet() {
    els.content.replaceChildren();
    const el = document.createElement("div");
    el.className = "exp-error";
    const p = document.createElement("p");
    p.textContent = "No daemon node is connected. Connect a node to use the chain explorer.";
    const btn = document.createElement("button");
    btn.className = "exp-retry";
    btn.textContent = "Go to Node Status";
    btn.onclick = () => {
      const nav = document.querySelector("[data-page=server]");
      if (nav) nav.click();
    };
    el.append(p, btn);
    els.content.appendChild(el);
  }

  // ---- Blocks ----

  function renderBlocks(page) {
    showLoading("");
    const from = page * PAGE_SIZE;
    fetch("/api/explorer/blocks?count=" + PAGE_SIZE + "&from=" + from, { headers: { Accept: "application/json" } })
      .then((r) => r.json())
      .then((d) => {
        if (!d || d.ok === false) {
          renderError();
          return;
        }
        const res = d.result;
        els.content.replaceChildren();

        const heading = document.createElement("div");
        heading.className = "exp-heading";
        const span = document.createElement("span");
        span.textContent = "Blocks";
        const sub = document.createElement("span");
        sub.className = "exp-heading-sub";
        sub.textContent = res.top ? "tip topo " + fmtNum(res.top) : "";
        heading.append(span, sub);
        els.content.appendChild(heading);

        const tableWrap = tableWrapEl(["TOPO", "HEIGHT", "HASH", "TXS", "REWARD", "SIZE", "AGE"]);
        const tbody = tableWrap.querySelector("tbody");
        if (res.headers && res.headers.length) {
          res.headers.forEach((h) => tbody.appendChild(UI.ExplorerBlockRow(h, { onOpen: (hd) => setHash("block/" + hd.hash) })));
        } else {
          tbody.appendChild(emptyRow("no blocks"));
        }
        els.content.appendChild(tableWrap);

        els.content.appendChild(pager(page, from, res.top));
      })
      .catch(() => renderError());
  }

  function pager(page, from, top) {
    const wrap = document.createElement("div");
    wrap.className = "exp-pager";

    const prev = document.createElement("button");
    prev.className = "exp-pager-btn";
    prev.disabled = page <= 0;
    prev.textContent = "◀ Newer";
    prev.onclick = () => setHash("blocks/" + (page - 1));

    const label = document.createElement("span");
    label.className = "exp-pager-label";
    const bottom = Math.max(0, from - PAGE_SIZE + 1);
    label.textContent = (from >= top && top > 0 ? top : from) + " → " + bottom;

    const next = document.createElement("button");
    next.className = "exp-pager-btn";
    next.disabled = from >= top;
    next.textContent = "Older ▶";
    next.onclick = () => setHash("blocks/" + (page + 1));

    wrap.append(prev, label, next);
    return wrap;
  }

  // ---- Mempool ----

  function renderMempool() {
    showLoading("Contacting node for pooled transactions…");
    fetch("/api/explorer/mempool?count=200", { headers: { Accept: "application/json" } })
      .then((r) => r.json())
      .then((d) => {
        if (!d || d.ok === false) {
          renderError();
          return;
        }
        const res = d.result;
        els.content.replaceChildren();

        const heading = document.createElement("div");
        heading.className = "exp-heading";
        const span = document.createElement("span");
        span.textContent = "Mempool";
        const sub = document.createElement("span");
        sub.className = "exp-heading-sub";
        sub.textContent = (res.pool_size || 0) + " pooled";
        heading.append(span, sub);
        els.content.appendChild(heading);

        const tableWrap = tableWrapEl(["HASH", "TYPE", "FEE", "RING", "SIZE", "SIGNER", "BLOCK", "AGE"]);
        const tbody = tableWrap.querySelector("tbody");
        if (res.txs && res.txs.length) {
          const txs = res.txs.filter((t) => t && t.hash);
          txs.forEach((t) =>
            tbody.appendChild(
              UI.ExplorerTxRow(t, { onOpenTx: (tx) => setHash("tx/" + tx.hash), onOpenBlock: () => {} }),
            ),
          );
        } else {
          tbody.appendChild(emptyRow("mempool is empty"));
        }
        els.content.appendChild(tableWrap);

        const refresh = document.createElement("div");
        refresh.className = "exp-refresh";
        const btn = document.createElement("button");
        btn.className = "exp-retry";
        btn.textContent = RETRY + " Refresh";
        btn.onclick = () => renderMempool();
        refresh.appendChild(btn);
        els.content.appendChild(refresh);
      })
      .catch(() => renderError());
  }

  function loadMempoolInto(tbody, count) {
    fetch("/api/explorer/mempool?count=" + count, { headers: { Accept: "application/json" } })
      .then((r) => r.json())
      .then((d) => {
        tbody.replaceChildren();
        if (d && d.ok && d.result.txs && d.result.txs.length) {
          d.result.txs
            .filter((t) => t && t.hash)
            .forEach((t) => tbody.appendChild(UI.ExplorerTxRow(t, { onOpenTx: (tx) => setHash("tx/" + tx.hash), onOpenBlock: () => {} })));
        } else {
          tbody.appendChild(emptyRow("mempool is empty"));
        }
      })
      .catch(() => {
        tbody.appendChild(emptyRow("mempool unavailable"));
      });
  }

  // ---- Block detail ----

  function renderBlock(bl) {
    els.content.replaceChildren();
    els.content.appendChild(breadcrumb("block", bl.header && bl.header.hash));

    const head = bl.header || {};
    const kv = [
      ["Height", head.height],
      ["Topoheight", head.topoheight],
      ["Hash", head.hash],
      ["Depth", head.depth],
      ["Version", cvar(head.major_version) + "." + cvar(head.minor_version)],
      ["Nonce", cvar(head.nonce)],
      ["Difficulty", head.difficulty],
      ["Reward", head.reward_dero + " DERO"],
      ["Fees", bl.fees + " DERO"],
      ["Size", bl.size + " kB"],
      ["Timestamp", head.timestamp ? dateFromMs(head.timestamp) : ""],
      ["Age", head.timestamp ? agoFromMs(head.timestamp) : ""],
      ["Txs", (bl.tx_count ?? 0) + (bl.miner_tx ? " (incl. miner tx)" : "")],
      ["Sync block", head.sync_block ? "yes" : "no"],
      ["Side block", head.side_block ? "yes" : "no"],
    ];
    els.content.appendChild(keyValues("Block " + (head.topoheight !== undefined ? head.topoheight : head.height), kv, "block"));

    if (bl.miner_tx) {
      const mt = tableWrapEl(["HASH", "TYPE", "AMOUNT", "SIZE", "MINER"]);
      const tbody = mt.querySelector("tbody");
      appendTxRow(tbody, bl.miner_tx);
      els.content.appendChild(panel("Miner tx", mt));
    }

    if (bl.txs && bl.txs.length) {
      const tw = tableWrapEl(["HASH", "TYPE", "FEE", "RING", "SIZE", "SIGNER", "BLOCK", "AGE"]);
      const tbody = tw.querySelector("tbody");
      bl.txs.forEach((t) => appendTxRow(tbody, t));
      els.content.appendChild(panel("Transactions (" + bl.txs.length + ")", tw));
    } else if (bl.tx_count > 1) {
      els.content.appendChild(note("Transactions are not available from this node."));
    }

    els.content.appendChild(prevNextBlock(head));
  }

  function appendTxRow(tbody, tx) {
    tbody.appendChild(UI.ExplorerTxRow(tx, { onOpenTx: (t) => setHash("tx/" + t.hash), onOpenBlock: () => {} }));
  }

  function prevNextBlock(head) {
    const wrap = document.createElement("div");
    wrap.className = "exp-pager";
    if (head.topoheight > 0) {
      const prev = document.createElement("button");
      prev.className = "exp-pager-btn";
      prev.textContent = "◀ Previous block";
      prev.onclick = () => setHash("block/" + (head.topoheight - 1));
      wrap.appendChild(prev);
    }
    const next = document.createElement("button");
    next.className = "exp-pager-btn";
    next.textContent = "Next block ▶";
    next.onclick = () => setHash("block/" + (head.topoheight + 1));
    wrap.appendChild(next);
    return wrap;
  }

  // ---- Tx detail ----

  function renderTx(tx) {
    els.content.replaceChildren();
    els.content.appendChild(breadcrumb("tx", tx.hash));

    const kv = [
      ["Hash", tx.hash],
      ["Type", tx.type],
      ["Version", cvar(tx.version)],
      ["Status", tx.in_pool ? "in mempool" : "confirmed"],
      ["Height", tx.in_pool ? "—" : tx.height],
      ["Block", tx.valid_block || (tx.in_pool ? "—" : "")],
      ["Block time", tx.block_time || ""],
      ["Age", tx.age || ""],
      ["Signer", tx.signer],
      ["Fee", tx.fee + " DERO" || "—"],
      ["Burn", tx.burn_value],
      ["Ring size", tx.ring_size],
      ["Size", tx.size + " kB"],
      ["SCID", tx.scid],
      ["SC balance", tx.sc_balance],
      ["BLID", tx.blid],
      ["Built at", tx.height_built],
      ["Output indices", tx.output_indices ? tx.output_indices.join(", ") : ""],
      ["Invalid in", tx.invalid_block ? tx.invalid_block.slice(0, 3).join(", ") : ""],
    ];
    els.content.appendChild(keyValues("Transaction", kv, "tx"));

    if (tx.payloads && tx.payloads.length) {
      const pw = tableWrapEl(["SCID", "FEE", "BURN", "RING"]);
      const tbody = pw.querySelector("tbody");
      tx.payloads.forEach((p) => {
        const tr = document.createElement("tr");
        const mk = (txt) => {
          const td = document.createElement("td");
          td.textContent = txt;
          td.className = "mono";
          return td;
        };
        const link = document.createElement("td");
        const a = document.createElement("a");
        a.className = "exp-link";
        a.href = "";
        a.textContent = p.scid.slice(0, 18) + "…";
        a.title = p.scid;
        a.onclick = (e) => {
          e.preventDefault();
          setHash("sc/" + p.scid);
        };
        link.appendChild(a);
        tr.append(link, mk(p.fees), mk(p.burn), mk(p.ring_size));
        tbody.appendChild(tr);
      });
      els.content.appendChild(panel("Payloads", pw));
    }

    if (tx.sc_args && tx.sc_args.length) {
      els.content.appendChild(panel("SC arguments", preCode(JSON.stringify(tx.sc_args, null, 2))));
    }
    if (tx.sc_code) {
      els.content.appendChild(panel("SC code", preCode(tx.sc_code)));
    }
    if (tx.hex) {
      els.content.appendChild(panel("Raw hex", preCode(tx.hex)));
    }
  }

  // ---- SC detail ----

  function renderSC(sc) {
    els.content.replaceChildren();
    els.content.appendChild(breadcrumb("sc", sc.scid));

    const kv = [
      ["SCID", sc.scid],
      ["Balance", sc.balance_dero + " DERO"],
      ["Code length", sc.code ? fmtNum(sc.code.length) : "—"],
    ];
    els.content.appendChild(keyValues("Smart Contract", kv, "sc"));

    if (sc.code) els.content.appendChild(panel("Code", preCode(sc.code)));

    if (sc.stringkeys && Object.keys(sc.stringkeys).length) {
      els.content.appendChild(panel("Variables (string keys)", kvTable(sc.stringkeys)));
    }
    if (sc.uint64keys && Object.keys(sc.uint64keys).length) {
      els.content.appendChild(panel("Variables (uint64 keys)", kvTable(sc.uint64keys)));
    }
    if (sc.balances && Object.keys(sc.balances).length) {
      const bt = tableWrapEl(["ADDRESS", "BALANCE"]);
      const tbody = bt.querySelector("tbody");
      Object.keys(sc.balances)
        .slice(0, 50)
        .forEach((addr) => {
          const tr = document.createElement("tr");
          const a1 = document.createElement("td");
          a1.className = "mono";
          a1.textContent = addr;
          const a2 = document.createElement("td");
          a2.className = "num mono";
          a2.textContent = (sc.balances[addr] / 100000).toFixed(5);
          tr.append(a1, a2);
          tbody.appendChild(tr);
        });
      els.content.appendChild(panel("Balances", bt));
    }
  }

  // ---- Address detail ----

  function renderAddress(info) {
    els.content.replaceChildren();
    els.content.appendChild(breadcrumb("address", info.address));

    const kv = [["Address", info.address]];
    if (info.name) kv.push(["Registered name", info.name]);
    els.content.appendChild(keyValues("Address", kv, "address"));

    els.content.appendChild(
      note("DERO balances and transaction history are private and only visible with the owner's view key. To track an address, add it to your wallet and connect via xswd."),
    );
  }

  // ---- helpers ----

  function breadcrumb(kind, id) {
    const el = document.createElement("div");
    el.className = "exp-crumb";
    const back = document.createElement("a");
    back.className = "exp-link";
    back.href = "";
    back.textContent = "← Explorer";
    back.onclick = (e) => {
      e.preventDefault();
      setHash("");
    };
    const sep = document.createElement("span");
    sep.textContent = " / " + kind;
    const idEl = document.createElement("span");
    idEl.className = "exp-crumb-id";
    idEl.textContent = id ? short(id) : "";
    el.append(back, sep, idEl);
    return el;
  }

  function keyValues(title, rows, kind) {
    const wrap = document.createElement("div");
    wrap.className = "exp-kvwrap";
    if (title) {
      const h = document.createElement("div");
      h.className = "exp-kv-title";
      h.textContent = title;
      wrap.appendChild(h);
    }
    const table = document.createElement("table");
    table.className = "exp-kv";
    rows.forEach(([k, v]) => {
      const tr = document.createElement("tr");
      const ktd = document.createElement("td");
      ktd.className = "exp-kv-key";
      ktd.textContent = k;
      const vtd = document.createElement("td");
      vtd.className = "exp-kv-val";
      if (v !== null && v !== undefined && String(v) !== "") {
        if (isLinkable(kind, k, v)) {
          const a = document.createElement("a");
          a.className = "exp-link";
          a.href = "";
          a.textContent = String(v);
          a.onclick = (e) => {
            e.preventDefault();
            setHash(linkTarget(kind, k, v));
          };
          vtd.appendChild(a);
        } else {
          vtd.textContent = String(v);
        }
      } else {
        vtd.textContent = "—";
      }
      tr.append(ktd, vtd);
      table.appendChild(tr);
    });
    wrap.appendChild(table);
    return wrap;
  }

  function isLinkable(kind, key, value) {
    const v = String(value);
    if (key === "Hash" && (kind === "block" || kind === "tx")) return true;
    if (key === "Block" && v !== "—") return v.length === 64;
    if (key === "SCID" && v.length === 64) return true;
    if (key === "Address" && v.startsWith("dero1")) return v !== (null) && kind !== "address";
    if (key === "Signer" && v.startsWith("dero1")) return true;
    return false;
  }

  function linkTarget(kind, key, value) {
    const v = String(value);
    if (key === "Hash") return kind + "/" + v;
    if (key === "Block") return "block/" + v;
    if (key === "SCID") return "sc/" + v;
    if (key === "Signer") return "address/" + v;
    if (key === "Address") return "address/" + v;
    return "";
  }

  function panel(title, body) {
    const el = document.createElement("div");
    el.className = "exp-panel";
    const h = document.createElement("div");
    h.className = "exp-panel-title";
    h.textContent = title;
    el.append(h, body);
    return el;
  }

  function explorerTable(title, cols, onMore, moreLabel) {
    const el = document.createElement("div");
    el.className = "exp-section";
    const head = document.createElement("div");
    head.className = "exp-section-head";
    const span = document.createElement("span");
    span.className = "exp-section-title";
    span.textContent = title;
    head.appendChild(span);
    if (onMore) {
      const a = document.createElement("a");
      a.className = "exp-link exp-section-more";
      a.href = "";
      a.textContent = moreLabel + " →";
      a.onclick = (e) => {
        e.preventDefault();
        onMore();
      };
      head.appendChild(a);
    }
    el.appendChild(head);
    el.appendChild(tableWrapEl(cols));
    return el;
  }

  function tableWrapEl(cols) {
    const wrap = document.createElement("div");
    wrap.className = "exp-table-wrap";
    const table = document.createElement("table");
    table.className = "exp-table";
    const thead = document.createElement("thead");
    const tr = document.createElement("tr");
    cols.forEach((c) => {
      const th = document.createElement("th");
      th.textContent = c;
      tr.appendChild(th);
    });
    thead.appendChild(tr);
    const tbody = document.createElement("tbody");
    table.append(thead, tbody);
    wrap.appendChild(table);
    return wrap;
  }

  function emptyRow(msg) {
    const tr = document.createElement("tr");
    tr.className = "exp-empty-row";
    const td = document.createElement("td");
    td.colSpan = 99;
    td.textContent = msg;
    tr.appendChild(td);
    return tr;
  }

  function kvTable(obj) {
    const wrap = tableWrapEl(["KEY", "VALUE"]);
    const tbody = wrap.querySelector("tbody");
    Object.keys(obj)
      .slice(0, 100)
      .forEach((k) => {
        const tr = document.createElement("tr");
        const ktd = document.createElement("td");
        ktd.className = "mono";
        ktd.textContent = k;
        const vtd = document.createElement("td");
        vtd.className = "exp-kv-val";
        vtd.textContent = obj[k];
        tr.append(ktd, vtd);
        tbody.appendChild(tr);
      });
    return wrap;
  }

  // Tokenizer for DERO smart-contract source (C-like or BASIC
  // dialect, keyword matching is case-insensitive). Produces HTML
  // with hl-* spans; see the .hl-* rules in style.css.
  const DERO_KEYWORDS = new Set([
    "if", "else", "for", "while", "do", "func", "function", "return",
    "goto", "then", "end", "endif", "import", "package", "var",
    "const", "int", "uint64", "uint32", "string", "bool", "true",
    "false", "nil", "null", "contract", "handler", "initialized",
    "balance", "ringsize", "signature", "destination", "amount",
    "code", "secret", "storage", "store", "load", "call", "shadow",
    "split", "strlen", "new",
  ]);

  function highlightDERO(code) {
    const esc = (s) =>
      s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
    const out = [];
    let pos = 0;
    const n = code.length;

    while (pos < n) {
      const c = code[pos];
      const rest = code.slice(pos);

      if (c === "/" && code[pos + 1] === "/") {
        const m = rest.match(/^\/\/[^\n]*/);
        if (m) { out.push('<span class="hl-cmt">' + esc(m[0]) + "</span>"); pos += m[0].length; continue; }
      }
      if (c === "/" && code[pos + 1] === "*") {
        const m = rest.match(/^\/\*[\s\S]*?\*\//);
        if (m) { out.push('<span class="hl-cmt">' + esc(m[0]) + "</span>"); pos += m[0].length; continue; }
      }
      if (c === '"') {
        const m = rest.match(/^"(?:[^"\\]|\\.)*"/);
        if (m) { out.push('<span class="hl-str">' + esc(m[0]) + "</span>"); pos += m[0].length; continue; }
      }
      if (c >= "0" && c <= "9") {
        const m = rest.match(/^(?:0x[0-9a-fA-F]+|\d+(?:\.\d+)?)/);
        if (m) { out.push('<span class="hl-num">' + esc(m[0]) + "</span>"); pos += m[0].length; continue; }
      }
      if (/[A-Za-z_]/.test(c)) {
        const m = rest.match(/^[A-Za-z_]\w*/);
        if (m) {
          const cls = DERO_KEYWORDS.has(m[0].toLowerCase()) ? "hl-kw" : "";
          out.push(cls ? '<span class="' + cls + '">' + esc(m[0]) + "</span>" : esc(m[0]));
          pos += m[0].length;
          continue;
        }
      }
      if (/[\s]/.test(c)) {
        let ws = "";
        while (pos < n && /\s/.test(code[pos])) { ws += code[pos]; pos++; }
        out.push(esc(ws));
        continue;
      }
      if ("+-*/%=!<>&|^~?:;,.()[]{}@#".includes(c)) {
        out.push('<span class="hl-op">' + esc(c) + "</span>");
        pos++;
        continue;
      }
      out.push(esc(c));
      pos++;
    }
    return out.join("");
  }

  function preCode(txt) {
    const pre = document.createElement("pre");
    pre.className = "exp-code";
    const code = document.createElement("code");
    code.innerHTML = highlightDERO(txt);
    pre.appendChild(code);
    return pre;
  }

  // ---- search ----

  function doSearch() {
    const q = els.search.value.trim();
    if (!q) return;
    showLoading("Resolving " + q + "…");
    fetch("/api/explorer/search?q=" + encodeURIComponent(q), { headers: { Accept: "application/json" } })
      .then((r) => r.json())
      .then((d) => {
        if (!d || d.ok === false || !d.result || d.result.type === "none") {
          renderNoResult(q);
          return;
        }
        const res = d.result;
        if (res.type === "block") setHash("block/" + res.id);
        else if (res.type === "tx") setHash("tx/" + res.id);
        else if (res.type === "sc") setHash("sc/" + res.id);
        else if (res.type === "address") setHash("address/" + res.address + (res.name ? "/" + encodeURIComponent(res.name) : ""));
      })
      .catch(() => renderError());
  }

  // Public hook for other modules (e.g. search.js) to run an explorer
  // search for a given query — used by the "Open in explorer" result action.
  window.openExplorerSearch = function (q) {
    if (els.search && q) els.search.value = q;
    doSearch();
  };

  function renderNoResult(q) {
    els.content.replaceChildren();
    const el = document.createElement("div");
    el.className = "exp-error";
    const p = document.createElement("p");
    p.textContent = "No match for \u201c" + q + "\u201d. Try a height, 64-hex hash/SCID, dero1 address, or registered name.";
    const btn = document.createElement("button");
    btn.className = "exp-retry";
    btn.textContent = "Clear";
    btn.onclick = () => {
      els.search.value = "";
      setHash("");
    };
    el.append(p, btn);
    els.content.appendChild(el);
  }

  // ---- formatting ----

  function fmtNum(n) {
    if (n === null || n === undefined) return "—";
    if (typeof n === "number" || typeof n === "bigint") return n.toLocaleString("en-US");
    return String(n);
  }

  function cvar(v) {
    if (v === null || v === undefined || v === "") return "—";
    return String(v);
  }

  function short(s) {
    if (!s) return "";
    if (s.length <= 18) return s;
    return s.slice(0, 12) + "…" + s.slice(-6);
  }

  function dateFromMs(ms) {
    return new Date(ms).toISOString().replace("T", " ").slice(0, 19) + " UTC";
  }

  function agoFromMs(msStr) {
    const ms = Number(msStr);
    if (!ms) return "";
    const s = Math.max(0, Math.floor((Date.now() - ms) / 1000));
    if (s < 60) return s + "s ago";
    if (s < 3600) return Math.floor(s / 60) + "m ago";
    if (s < 86400) return Math.floor(s / 3600) + "h ago";
    return Math.floor(s / 86400) + "d ago";
  }
})();