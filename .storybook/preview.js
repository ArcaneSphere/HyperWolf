import "../web/tokens.css";
import "../web/style.css";

export const decorators = [
  (story) => {
    const wrap = document.createElement("div");
    wrap.style.cssText =
      "font-family: var(--font-ui); color: var(--color-text); background: var(--color-bg); min-height: 100vh; padding: 24px;";
    const node = story();
    wrap.appendChild(node);
    return wrap;
  },
];