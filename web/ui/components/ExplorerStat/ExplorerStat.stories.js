import "./ExplorerStat.js";
import "./ExplorerStat.css";

export default {
  title: "Explorer/ExplorerStat",
  parameters: {
    llm: {
      description: "Stat tile for the chain explorer: mono value + uppercase label + muted sub-line.",
      useWhen: ["Explorer overview stat grids", "Any key/value/status tile in a panel"],
      avoidWhen: ["Small inline status chips — use StatusDot or TagChip"],
      related: ["StatusCard", "TagChip", "ExplorerBlockRow", "ExplorerTxRow"],
    },
  },
};

export const Default = () => {
  const UI = window.UI;
  const root = document.createElement("div");
  root.style.display = "grid";
  root.style.gridTemplateColumns = "repeat(3, 1fr)";
  root.style.gap = "12px";
  root.style.maxWidth = "640px";
  root.append(
    UI.ExplorerStat({ label: "Network", value: "Mainnet", sub: "derod 3.6.0" }),
    UI.ExplorerStat({ label: "Topo height", value: "7,580,807", sub: "1 in 18.4s" }),
    UI.ExplorerStat({ label: "Mempool", value: "2 txs", sub: "dynamic fee 0.00000" }),
  );
  return root;
};