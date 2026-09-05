import "./HexIcon.js";
import "./HexIcon.css";

export default {
  title: "Foundations/HexIcon",
  parameters: {
    llm: {
      description: "DERO/SCID hexagon placeholder used when no app icon is available.",
      useWhen: ["Missing app icon in result rows, latest finds, or sidebar"],
      avoidWhen: ["Real app icons exist for the item"],
      related: ["ResultRow", "LatestFindItem"],
    },
  },
};

export const Default = () => {
  const wrap = document.createElement("div");
  wrap.className = "icon-slot";
  const UI = window.UI;
  wrap.appendChild(UI.HexIcon());
  return wrap;
};

export const DarkMode = () => {
  document.documentElement.dataset.theme = "dark";
  return Default();
};