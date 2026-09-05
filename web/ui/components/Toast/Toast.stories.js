import "./Toast.js";
import "./Toast.css";
import "../StatusDot/StatusDot.js";

export default {
  title: "Components/Toast",
  parameters: {
    llm: {
      description: "Transient notification with status dot, message and dismiss button.",
      useWhen: ["Reporting operation outcomes briefly (connected, error, warning, pending)"],
      avoidWhen: ["Persistent inline status", "Long-form messages"],
      related: ["StatusDot", "StatusCard"],
    },
  },
};

const wrap = (el) => {
  const box = document.createElement("div");
  box.className = "status-box";
  box.style.position = "static";
  box.appendChild(el);
  return box;
};

export const Connected = () => {
  const UI = window.UI;
  return wrap(UI.Toast("connected", "Connected to node.dero.example"));
};

export const Error = () => {
  const UI = window.UI;
  return wrap(UI.Toast("error", "Connection to node failed"));
};

export const Warning = () => {
  const UI = window.UI;
  return wrap(UI.Toast("warning", "Sync is falling behind"));
};

export const Pending = () => {
  const UI = window.UI;
  return wrap(UI.Toast("pending", "Waiting for node response…"));
};