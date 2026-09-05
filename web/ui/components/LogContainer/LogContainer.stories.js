import "./LogContainer.js";
import "./LogContainer.css";
import "../LogEntry/LogEntry.js";
import "../LogEntry/LogEntry.css";

export default {
  title: "Server/LogContainer",
  parameters: {
    llm: {
      description:
        "Live log view shell: level filter + refresh/clear actions + auto-follow checkbox + scrollable monospace terminal panel. Builds the same ids (#log-level-filter, #log-refresh-btn, #log-clear-btn, #log-auto-scroll, #log-container) the app wires on DOMContentLoaded; pass UI.LogEntry rows as children.",
      useWhen: [
        "A live log/event stream on the server page",
        "Recreating the activity-log panel",
      ],
      avoidWhen: [
        "Static text listings — use a plain log-container or info-box",
        "Search result lists — use ResultRow/SearchCard",
      ],
      related: ["LogEntry", "StatusCard", "Browser", "PopupSearch"],
    },
  },
};

export const Empty = () => {
  const { element } = window.UI.LogContainer();
  return element;
};

export const WithEntries = () => {
  const UI = window.UI;
  const lines = [
    "2026-09-05 14:41:23 [INFO]  node connected",
    "2026-09-05 14:41:23 [INFO]  tela apps scan started",
    "2026-09-05 14:41:22 [WARN]  sync behind by 12 blocks",
    "2026-09-05 14:41:19 [DEBUG] websocket subscription ok",
    "2026-09-05 14:41:18 [INFO]  fastsync checkpoint loaded",
    "2026-09-05 14:41:17 [ERROR] rpc timeout, retrying",
  ];
  const { element } = UI.LogContainer({ children: lines.map(UI.LogEntry) });
  return element;
};