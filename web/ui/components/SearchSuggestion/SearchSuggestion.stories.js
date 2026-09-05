import "./SearchSuggestion.js";
import "./SearchSuggestion.css";

export default {
  title: "Search/SearchSuggestion",
  parameters: {
    llm: {
      description: "Single autocomplete row shown in the search dropdown: primary domain URL with a secondary name/SCID meta line.",
      useWhen: ["Autocomplete dropdowns under a search box", "Compact domain listings"],
      avoidWhen: ["Long message bodies", "When results need icons or ratings"],
      related: ["SearchCard", "ResultRow", "LatestFindItem"],
    },
  },
};

const dropdown = (children) => {
  const el = document.createElement("div");
  el.style.maxWidth = "520px";
  el.style.fontFamily = "-apple-system, BlinkMacSystemFont, Segoe UI, Roboto, sans-serif";
  children.forEach((c) => el.appendChild(c));
  return el;
};

const items = [
  { dURL: "vault.tela", nameHdr: "Vault — secure wallet" },
  { dURL: "explorer.tela", nameHdr: "scid9682a90ef..." },
  { dURL: "games.tela", nameHdr: "Casino & instant games" },
];

export const Default = () => {
  const UI = window.UI;
  return dropdown(items.map((r) => UI.SearchSuggestion(r, { onSelect: (x) => console.log("select", x.dURL) })));
};

export const KeyboardSelected = () => {
  const UI = window.UI;
  const rows = items.map((r) => UI.SearchSuggestion(r));
  rows[1].classList.add("selected");
  return dropdown(rows);
};

export const LongDomain = () => {
  const UI = window.UI;
  return dropdown([UI.SearchSuggestion({ dURL: "this-is-an-extremely-long-subdomain.name.tela", nameHdr: "scid00ac– de 64-char SCID overflows into ellipsis", scid: "x" })]);
};