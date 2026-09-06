(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  // ExplorerTxRow renders a table row for one transaction.
  // opts: { onOpenTx(tx), onOpenBlock(tx) }
  UI.ExplorerTxRow = function ExplorerTxRow(tx, opts = {}) {
    const tr = document.createElement("tr");
    tr.className = "explorer-tx-row";
    if (tx.in_pool) tr.classList.add("in-pool");

    const hash = document.createElement("td");
    const a = document.createElement("a");
    a.className = "tx-hash-link";
    a.href = "#";
    a.textContent = shortHash(tx.hash);
    a.title = tx.hash;
    a.onclick = (e) => {
      e.preventDefault();
      if (opts.onOpenTx) opts.onOpenTx(tx);
    };
    hash.appendChild(a);
    tr.appendChild(hash);

    const type = document.createElement("td");
    const badge = document.createElement("span");
    badge.className = "tx-type tx-type-" + typeClass(tx.type);
    badge.textContent = tx.type || "—";
    badge.title = tx.type || "";
    type.appendChild(badge);
    tr.appendChild(type);

    const fee = document.createElement("td");
    fee.textContent = tx.in_pool ? (tx.fee || "—") : (tx.fee || "—");
    fee.className = "num mono";
    fee.title = tx.fee_uint64 ? "Fee: " + tx.fee + " DERO" : "Fee hidden by node";
    tr.appendChild(fee);

    const ring = document.createElement("td");
    ring.textContent = tx.ring_size ? String(tx.ring_size) : "—";
    ring.className = "num";
    tr.appendChild(ring);

    const size = document.createElement("td");
    size.textContent = tx.size ? sizeKB(tx.size) : "—";
    size.className = "num";
    size.title = tx.size_bytes ? tx.size_bytes + " bytes" : "";
    tr.appendChild(size);

    const signer = document.createElement("td");
    signer.textContent = shortAddress(tx.signer) || "—";
    signer.className = "muted";
    signer.title = tx.signer || "";
    tr.appendChild(signer);

    const block = document.createElement("td");
    if (tx.in_pool) {
      block.textContent = "pool";
      block.className = "muted pool";
    } else if (tx.valid_block) {
      const ba = document.createElement("a");
      ba.className = "block-link";
      ba.href = "#";
      ba.textContent = tx.height ? String(tx.height) : shortHash(tx.valid_block);
      ba.title = "Block " + (tx.height || "") + "\n" + tx.valid_block;
      ba.onclick = (e) => {
        e.preventDefault();
        if (opts.onOpenBlock) opts.onOpenBlock(tx);
      };
      block.appendChild(ba);
    } else {
      block.textContent = tx.height ? String(tx.height) : "—";
    }
    tr.appendChild(block);

    const time = document.createElement("td");
    time.textContent = tx.in_pool ? poolAge(tx) : (tx.age || "");
    time.className = "muted";
    tr.appendChild(time);

    return tr;
  };

  function shortHash(h) {
    if (!h) return "…";
    return h.slice(0, 8) + "…" + h.slice(-8);
  }

  function shortAddress(a) {
    if (!a) return "";
    if (a.length <= 14) return a;
    return a.slice(0, 9) + "…" + a.slice(-4);
  }

  function sizeKB(sizeStr) {
    const n = parseFloat(sizeStr);
    if (Number.isFinite(n)) return n.toFixed(3) + " kB";
    return sizeStr;
  }

  function poolAge(tx) {
    return tx.pool_age || "pooled";
  }

  function typeClass(type) {
    const t = (type || "").toUpperCase();
    if (t === "COINBASE") return "coinbase";
    if (t === "SC") return "sc";
    if (t === "BURN") return "burn";
    if (t === "REGISTRATION") return "reg";
    if (t === "PREMINE") return "premine";
    return "normal";
  }
})();