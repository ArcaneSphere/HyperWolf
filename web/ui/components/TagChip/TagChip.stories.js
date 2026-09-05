import "./TagChip.js";
import "./TagChip.css";

export default {
  title: "Components/TagChip",
  parameters: {
    llm: {
      description: "Compact removable tag chip with a ✕ button that fires onRemove (rendering the chip list is the parent's job).",
      useWhen: ["Hide-extension tag lists", "Small removable token inputs"],
      avoidWhen: ["Long labels (keep < 20 chars)", "Non-removable static badges"],
      related: ["ToggleSwitch", "Popover", "TagInput"],
    },
  },
};

export const Default = () => {
  const UI = window.UI;
  return UI.TagChip("exe", { onRemove: (t) => console.log("remove", t) });
};

export const Group = () => {
  const UI = window.UI;
  const list = document.createElement("div");
  list.className = "tag-list";
  list.style.display = "flex";
  list.style.flexWrap = "wrap";
  list.style.gap = "6px";
  list.style.alignItems = "center";
  ["exe", "jar", "msi", "sh", "py"].forEach((t) =>
    list.appendChild(window.UI.TagChip(t, { onRemove: (x) => console.log("remove", x) }))
  );
  return list;
};

export const NoRemove = () => {
  const UI = window.UI;
  return UI.TagChip("read-only");
};