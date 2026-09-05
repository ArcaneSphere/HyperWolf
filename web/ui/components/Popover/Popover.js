/* ============================================================
   Popover — modal overlay dialog (bookmark save/remove, confirm…).
   Pure builder, no DOM side effects. Populates window.UI.
   Returns a handle: { element, titleEl, messageEl, inputEl,
   actions, show(), hide() }. Visibility is owned by the caller.
   Styling: Popover.css (mirrors style.css until visual baseline).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {Object} opts
   * @param {string} opts.title
   * @param {string=} opts.message — secondary text under the title
   * @param {Object=} opts.input — { value, placeholder, hidden, maxLength }
   * @param {Array<{label: string, kind?: "primary"|"secondary"|"danger", onClick: (handle) => void}>} opts.actions
   * @param {(handle: Object) => void=} opts.onBackdrop — fired when overlay backdrop is pressed
   * @returns {{element: HTMLElement, titleEl: HTMLElement, messageEl: HTMLElement,
   *   inputEl: HTMLElement, actions: Object, show(): void, hide(): void}}
   */
  UI.Popover = function Popover(opts = {}) {
    const el = document.createElement("div");
    el.className = "popover hidden";

    const content = document.createElement("div");
    content.className = "popover-content";

    const titleEl = document.createElement("div");
    titleEl.className = "popover-title";
    titleEl.textContent = opts.title || "";
    content.appendChild(titleEl);

    let messageEl = null;
    if (opts.message) {
      messageEl = document.createElement("p");
      messageEl.textContent = opts.message;
      content.appendChild(messageEl);
    }

    let inputEl = null;
    if (opts.input) {
      inputEl = document.createElement("input");
      inputEl.type = "text";
      inputEl.maxLength = opts.input.maxLength || 48;
      if (opts.input.value) inputEl.value = opts.input.value;
      if (opts.input.placeholder) inputEl.placeholder = opts.input.placeholder;
      if (opts.input.hidden) inputEl.style.display = "none";
      content.appendChild(inputEl);
    }

    const actionsEl = document.createElement("div");
    actionsEl.className = "popover-actions";

    let handle;
    const actionRefs = {};
    (opts.actions || []).forEach((a) => {
      const btn = document.createElement("button");
      btn.textContent = a.label;
      if (a.kind === "secondary") btn.classList.add("secondary");
      else if (a.kind === "danger") btn.classList.add("danger");
      btn.onclick = () => a.onClick && a.onClick(handle);
      actionRefs[a.label] = btn;
      actionsEl.appendChild(btn);
    });
    content.appendChild(actionsEl);

    el.appendChild(content);
    el.addEventListener("mousedown", (e) => {
      if (e.target === el && opts.onBackdrop) opts.onBackdrop(handle);
    });

    handle = {
      element: el,
      titleEl,
      messageEl,
      inputEl,
      actions: actionRefs,
      show() { el.classList.remove("hidden"); },
      hide() { el.classList.add("hidden"); },
    };
    return handle;
  };
})();