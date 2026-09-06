(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  // ExplorerBlockRow renders a table row for one block header.
  // opts: { onOpen(header) } invoked when the hash links are clicked.
  UI.ExplorerBlockRow = function ExplorerBlockRow(h, opts = {}) {
    const tr = document.createElement("tr");
    tr.className = "explorer-block-row";

    const topo = document.createElement("td");
    topo.textContent = String(h.topoheight);
    topo.title = "Topological height";
    tr.appendChild(topo);

    const height = document.createElement("td");
    height.textContent = String(h.height);
    height.title = "Block height";
    tr.appendChild(height);

    const hash = document.createElement("td");
    const a = document.createElement("a");
    a.className = "hash-link";
    a.href = "#";
    a.textContent = String(h.hash).slice(0, 18) + "…";
    a.title = h.hash;
    a.onclick = (e) => {
      e.preventDefault();
      if (opts.onOpen) opts.onOpen(h);
    };
    hash.appendChild(a);
    tr.appendChild(hash);

    const txs = document.createElement("td");
    txs.textContent = String(h.txcount ?? 0);
    txs.className = "num";
    tr.appendChild(txs);

    const reward = document.createElement("td");
    reward.textContent = h.reward_dero || "0.00000";
    reward.className = "num mono";
    reward.title = "Reward (DERO)";
    tr.appendChild(reward);

    const size = document.createElement("td");
    size.textContent = "n/a";
    size.className = "num";
    tr.appendChild(size);

    const age = document.createElement("td");
    age.textContent = ageOf(h.timestamp);
    age.className = "muted";
    tr.appendChild(age);

    return tr;
  };

  function ageOf(ms) {
    if (!ms) return "";
    const s = Math.max(0, Math.floor((Date.now() - ms) / 1000));
    if (s < 60) return s + "s ago";
    if (s < 3600) return Math.floor(s / 60) + "m ago";
    if (s < 86400) return Math.floor(s / 3600) + "h ago";
    return Math.floor(s / 86400) + "d ago";
  }
})();