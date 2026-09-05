import "./StatusDot.js";
import "./StatusDot.css";

export default {
  title: "Foundations/StatusDot",
  parameters: {
    llm: {
      description: "Small colored status indicator (8px dot) for live state.",
      useWhen: ["Showing connected/error/warning/pending state inline"],
      avoidWhen: ["Standalone status text without a dot"],
      related: ["Toast", "Sidebar", "StatusCard"],
    },
  },
};

const states = ["connected", "error", "warning", "pending"];

export const AllStates = () => {
  const UI = window.UI;
  const root = document.createElement("div");
  root.style.display = "flex";
  root.style.flexDirection = "column";
  root.style.gap = "8px";
  root.style.padding = "16px";
  for (const s of states) {
    const line = document.createElement("div");
    line.append(UI.StatusDot(s), document.createTextNode(" " + s));
    root.appendChild(line);
  }
  return root;
};