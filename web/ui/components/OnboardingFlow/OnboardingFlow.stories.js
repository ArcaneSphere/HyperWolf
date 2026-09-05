import "./OnboardingFlow.js";
import "./OnboardingFlow.css";

export default {
  title: "Server/OnboardingFlow",
  parameters: {
    llm: {
      description:
        "First-run overlay to pick a default node: selectable bookmark list (.onboarding-node-option with .onb-label/.onb-addr), custom address input, Skip/Connect actions. Selecting clears the input, typing clears the selection, Enter connects. Mounted on the document body (position overlay from Popover shell).",
      useWhen: [
        "First-run / onboarding flows with a selectable list of options",
        "Overlay pickers with a 'custom value' input + primary/secondary actions",
      ],
      avoidWhen: [
        "Single confirm dialogs — use Popover",
        "Normal forms or in-page settings — use SettingItem",
      ],
      related: ["Popover", "SettingItem", "StatusDot"],
    },
  },
  decorators: [
    (story) => {
      const host = document.createElement("div");
      host.style.position = "relative";
      const el = story();
      if (!el.classList.contains("hidden")) host.appendChild(el);
      else host.appendChild(el);
      return host;
    },
  ],
};

export const WithNodes = () => {
  const { element, show } = window.UI.OnboardingFlow({
    title: "👋 Welcome to HyperWolf",
    description:
      "Choose a node to connect to. Your choice becomes your default node and can be changed anytime in Settings.",
    nodes: [
      { label: "Community Node (EU)", addr: "community.dero.io:10102" },
      { label: "Local Daemon", addr: "127.0.0.1:10102" },
      { label: "Faucet Node (US)", addr: "faucet.dero.live:10102" },
    ],
    onConnect: () => {},
    onSkip: () => {},
  });
  show();
  return element;
};

export const EmptyList = () => {
  const { element, show } = window.UI.OnboardingFlow({
    title: "👋 Welcome to HyperWolf",
    description: "No nodes bookmarked yet — enter one below.",
    nodes: [],
    onConnect: () => {},
    onSkip: () => {},
  });
  show();
  return element;
};

export const Preselected = () => {
  const { element, show } = window.UI.OnboardingFlow({
    title: "👋 Welcome to HyperWolf",
    description: "Switch default node.",
    nodes: [
      { label: "Community Node (EU)", addr: "community.dero.io:10102", selected: true },
      { label: "Local Daemon", addr: "127.0.0.1:10102" },
    ],
    onConnect: () => {},
    onSkip: () => {},
  });
  show();
  return element;
};