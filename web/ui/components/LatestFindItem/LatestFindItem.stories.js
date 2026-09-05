import "./LatestFindItem.js";
import "./LatestFindItem.css";
import "../HexIcon/HexIcon.js";

const SCID = "abc123def4567890abc123def4567890abc123def4567890abc123def4567890";

export default {
  title: "Search/LatestFindItem",
  parameters: {
    llm: {
      description: "Row in the latest-finds list: icon slot, uppercase domain, app name, SCID and a meta line that can carry a NEW tag and install height.",
      useWhen: ["Recent app lists with icon + NEW highlight", "Search-card latest finds (icon/meta/scid hidden via SearchCard scope)"],
      avoidWhen: ["Full search results with ratings and bookmark controls"],
      related: ["HexIcon", "ResultRow", "SearchCard", "SearchSuggestion"],
    },
  },
};

const wrap = (rows) => {
  const el = document.createElement("div");
  el.style.maxWidth = "560px";
  rows.forEach((r) => el.appendChild(r));
  return el;
};

const base = {
  scid: SCID,
  name: "Vault.tela",
  durl: "vault.tela",
  iconURL: "",
  install_height: 1234567,
};

export const Default = () => {
  const UI = window.UI;
  return UI.LatestFindItem({ ...base, isNew: true }, { onClick: (a) => console.log("open", a.scid) });
};

export const WithIcon = () => {
  const UI = window.UI;
  return UI.LatestFindItem({ ...base, name: "Games.tela", iconURL: "https://example.com/icon.png", isNew: false });
};

export const Stacked = () => {
  const UI = window.UI;
  return wrap([
    UI.LatestFindItem({ ...base, isNew: true }),
    UI.LatestFindItem({ ...base, name: "Explorer.tela", durl: "explorer.tela" }),
    UI.LatestFindItem({ ...base, name: "Chat.tela", durl: "chat.tela", install_height: 987654 }),
  ]);
};

export const NoHeight = () => {
  const UI = window.UI;
  return UI.LatestFindItem({ ...base, install_height: 0 });
};