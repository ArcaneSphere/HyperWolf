import "./ResultRow.js";
import "./ResultRow.css";
import "../HexIcon/HexIcon.js";

const sample = {
  scid: "abc123def4567890abc123def4567890abc123def4567890abc123def4567890",
  nameHdr: "Vault.tela",
  dURL: "vault.tela",
  descrHdr: "Secure cross-platform DERO wallet with instant payments.",
  iconURL: "",
  likes: 42,
  dislikes: 3,
  average: 87,
  ratingsLoaded: true,
};

export default {
  title: "Search/ResultRow",
  parameters: {
    llm: {
      description: "Compact representation of one search result with icon, domain, name, SCID, description, rating and bookmark.",
      useWhen: ["Displaying search results", "Listing DERO TELA apps with ratings"],
      avoidWhen: ["Displaying more than ~20 items", "Showing complex tabular data"],
      related: ["HexIcon", "SearchCard", "StatusBadge", "LatestFindItem"],
    },
  },
};

export const Default = () => {
  const UI = window.UI;
  const root = document.createElement("div");
  root.style.maxWidth = "720px";
  root.appendChild(UI.ResultRow(sample, { onClick: (s) => console.log("load", s) }));
  return root;
};

export const Bookmarked = () => {
  const UI = window.UI;
  const root = document.createElement("div");
  root.style.maxWidth = "720px";
  root.appendChild(UI.ResultRow(sample, { bookmarked: true }));
  return root;
};

export const LoadingRatings = () => {
  const UI = window.UI;
  const root = document.createElement("div");
  root.style.maxWidth = "720px";
  root.appendChild(UI.ResultRow({ ...sample, ratingsLoaded: false }));
  return root;
};

export const KeyboardSelected = () => {
  const UI = window.UI;
  const root = document.createElement("div");
  root.style.maxWidth = "720px";
  const row = UI.ResultRow(sample, {});
  row.classList.add("keyboard-selected");
  root.appendChild(row);
  return root;
};