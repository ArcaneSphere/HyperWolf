import "./ExplorerBlockRow.js";
import "./ExplorerBlockRow.css";

export default {
  title: "Explorer/ExplorerBlockRow",
  parameters: {
    llm: {
      description: "Table row for a DERO block header: topoheight, height, short hash link, tx count, reward, age.",
      useWhen: ["Block list tables in the explorer (overview + blocks view)"],
      avoidWhen: ["Transactions — use ExplorerTxRow", "Non-table block cards"],
      related: ["ExplorerStat", "ExplorerTxRow"],
    },
  },
};

export const Headers = () => {
  const UI = window.UI;
  const now = Date.now();
  const table = document.createElement("table");
  table.style.width = "100%";
  table.style.borderCollapse = "collapse";
  const mk = (topo, height, hash, txcount, reward, secs) => ({
    topoheight: topo,
    height,
    hash,
    txcount,
    reward_dero: reward,
    timestamp: now - secs * 1000,
  });
  const rows = [
    mk(100, 100, "0".repeat(64), 2, "0.25000", 30),
    mk(99, 99, "1".repeat(64), 0, "0.12500", 120),
    mk(98, 98, "2".repeat(64), 5, "0.50000", 3600),
  ];
  rows.forEach((h) => table.appendChild(UI.ExplorerBlockRow(h, { onOpen: () => {} })));
  return table;
};