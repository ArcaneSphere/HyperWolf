/* ============================================================
   OnboardingFlow — first-run node picker: overlay with a title,
   description, a selectable node list, a custom-node input and
   Skip/Connect actions. Pure builder, no DOM side effects (the
   caller mounts the returned element). Populates window.UI.
   The server page ships a static equivalent in index.html and
   dashboard.js drives it by id; the builder reproduces the same
   ids and behaviors (select clears the input, typing clears the
   selection, Enter connects) so it can take over the flow.
   Styling: OnboardingFlow.css + Popover.css (shell).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {Object=} opts
   * @param {string=} opts.id - id for the overlay root (default "onboarding-popover")
   * @param {string|Node=} opts.title - overlay title
   * @param {string|Node=} opts.description - helper text (class .onboarding-desc, styled in Popover.css)
   * @param {string=} opts.placeholder - custom-node input placeholder
   * @param {string=} opts.skipLabel - (default "Skip")
   * @param {string=} opts.connectLabel - (default "Connect")
   * @param {Array.<{label: string, addr: string, selected?: boolean}>=} opts.nodes - initial node options
   * @param {function(string, Object)=} opts.onNodeSelect - called with the chosen addr
   * @param {function(string)=} opts.onSkip - called when skipping (flow hidden)
   * @param {function(string)=} opts.onConnect - called with the resolved addr (selected or custom)
   * @param {function(string)=} opts.onWarn - shown when Connect is pressed with no selection
   * @returns {{element: HTMLElement, listEl: HTMLElement, inputEl: HTMLElement, connectBtn: HTMLElement, skipBtn: HTMLElement, getSelection: function(): string, renderNodes: function(Array.<Object>): void, show: function(): void, hide: function(): void}}
   */
  UI.OnboardingFlow = function OnboardingFlow(opts = {}) {
    const shell = document.createElement("div");
    shell.id = opts.id || "onboarding-popover";
    shell.className = "popover hidden";

    const content = document.createElement("div");
    content.className = "popover-content onboarding-content";

    const titleEl = document.createElement("div");
    titleEl.className = "popover-title";
    titleEl.id = "onboarding-popover-title";
    if (opts.title != null) {
      if (opts.title.nodeType) titleEl.appendChild(opts.title);
      else titleEl.textContent = String(opts.title);
    }

    const descEl = document.createElement("p");
    descEl.className = "onboarding-desc";
    if (opts.description != null) {
      if (opts.description.nodeType) descEl.appendChild(opts.description);
      else descEl.textContent = String(opts.description);
    }

    const listEl = document.createElement("div");
    listEl.className = "onboarding-nodes";
    listEl.id = "onboarding-node-list";

    const inputEl = document.createElement("input");
    inputEl.type = "text";
    inputEl.id = "onboarding-node-input";
    inputEl.placeholder = opts.placeholder || "Or enter your own node address (e.g. 127.0.0.1:10102)";
    inputEl.spellcheck = false;
    inputEl.autocomplete = "off";

    const actions = document.createElement("div");
    actions.className = "popover-actions";
    const skipBtn = document.createElement("button");
    skipBtn.id = "onboarding-skip";
    skipBtn.className = "secondary";
    skipBtn.textContent = opts.skipLabel || "Skip";
    const connectBtn = document.createElement("button");
    connectBtn.id = "onboarding-connect";
    connectBtn.textContent = opts.connectLabel || "Connect";
    actions.append(skipBtn, connectBtn);

    content.append(titleEl, descEl, listEl, inputEl, actions);
    shell.appendChild(content);

    let selection = "";

    function renderNodes(nodes) {
      listEl.replaceChildren();
      if (!nodes || !nodes.length) {
        const empty = document.createElement("div");
        empty.className = "onboarding-empty";
        empty.textContent = "No bookmarked nodes available — enter your own below.";
        listEl.appendChild(empty);
        return;
      }
      nodes.forEach((n) => {
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "onboarding-node-option" + (n.selected ? " selected" : "");
        btn.dataset.node = n.addr != null ? String(n.addr) : "";
        const label = document.createElement("span");
        label.className = "onb-label";
        label.textContent = n.label || "Node";
        const addr = document.createElement("span");
        addr.className = "onb-addr";
        addr.textContent = n.addr != null ? String(n.addr) : "";
        btn.append(label, addr);
        btn.addEventListener("click", () => {
          listEl.querySelectorAll(".onboarding-node-option").forEach((b) => b.classList.remove("selected"));
          btn.classList.add("selected");
          selection = btn.dataset.node;
          inputEl.value = "";
          if (opts.onNodeSelect) opts.onNodeSelect(selection, n);
        });
        listEl.appendChild(btn);
      });
    }
    if (opts.nodes) renderNodes(opts.nodes);

    skipBtn.addEventListener("click", () => {
      shell.classList.add("hidden");
      if (opts.onSkip) opts.onSkip();
    });
    connectBtn.addEventListener("click", () => {
      const custom = inputEl.value.trim();
      const node = custom || selection;
      if (!node) {
        if (opts.onWarn) opts.onWarn("Pick a node or enter one first");
        return;
      }
      shell.classList.add("hidden");
      if (opts.onConnect) opts.onConnect(node);
    });
    inputEl.addEventListener("input", () => {
      if (!inputEl.value.trim()) return;
      listEl.querySelectorAll(".onboarding-node-option").forEach((b) => b.classList.remove("selected"));
      selection = "";
    });
    inputEl.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        connectBtn.click();
      }
    });

    return {
      element: shell,
      listEl,
      inputEl,
      connectBtn,
      skipBtn,
      getSelection: () => selection,
      renderNodes,
      show() { shell.classList.remove("hidden"); },
      hide() { shell.classList.add("hidden"); },
    };
  };
})();