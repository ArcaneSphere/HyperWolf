import "./LogEntry.js";
import "./LogEntry.css";

const sample = (level, message) => ({ timestamp: Date.now(), level, message });

export default {
  title: "Components/LogEntry",
  parameters: {
    llm: {
      description: "Single terminal log line: timestamp, level pill, message.",
      useWhen: ["Rendering node/TELA/Gnomon log streams"],
      avoidWhen: ["Structured data tables"],
      related: ["StatusDot", "LogContainer"],
    },
  },
};

export const Info = () => {
  const UI = window.UI;
  return UI.LogEntry(sample("INFO", "Connected to node at height 1848291")).trim();
};

export const Warn = () => {
  const UI = window.UI;
  return UI.LogEntry(sample("WARN", "Sync is 42 blocks behind")).trim();
};

export const Error = () => {
  const UI = window.UI;
  return UI.LogEntry(sample("ERROR", "Failed to fetch SCID registry: timeout")).trim();
};

export const Success = () => {
  const UI = window.UI;
  return UI.LogEntry(sample("SUCCESS", "Indexer completed successfully")).trim();
};

export const List = () => {
  const UI = window.UI;
  const entries = [sample("INFO", "TELA apps: 137"), sample("SUCCESS", "Wait for new block"), sample("WARN", "Daemon not responding")].map(
    (e) => UI.LogEntry(e).trim()
  );
  return entries.join("");
};