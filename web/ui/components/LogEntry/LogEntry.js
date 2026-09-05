/* ============================================================
   LogEntry — one terminal log line (timestamp, level pill, message).
   Pure builder → returns HTML string (matches original createLogEntry).
   Populates window.UI.
   Styling: LogEntry.css (mirrors style.css until visual baseline).
   ============================================================ */
(function () {
  "use strict";
  const UI = (window.UI = window.UI || {});

  function escapeHtml(text) {
    const div = document.createElement("div");
    div.textContent = text;
    return div.innerHTML;
  }

  /**
   * @param {Object} entry
   * @param {number|string} entry.timestamp
   * @param {string=} entry.level — INFO | WARN | ERROR | SUCCESS
   * @param {string=} entry.message
   * @returns {string} HTML string for one .log-entry
   */
  UI.LogEntry = function LogEntry(entry) {
    const timestamp = new Date(entry.timestamp).toLocaleString();
    const level = entry.level || "INFO";
    const message = escapeHtml(entry.message || "");

    return `
      <div class="log-entry">
        <span class="log-timestamp">${timestamp}</span>
        <span class="log-level ${level}">${level}</span>
        <span class="log-message">${message}</span>
      </div>
    `;
  };
})();