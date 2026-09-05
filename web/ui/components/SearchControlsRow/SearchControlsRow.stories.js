import "./SearchControlsRow.js";
import "./SearchControlsRow.css";

export default {
  title: "Search/SearchControlsRow",
  parameters: {
    llm: {
      description:
        "The search page filtering bar: a min-rating range slider with live value, a custom sort-mode dropdown (.custom-select family of classes), an 'All SCIDs' toggle-switch (compact overlay on ToggleSwitch) and an optional status readout. Builder returns refs and helpers (sortSelect.getValue, setStatus).",
      useWhen: [
        "Filtering/search-result toolbars with slider + dropdown + toggle",
        "Any compact custom-select dropdown (trigger + menu) composition",
      ],
      avoidWhen: [
        "Single native-styled select — keep a plain <select>",
        "Settings page rows — use SettingItem",
      ],
      related: ["SettingItem", "ToggleSwitch", "ResultRow"],
    },
  },
};

export const Standard = () => {
  const { element } = window.UI.SearchControlsRow({
    onSort: () => {},
    onMinRating: () => {},
    onShowAll: () => {},
    status: "Loading SCIDs...",
  });
  return element;
};

export const SortOnly = () => {
  const { element } = window.UI.SearchControlsRow({
    sortOptions: [
      { value: "newest", label: "Newest first" },
      { value: "oldest", label: "Oldest first" },
    ],
    sortValue: "newest",
    showAllLabel: "Show all",
  });
  return element;
};

export const RatingAndToggle = () => {
  const div = document.createElement("div");
  const { element } = window.UI.SearchControlsRow({
    minRating: 50,
    showAll: true,
    showAllLabel: "Include unpublished",
    status: "✅ 12 TELA apps · showing 8",
  });
  element.querySelectorAll(".custom-select").forEach((sel) => sel.remove());
  div.appendChild(element);
  return div;
};