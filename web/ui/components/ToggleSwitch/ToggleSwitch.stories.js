import "./ToggleSwitch.js";
import "./ToggleSwitch.css";

export default {
  title: "Components/ToggleSwitch",
  parameters: {
    llm: {
      description: "Boolean on/off control rendered as a sliding switch. The input is a real checkbox for a11y.",
      useWhen: ["Single boolean settings (auto-connect, show top bar, check updates)"],
      avoidWhen: ["Three-state or numeric options", "Requiring immediate save confirmation"],
      related: ["StatusCard", "Popover", "TagChip"],
    },
  },
};

export const Off = () => {
  const UI = window.UI;
  return UI.ToggleSwitch(false);
};

export const On = () => {
  const UI = window.UI;
  return UI.ToggleSwitch(true);
};

export const Rows = () => {
  const UI = window.UI;
  const wrap = document.createElement("div");
  wrap.style.display = "flex";
  wrap.style.flexDirection = "column";
  wrap.style.gap = "12px";
  wrap.style.alignItems = "flex-start";
  const group = (labelText, on) => {
    const row = document.createElement("div");
    row.style.display = "flex";
    row.style.alignItems = "center";
    row.style.gap = "10px";
    const label = document.createElement("span");
    label.style.fontSize = "13px";
    label.textContent = labelText;
    row.append(label, UI.ToggleSwitch(on));
    return row;
  };
  wrap.append(group("Auto-connect", true), group("Show top bar", false), group("Check updates", true));
  return wrap;
};