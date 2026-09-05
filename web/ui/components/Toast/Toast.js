/* ============================================================
   Toast — transient notification with dot, message and close.
   Built as a DOM element; lifecycle (append, timers, dismiss)
   is owned by the caller.
   Pure builder, populates window.UI.
   Depends on: UI.StatusDot.
   Styling: Toast.css (mirrors style.css until visual baseline).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  /**
   * @param {"connected"|"error"|"warning"|"pending"} state
   * @param {string} message
   * @param {(toast: HTMLElement) => void} onClose — wired to the ✕ button
   * @returns {HTMLElement} div.toast.toast-<state>
   */
  UI.Toast = function Toast(state, message, onClose) {
    const toast = document.createElement("div");
    toast.className = "toast toast-" + state;

    const dot = UI.StatusDot(state);
    const msg = document.createElement("span");
    msg.className = "toast-msg";
    msg.textContent = message;

    const close = document.createElement("button");
    close.className = "toast-close";
    close.textContent = "✕";
    close.onclick = () => onClose && onClose(toast);

    toast.append(dot, msg, close);
    return toast;
  };
})();