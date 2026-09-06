import "./ExplorerTxRow.js";
import "./ExplorerTxRow.css";

export default {
  title: "Explorer/ExplorerTxRow",
  parameters: {
    llm: {
      description: "Table row for a DERO transaction: short hash link, type badge, fee, ring size, size, signer, block link, age. Pooled rows get a leading status dot.",
      useWhen: ["Tx tables in the explorer (block detail, mempool, address history)"],
      avoidWhen: ["Blocks — use ExplorerBlockRow"],
      related: ["ExplorerStat", "ExplorerBlockRow"],
    },
  },
};

export const Types = () => {
  const UI = window.UI;
  const table = document.createElement("table");
  table.style.width = "100%";
  table.style.borderCollapse = "collapse";
  const tx = (type, hash, opts = {}) =>
    Object.assign({ type, hash, signer: "deto1qyfound1500xct8v6t" + type.toLowerCase() + "deadbeef", size: "1.234 kB", size_bytes: 1234 }, opts);
  const rows = [
    tx("COINBASE", "a".repeat(64)),
    tx("SC", "b".repeat(64)),
    tx("BURN", "c".repeat(64)),
    tx("REGISTRATION", "d".repeat(64)),
    tx("NORMAL", "e".repeat(64), { in_pool: true, fee: "0.00001", ring_size: 2, pool_age: "2m" }),
    tx("NORMAL", "f".repeat(64), { ring_size: 8, fee: "0.04000", height: 100, valid_block: "1".repeat(64), age: "5h" }),
  ];
  rows.forEach((t) => table.appendChild(UI.ExplorerTxRow(t, { onOpenTx: () => {}, onOpenBlock: () => {} })));
  return table;
};