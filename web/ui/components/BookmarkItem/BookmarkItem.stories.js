import "./BookmarkItem.js";
import "./BookmarkItem.css";

export default {
  title: "Bookmarks/BookmarkItem",
  parameters: {
    llm: {
      description:
        "A bookmark row for the Bookmarks page: label + monospace value on the left, Load / ✏ inline-edit / Remove actions on the right. Callers get { element, labelEl, valueEl }; label edits surface via onCommit.",
      useWhen: [
        "Rendering saved nodes or SCIDs in bookmark lists",
        "Any list row needing load + destructive + rename actions",
      ],
      avoidWhen: [
        "Search results with ratings and icons — use ResultRow",
        "Flow of adding/removing via popover — use BookmarkFlow/Popover",
      ],
      related: ["ResultRow", "Popover", "StatusCard", "NoResults"],
    },
  },
};

export const Node = () => {
  const { element } = window.UI.BookmarkItem({
    label: "Main daemon",
    value: "10.0.0.7:10102",
    onLoad: () => console.log("load node"),
    onRemove: () => console.log("remove node"),
  });
  return element;
};

export const Scid = () => {
  const { element } = window.UI.BookmarkItem({
    label: "Dero World",
    value: "a6832a5a09b82dc4b1034fd726b118da1df8ca9ad33e76bee4563e3f69d1d99a",
    onLoad: () => console.log("load scid"),
    onRemove: () => console.log("remove scid"),
  });
  return element;
};

export const Stack = () => {
  const wrap = document.createElement("div");
  [
    { label: "Main daemon", value: "10.0.0.7:10102" },
    { label: "Dero World", value: "a6832a5a09b82dc4b1034fd726b118da1df8ca9ad33e76bee4563e3f69d1d99a" },
    { label: "Vault", value: "b6832a5a09b82dc4b1034fd726b118da1df8ca9ad33e76bee4563e3f69d1d99a" },
  ].forEach((bm) =>
    wrap.appendChild(window.UI.BookmarkItem(bm).element)
  );
  return wrap;
};