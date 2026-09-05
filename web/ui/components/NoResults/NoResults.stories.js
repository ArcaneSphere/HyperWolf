import "./NoResults.js";
import "./NoResults.css";

export default {
  title: "Components/NoResults",
  parameters: {
    llm: {
      description: "Empty-state placeholder shown when a list has no items.",
      useWhen: ["Search returned nothing", "Empty bookmark lists", "Empty logs"],
      avoidWhen: ["Loading states (use skeletons/spinners instead)"],
      related: ["ResultRow", "BookmarkItem"],
    },
  },
};

export const Default = () => {
  const UI = window.UI;
  return UI.NoResults("No results found");
};

export const CustomMessage = () => {
  const UI = window.UI;
  return UI.NoResults("No bookmarked nodes yet");
};