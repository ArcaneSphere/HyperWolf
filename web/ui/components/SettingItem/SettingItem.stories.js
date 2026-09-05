import "./SettingItem.js";
import "./SettingItem.css";

export default {
  title: "Settings/SettingItem",
  parameters: {
    llm: {
      description:
        "One settings card. Stack layout: uppercase accent label, body content, trailing description. Row layout: label + description left, control (toggle/button) right. Hover lifts the card. Responsive: at <=640px the row stacks and padding shrinks.",
      useWhen: [
        "Settings cards with a label and a description",
        "Toggle rows (label + description left, toggle right)",
      ],
      avoidWhen: [
        "Search toolbar filters — use SearchControlsRow",
        "Plain file-folder info blocks — use info-box (page shell)",
      ],
      related: ["SearchControlsRow", "ToggleSwitch", "Popover"],
    },
  },
};

function makeToggle(labelText) {
  const toggle = document.createElement("label");
  toggle.className = "toggle-switch";
  const input = document.createElement("input");
  input.type = "checkbox";
  input.checked = true;
  const slider = document.createElement("span");
  slider.className = "toggle-slider";
  toggle.append(input, slider);
  return toggle;
}

export const StackInput = () => {
  const wrap = document.createElement("div");
  const input = document.createElement("input");
  input.type = "text";
  input.placeholder = "e.g. 127.0.0.1:10102";
  const { element } = window.UI.SettingItem({
    label: "Default Node",
    content: input,
    description: "Auto-connected when the app starts",
  });
  wrap.appendChild(element);
  return wrap;
};

export const RowToggle = () => {
  const wrap = document.createElement("div");
  const { element } = window.UI.SettingItem({
    label: "Auto-Connect",
    description: "Connect to the default node on startup",
    control: makeToggle(),
  });
  wrap.appendChild(element);
  return wrap;
};

export const RowGroup = () => {
  const wrap = document.createElement("div");
  const { element: a } = window.UI.SettingItem({
    label: "Show Search Cards",
    description: "Show bottom cards panel on the Search page",
    control: makeToggle(),
  });
  const { element: b } = window.UI.SettingItem({
    label: "Theme Mode",
    description: "Switch between dark and light theme",
    control: makeToggle(),
  });
  wrap.append(a, b);
  return wrap;
};