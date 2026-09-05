import "./StatusCard.js";
import "./StatusCard.css";

export default {
  title: "Server/StatusCard",
  parameters: {
    llm: {
      description:
        "Dashboard status card: header, monospace label/value rows, optional expandable <details>, and a .card-apps variant with a centered large count. Grid wrapper .status-grid stacks a 1-col responsive layout.",
      useWhen: [
        "Server overview panels (Network / TELA Apps / Sync Status)",
        "Compact key-value readouts with optional collapsible details",
      ],
      avoidWhen: [
        "Search-result bottom panels — use SearchCard",
        "Long-form content — use SettingItem / info-box",
      ],
      related: ["SearchCard", "SettingItem", "StatusDot", "Toast"],
    },
  },
};

export const Network = () => {
  const { element } = window.UI.StatusCard({
    header: "Network",
    rows: [
      { label: "Node", value: "—" },
      { label: "Connected", value: "—" },
    ],
    expand: {
      summary: "Details",
      rows: [
        { label: "Version", value: "—" },
        { label: "Uptime", value: "—" },
        { label: "Difficulty", value: "—" },
        { label: "Mempool", value: "0" },
      ],
    },
  });
  return element;
};

export const TelaApps = () => {
  const { element } = window.UI.StatusCard({
    className: "card-apps",
    header: "TELA Apps",
    rows: [
      { label: "", value: "—", lg: true },
      { label: "", value: "discovered" },
      { label: "", value: "—" },
    ],
  });
  return element;
};

export const Sync = () => {
  const { element } = window.UI.StatusCard({
    header: "Sync",
    rows: [
      { label: "Indexed", value: "—" },
      { label: "Chain", value: "—" },
    ],
  });
  return element;
};

export const Grid = () => {
  const grid = document.createElement("div");
  grid.className = "status-grid";
  grid.appendChild(Network());
  grid.appendChild(TelaApps());
  grid.appendChild(Sync());
  return grid;
};