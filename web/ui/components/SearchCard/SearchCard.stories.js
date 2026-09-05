import "./SearchCard.js";
import "./SearchCard.css";

export default {
  title: "Search/SearchCard",
  parameters: {
    llm: {
      description: "Bottom-panel card shell with a labelled header row (title + optional trailing badge/button) and a body container. Ships as the fixed 3-up `.search-cards` grid; each card stacks full-width below 640px.",
      useWhen: [
        "Building a search-page bottom card panel (Latest Finds, Updates & News, Node Status)",
        "Any fixed bottom information card that needs a header + body",
      ],
      avoidWhen: [
        "Inline cards in a normal document flow — those should use plain panels",
        "Card content that is itself a self-contained component (use LatestFindItem, ResultRow, … in the body)",
      ],
      related: ["LatestFindItem", "ResultRow", "SearchSuggestion", "LiveInfoCard", "StatusCard"],
    },
  },
};

export const LatestFinds = () => {
  const UI = window.UI;
  const badge = document.createElement("span");
  badge.className = "latest-finds-new";
  badge.textContent = "✨ 2 new";
  const body = [
    UI.LatestFindItem({
      scid: "a6832a5a09b82dc4b1034fd726b118da1df8ca9ad33e76bee4563e3f69d1d99a",
      name: "Dero World",
      durl: "dero.world",
      isNew: true,
      install_height: 2310000,
    }),
    UI.LatestFindItem({
      scid: "b6832a5a09b82dc4b1034fd726b118da1df8ca9ad33e76bee4563e3f69d1d99a",
      name: "Vault",
      durl: "vault.tela",
      isNew: false,
      install_height: 2274000,
    }),
  ];
  return UI.SearchCard({
    className: "latest-finds",
    title: "Latest Finds",
    headerExtra: badge,
    bodyId: "latest-finds-list",
    children: body,
  }).element;
};

export const UpdatesAndNews = () => {
  const UI = window.UI;
  const refresh = document.createElement("button");
  refresh.className = "icon-btn";
  refresh.title = "Refresh";
  refresh.textContent = "⟳";
  const msg = document.createElement("div");
  msg.className = "update-current-message";
  const icon = document.createElement("span");
  icon.textContent = "✅";
  const label = document.createElement("span");
  label.textContent = "You're on the latest version (0.11.0)";
  msg.append(icon, label);
  return UI.SearchCard({
    className: "live-info",
    title: "Updates & News",
    headerExtra: refresh,
    bodyClass: "live-info-body",
    children: msg,
  }).element;
};

export const NodeStatus = () => {
  const UI = window.UI;
  const mkRow = (label, value) => {
    const row = document.createElement("div");
    row.className = "sc-status-row";
    const l = document.createElement("span");
    l.className = "sc-status-label";
    l.textContent = label;
    const v = document.createElement("span");
    v.className = "sc-status-value";
    v.textContent = value;
    row.append(l, v);
    return row;
  };
  return UI.SearchCard({
    className: "search-card-status",
    title: "Node Status",
    bodyClass: "search-card-status-body",
    children: [
      mkRow("TELA Apps", "1,284 discovered"),
      mkRow("Node", "Connected"),
      mkRow("TELA", "Running"),
      mkRow("Gnomon", "Running"),
      mkRow("Web Socket", "Allowed"),
    ],
  }).element;
};

export const EmptyState = () => {
  const UI = window.UI;
  return UI.SearchCard({
    title: "Coming Soon",
    empty: true,
  }).element;
};

export const Stack = () => {
  const grid = document.createElement("div");
  grid.className = "search-cards";
  grid.appendChild(LatestFinds());
  grid.appendChild(UpdatesAndNews());
  grid.appendChild(NodeStatus());
  return grid;
};