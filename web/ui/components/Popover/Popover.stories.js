import "./Popover.js";
import "./Popover.css";

export default {
  title: "Overlays/Popover",
  parameters: {
    llm: {
      description: "Modal overlay dialog with title, optional message, optional single-line input and an action row. Backdrop click closes. Parent owns visibility via .show()/.hide().",
      useWhen: ["Requiring a decision (bookmark save/remove, confirm) with focus on one task"],
      avoidWhen: ["Multi-step flows", "Onboarding with custom node lists (reuse shell, app adds .onboarding-* body)"],
      related: ["ToggleSwitch", "TagChip", "Toast", "OnboardingFlow"],
    },
  },
};

const show = (handle) => {
  handle.show();
  return handle.element;
};

export const BookmarkSave = () => {
  const UI = window.UI;
  return show(UI.Popover({
    title: "Bookmark",
    input: { placeholder: "Label (default: abc12345)" },
    actions: [
      { label: "Save", onClick: (h) => console.log("save", h.inputEl.value) },
      { label: "Cancel", kind: "secondary", onClick: (h) => h.hide() },
    ],
    onBackdrop: (h) => h.hide(),
  }));
};

export const BookmarkRemove = () => {
  const UI = window.UI;
  return show(UI.Popover({
    title: "Remove bookmark?",
    message: "This only removes the bookmark from your list; the app itself is untouched.",
    actions: [
      { label: "Yes", kind: "danger", onClick: (h) => console.log("removed") },
      { label: "Cancel", kind: "secondary", onClick: (h) => h.hide() },
    ],
    onBackdrop: (h) => h.hide(),
  }));
};

export const Confirm = () => {
  const UI = window.UI;
  return show(UI.Popover({
    title: "Confirm",
    message: "Are you sure you want to do this? This action cannot be undone.",
    actions: [
      { label: "Confirm", kind: "danger", onClick: (h) => console.log("confirmed") },
      { label: "Cancel", kind: "secondary", onClick: (h) => h.hide() },
    ],
    onBackdrop: (h) => h.hide(),
  }));
};

export const EditBookmarkLabel = () => {
  const UI = window.UI;
  return show(UI.Popover({
    title: "Bookmark",
    input: { value: "My wallet", placeholder: "Label" },
    actions: [
      { label: "Save", onClick: () => {} },
      { label: "Cancel", kind: "secondary", onClick: (h) => h.hide() },
    ],
    onBackdrop: (h) => h.hide(),
  }));
};