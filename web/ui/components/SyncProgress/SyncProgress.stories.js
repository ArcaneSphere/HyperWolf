import "./SyncProgress.js";
import "./SyncProgress.css";

export default {
  title: "Server/SyncProgress",
  parameters: {
    llm: {
      description:
        "Chain-sync progress block: state label, thin 8px progress bar with animated fill transition, ETA label. Supports indeterminate mode (sliding 30% segment) via setIndeterminate(true) or percent:-1. API: setPercent, setLabel, setEta, setIndeterminate.",
      useWhen: [
        "Sync/indexing progress indicators with percentage + ETA",
        "Indeterminate progress (chain indexing start)",
      ],
      avoidWhen: [
        "Determinate loading spinners — use StatusDot / a plain spinner",
        "Overall sync status summary — use StatusCard for the card wrapper",
      ],
      related: ["StatusCard", "StatusDot"],
    },
  },
};

export const Waiting = () => {
  const { element } = window.UI.SyncProgress({ label: "Waiting...", eta: "" });
  return element;
};

export const Indexing = () => {
  const { element } = window.UI.SyncProgress({
    label: "⏳ Syncing chain...",
    eta: "",
    percent: -1,
  });
  return element;
};

export const Halfway = () => {
  const { element } = window.UI.SyncProgress({
    label: "42.3% chain synced",
    eta: "~14m remaining",
    percent: 42.3,
  });
  return element;
};

export const Synced = () => {
  const { element } = window.UI.SyncProgress({
    label: "Chain synced",
    eta: "",
    percent: 100,
  });
  return element;
};

export const InsideStatusCard = () => {
  const { element } = window.UI.SyncProgress({
    label: "42.3% chain synced",
    eta: "~14m remaining",
    percent: 42.3,
  });
  return window.UI.StatusCard({
    header: "Sync",
    rows: [
      { label: "Indexed", value: "1_234_567" },
      { label: "Chain", value: "1_345_000" },
    ],
    children: [element],
  }).element;
};