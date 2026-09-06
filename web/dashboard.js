/**
 * @module dashboard
 * @description Core dashboard logic for HyperWolf. Manages SPA navigation,
 * WebSocket connection, node connection/disconnection, SCID loading,
 * bookmarks, settings persistence, sync progress visualization, and
 * toast notifications. All state is module-scoped within this IIFE.
 */

// ================= ELEMENTS =================
const header = document.getElementById("header");
const sidebar = document.getElementById("sidebar");
const sidebarBackdrop = document.getElementById("sidebar-backdrop");
const menuToggleBtn = document.getElementById("menu-toggle");
const statusEl = document.getElementById("status");
const navItems = document.querySelectorAll(".nav-item");
const pages = document.querySelectorAll(".page");

const sidebarNativeStatus = document.getElementById("sidebar-server-status");
const sidebarTelaStatus = document.getElementById("sidebar-tela-status");
const sidebarGnomonStatus = document.getElementById("sidebar-gnomon-status");
const sidebarXswdStatus = document.getElementById("sidebar-xswd-status");

const nodeInput = document.getElementById("node");
const connectNodeBtn = document.getElementById("connectNodeBtn");
const scidInput = document.getElementById("scid");
const loadBtn = document.getElementById("load");

const bookmarkScidBtn = document.getElementById("bookmark-scid");
const bookmarkNodeBtn = document.getElementById("bookmark-node");
const bookmarkSettingNodeBtn = document.getElementById("bookmark-setting-node");
const bookmarkedScidsEl = document.getElementById("bookmarked-scids");
const bookmarkedNodesEl = document.getElementById("bookmarked-nodes");
const bookmarkPopover = document.getElementById("bookmark-popover");
const bookmarkPopoverTitle = document.getElementById("bookmark-popover-title");
const bookmarkLabelInput = document.getElementById("bookmark-label-input");
const bookmarkPopoverSave = document.getElementById("bookmark-popover-save");
const bookmarkPopoverCancel = document.getElementById("bookmark-popover-cancel");

const themeToggle = document.getElementById("theme-toggle");


// ================= STATE =================
let bookmarks = { scids: {}, nodes: {} };
let settings = { 
  defaultNode: "", 
  autoConnect: true, 
  directLoad: true, 
  openDashboardOnStart: true, 
  hiddenExtensions: "", 
  showSearchCards: true, 
  showTopBar: false, 
  searchGradient: "default",
  checkUpdates: true,
  rssFeedUrl: "https://dero.world/anotherworld/feed/",
  fontScale: 1
};
let appConfig = { gnomon_api_port: 18082, tela_port: 18081 };
let wasConnected = false;
let connectTime = null;
let bookmarkTarget = null;
let syncStartTime = null;
let lastChainHeight = 0;
let lastBlockTime = null;

// ================= API HELPERS =================

/**
 * Sends a POST request to a dashboard API endpoint.
 * @param {string} method - API endpoint name (e.g. "set_node", "server_status", "load_scid")
 * @param {Object} [params={}] - JSON body parameters to send
 * @returns {Promise<{ok: boolean, result?: any, error?: string}>} Parsed JSON response
 * @throws {TypeError} If the network request itself fails
 */
async function send(method, params = {}) {
  const res = await fetch("/api/" + method, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(params),
  });
  return res.json();
}

window.getDirectLoadSetting = () => settings.directLoad !== false;
window.getShowSearchCardsSetting = () => settings.showSearchCards !== false;
window.getCheckUpdatesSetting = () => settings.checkUpdates !== false;
window.getRSSFeedUrlSetting = () => settings.rssFeedUrl || "https://dero.world/anotherworld/feed/";

// Expose bookmark helpers for search.js result rows
window.isBookmarked = (scid) => !!bookmarks.scids[scid];
window.toggleSearchBookmark = (scid) => {
  if (bookmarks.scids[scid]) {
    showBookmarkPopover("scid", scid, "remove");
  } else {
    showBookmarkPopover("scid", scid, "save");
  }
};

window.getHiddenExtensions = () => {
  return (settings.hiddenExtensions || "")
    .split(",")
    .map(e => e.trim().toLowerCase())
    .filter(Boolean);
};

// ================= THEME (System / Dark / Light) =================
const THEME_MODE_KEY = "hyperwolf.themeMode";
const prefersDark = window.matchMedia("(prefers-color-scheme: dark)");
const themeModeOptions = document.querySelectorAll(".theme-mode-option");

function resolveTheme(mode) {
  if (mode === "dark" || mode === "light") return mode;
  return prefersDark.matches ? "dark" : "light";
}

function themeModeLabel(mode) {
  if (mode === "system") return "Theme: System (follows OS)";
  return `Theme: ${mode[0].toUpperCase() + mode.slice(1)}`;
}

function applyThemeMode(mode) {
  const resolved = resolveTheme(mode);
  document.documentElement.setAttribute("data-theme", resolved);
  document.documentElement.setAttribute("data-theme-mode", mode);
  localStorage.setItem(THEME_MODE_KEY, mode);
  themeToggle.title = `${themeModeLabel(mode)} — click to change`;
  themeToggle.setAttribute("aria-label", themeModeLabel(mode));
  themeToggle.dataset.mode = mode;
  themeModeOptions.forEach((btn) => {
    const active = btn.dataset.themeMode === mode;
    btn.classList.toggle("active", active);
    btn.setAttribute("aria-pressed", String(active));
  });
}

const legacyTheme = localStorage.getItem("theme");
const initialMode =
  localStorage.getItem(THEME_MODE_KEY) ||
  (legacyTheme === "dark" || legacyTheme === "light" ? legacyTheme : "system");
applyThemeMode(initialMode);

prefersDark.addEventListener("change", () => {
  if ((localStorage.getItem(THEME_MODE_KEY) || "system") === "system") {
    applyThemeMode("system");
  }
});

themeToggle.onclick = () => {
  const order = ["system", "dark", "light"];
  const cur = localStorage.getItem(THEME_MODE_KEY) || "system";
  const next = order[(order.indexOf(cur) + 1) % order.length];
  applyThemeMode(next);
};

themeModeOptions.forEach((btn) => {
  btn.addEventListener("click", () => applyThemeMode(btn.dataset.themeMode));
});

// ================= NAV =================
// ================= SIDEBAR OVERLAY =================
function openSidebar() {
  sidebar.classList.add("sidebar-open");
  sidebarBackdrop.classList.add("active");
}

function closeSidebar() {
  sidebar.classList.remove("sidebar-open");
  sidebarBackdrop.classList.remove("active");
}

function toggleSidebar() {
  if (sidebar.classList.contains("sidebar-open")) closeSidebar();
  else openSidebar();
}

if (menuToggleBtn) menuToggleBtn.onclick = toggleSidebar;
if (sidebarBackdrop) sidebarBackdrop.onclick = closeSidebar;

document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && sidebar.classList.contains("sidebar-open")) {
    closeSidebar();
  }
});

/**
 * Navigates to a named page in the single-page application.
 * Updates the active nav item and page visibility, then dispatches
 * a custom "pageChanged" event so other modules (e.g. search.js) can react.
 * @param {string} page - Page identifier (e.g. "server", "search", "bookmarks", "settings", "help", "about")
 * @returns {void}
 */
function navigateTo(page) {
  navItems.forEach(n => n.classList.remove("active"));
  pages.forEach(p => p.classList.remove("active"));
  const navItem = document.querySelector(`[data-page=${page}]`);
  const pageEl  = document.getElementById(`page-${page}`);
  if (navItem) navItem.classList.add("active");
  if (pageEl)  pageEl.classList.add("active");
  document.dispatchEvent(new CustomEvent("pageChanged", { detail: { page } }));
}

navItems.forEach(item => {
  item.onclick = () => {
    navigateTo(item.dataset.page);
    closeSidebar();
  };
});

document.addEventListener("DOMContentLoaded", () => {
  document.querySelectorAll("a.nav-link[data-target]").forEach(a => {
    a.addEventListener("click", e => {
      e.preventDefault();
      navigateTo(a.dataset.target);
    });
  });
});

// ================= HELPERS =================

/**
 * Creates a status dot <span> element with the given state color.
 * @param {"connected"|"error"|"pending"|"warning"} state - Determines the dot color class
 * @returns {HTMLElement} A <span> element with class "status-dot {state}"
 */
function createDot(state) {
  return UI.StatusDot(state);
}

/**
 * Replaces an element's children with a status dot and text node.
 * @param {HTMLElement} el - Target element to update
 * @param {"connected"|"error"|"pending"|"warning"} state - Dot color state
 * @param {string} text - Text to display after the dot
 * @returns {void}
 */
function setDotText(el, state, text) {
  el.replaceChildren(createDot(state), document.createTextNode(" " + text));
}

/**
 * Updates a sidebar status row to show running/stopped state.
 * @param {HTMLElement} el - The status row element
 * @param {boolean} running - Whether the service is running
 * @param {string} [okText="Running"] - Text to show when running
 * @param {string} [failText="Stopped"] - Text to show when stopped
 * @returns {void}
 */
function setStatus(el, running, okText = "Running", failText = "Stopped") {
  const icon = el.querySelector(".sb-icon");
  const text = el.querySelector(".sb-text");
  if (icon && text) {
    icon.replaceChildren(createDot(running ? "connected" : "error"));
    text.textContent = running ? okText : failText;
  } else {
    setDotText(el, running ? "connected" : "error", running ? okText : failText);
  }
}

/**
 * Creates a "no results" placeholder div element.
 * @param {string} text - Message to display
 * @returns {HTMLElement} A div with class "no-results"
 */
function createNoResults(text) {
  return UI.NoResults(text);
}

// ================= TOAST NOTIFICATIONS =================

/**
 * Displays a toast notification in the status area. Automatically dismisses
 * after a duration based on the state type. Maximum 5 visible toasts.
 * @param {"connected"|"error"|"warning"|"pending"} state - Toast type (affects color and duration)
 * @param {string} message - Display text
 * @returns {HTMLElement|undefined} The created toast element, or undefined if statusEl is missing
 */
function pushToast(state, message) {
  if (!statusEl) return;
  const toast = UI.Toast(state, message, (t) => dismissToast(t));
  statusEl.appendChild(toast);
  const maxVisible = 5;
  while (statusEl.children.length > maxVisible) {
    statusEl.firstChild.remove();
  }
  const durations = { connected: 4000, error: 6000, warning: 5000, pending: 4000 };
  const delay = durations[state] || 5000;
  toast._timeout = setTimeout(() => dismissToast(toast), delay);
  return toast;
}

window.pushToast = pushToast;

/**
 * Dismisses a toast notification with a fade-out animation.
 * @param {HTMLElement} toast - The toast DOM element to remove
 * @returns {void}
 */
function dismissToast(toast) {
  if (!toast || toast.classList.contains("removing")) return;
  clearTimeout(toast._timeout);
  toast.classList.add("removing");
  setTimeout(() => toast.remove(), 250);
}

// ================= NODE CONNECT / DISCONNECT =================

/**
 * Updates the UI to reflect the current node connection state.
 * Toggles the connect/disconnect button, enables/disables the node input,
 * and shows appropriate toast notifications.
 * @param {boolean} connected - Whether currently connected to a node
 * @param {string} [node] - The connected node address (displayed in the input)
 * @returns {void}
 */
function setNodeConnected(connected, node) {
  if (connected) {
    if (node) nodeInput.value = node;
    connectNodeBtn.textContent = "Disconnect";
    connectNodeBtn.classList.add("danger");
    nodeInput.disabled = true;
    if (!wasConnected) pushToast("connected", "Connected to " + (node || nodeInput.value.trim()));
  } else {
    connectNodeBtn.textContent = "Connect";
    connectNodeBtn.classList.remove("danger");
    nodeInput.disabled = false;
    resetSyncProgress();
    if (wasConnected) pushToast("warning", "Waiting for node...");
  }
  wasConnected = connected;
  if (connected && !connectTime) connectTime = Date.now();
  if (!connected) connectTime = null;
}

connectNodeBtn.onclick = async () => {
  const isConnected = connectNodeBtn.textContent === "Disconnect";
  if (isConnected) {
    try { await send("disconnect_node"); } catch (e) {}
    setNodeConnected(false);
    updateStatusIndicators();
    document.dispatchEvent(new CustomEvent("nodeDisconnected"));
    return;
  }
  const node = nodeInput.value.trim();
  if (!node) return alert("Enter node first");
  pushToast("pending", "Connecting...");
  connectNodeBtn.disabled = true;
  try {
    const r = await send("set_node", { node });
    if (!r.ok) {
      pushToast("error", "Failed to connect");
      alert("Failed to connect node: " + r.error);
      return;
    }
    setNodeConnected(true, node);
    updateStatusIndicators();
    document.dispatchEvent(new CustomEvent("nodeConnected", { detail: { node } }));
    if (typeof saveSettings === "function") saveSettings();
  } catch (e) {
    pushToast("error", "Error connecting node");
    alert("Error: " + e.message);
  } finally {
    connectNodeBtn.disabled = false;
  }
};

/**
 * Loads a bookmarked node using the flow: disconnect the old node (if any),
 * then connect to the target node. Previously the "Load" action only worked
 * while disconnected — when a node was already active the connect click was
 * skipped and the status poll fell back to the existing node.
 * @param {string} node - The bookmarked node address to connect to
 * @returns {Promise<void>}
 */
async function loadBookmarkedNode(node) {
  const isConnected = connectNodeBtn.textContent === "Disconnect";
  if (isConnected) {
    // 1. Disconnect the old node first (backend stops sync + resets TELA proxy).
    try { await send("disconnect_node"); } catch (e) {}
    setNodeConnected(false);
    updateStatusIndicators();
    document.dispatchEvent(new CustomEvent("nodeDisconnected"));
  }
  // 2. Point the input at the bookmarked node and load it via the normal
  // connect flow (button now reads "Connect", so click() connects).
  nodeInput.value = node;
  updateBookmarkButtons();
  connectNodeBtn.click();
}

// ================= SCID LOADER =================
function scidLoaderShow() {
  const el = document.getElementById("scid-loader");
  if (!el) return;
  el.classList.remove("failed");
  el.classList.add("active");
}
function scidLoaderSuccess() {
  const el = document.getElementById("scid-loader");
  if (!el) return;
  el.classList.remove("active");
}
function scidLoaderFail() {
  const el = document.getElementById("scid-loader");
  if (!el) return;
  el.classList.add("failed");
  el.addEventListener("animationend", () => {
    el.classList.remove("active", "failed");
  }, { once: true });
}

// ================= LOAD SCID =================
loadBtn.onclick = async () => {
  const scid = scidInput.value.trim();
  if (!scid) return alert("Enter SCID first");
  if (!nodeInput.value.trim()) return alert("Set node first");
  pushToast("pending", "Loading SCID...");
  scidLoaderShow();
  try {
    const r = await send("load_scid", { scid });
    if (!r.ok) {
      scidLoaderFail();
      pushToast("error", r.error || "Unknown error");
      alert("Failed to load SCID: " + (r.error || "Unknown error"));
      return;
    }
    const url = r.result?.url;
    if (!url) {
      scidLoaderFail();
      pushToast("warning", "Loaded, but no URL returned");
      return;
    }
    scidLoaderSuccess();
    pushToast("connected", "SCID loaded");
    window.open(url, "_blank");
  } catch (e) {
    scidLoaderFail();
    pushToast("error", "Error loading SCID");
    alert("Error: " + e.message);
  }
};

// ================= XSWD =================

/**
 * Probes the XSWD (Dero WebSocket Daemon) endpoint at 127.0.0.1:44326.
 * @returns {Promise<boolean>} True if XSWD is reachable and responding
 */
async function probeXswd() {
  try {
    const r = await fetch("/api/probe_xswd");
    const d = await r.json();
    return d.xswd === true;
  } catch { return false; }
}

// ================= STATUS =================
async function updateStatusIndicators() {
  try {
    const r = await send("server_status");
    if (!r?.ok || !r?.result) return;
    const { tela, gnomon, connected, node, heights, tela_apps_count, connected_at, daemon } = r.result;
    if (sidebarNativeStatus) setStatus(sidebarNativeStatus, !!node, "Connected", "Not connected");
    if (sidebarTelaStatus) setStatus(sidebarTelaStatus, tela);
    if (sidebarGnomonStatus) setStatus(sidebarGnomonStatus, gnomon);
    if (sidebarXswdStatus) {
      const xswd = await probeXswd();
      const icon = sidebarXswdStatus.querySelector(".sb-icon");
      const text = sidebarXswdStatus.querySelector(".sb-text");
      if (icon && text) {
        icon.replaceChildren(createDot(xswd ? "connected" : "error"));
        text.textContent = xswd ? "Allowed" : "Blocked";
      }
      // Update search page status card
      const scXswd = document.getElementById("sc-xswd-status");
      if (scXswd) { scXswd.textContent = xswd ? "Allowed" : "Blocked"; scXswd.style.color = xswd ? "var(--color-success)" : "var(--color-danger)"; }
    }
    const hasNode = !!node && connected;
    setNodeConnected(hasNode, node);
    if (heights) {
      updateSyncProgress(heights.indexed, heights.chain);
      if (heights.chain !== lastChainHeight && heights.chain > 0) {
        lastChainHeight = heights.chain;
        lastBlockTime = Date.now();
      }
    }
    const nodeEl = document.getElementById("sv-node");
    if (nodeEl) nodeEl.textContent = node || "—";
    const versionEl = document.getElementById("sv-version");
    if (versionEl) versionEl.textContent = daemon?.version || "—";
    const networkEl = document.getElementById("sv-network");
    if (networkEl) networkEl.textContent = daemon?.network || "—";
    const tipAgeEl = document.getElementById("sv-tip-age");
    if (tipAgeEl && lastBlockTime) {
      tipAgeEl.textContent = formatAge(Date.now() - lastBlockTime);
    } else if (tipAgeEl) {
      tipAgeEl.textContent = "—";
    }
    const uptimeEl = document.getElementById("sv-uptime");
    if (uptimeEl && connected_at > 0) {
      const secs = Math.floor((Date.now() - connected_at) / 1000);
      uptimeEl.textContent = formatDuration(secs);
    }
    const diffEl = document.getElementById("sv-difficulty");
    if (diffEl && daemon) {
      const diff = Number(daemon.difficulty);
      diffEl.textContent = diff > 0 ? formatHashrate(diff) : "—";
    }
    const mempoolEl = document.getElementById("sv-mempool");
    if (mempoolEl) mempoolEl.textContent = daemon?.mempool_size != null ? daemon.mempool_size + " txs" : "—";
    const telaCountEl = document.getElementById("sv-tela-count");
    if (telaCountEl && tela_apps_count > 0) {
      telaCountEl.textContent = tela_apps_count.toLocaleString();
    }
    // Update search page status card
    const scTelaCount = document.getElementById("sc-tela-count");
    if (scTelaCount) scTelaCount.textContent = tela_apps_count > 0 ? tela_apps_count.toLocaleString() + " discovered" : "—";
    const scNodeStatus = document.getElementById("sc-node-status");
    if (scNodeStatus) { scNodeStatus.textContent = node ? "Connected" : "Not connected"; scNodeStatus.style.color = node ? "var(--color-success)" : "var(--color-danger)"; }
    const scTelaStatus = document.getElementById("sc-tela-status");
    if (scTelaStatus) { scTelaStatus.textContent = tela ? "Running" : "Stopped"; scTelaStatus.style.color = tela ? "var(--color-success)" : "var(--color-danger)"; }
    const scGnomonStatus = document.getElementById("sc-gnomon-status");
    if (scGnomonStatus) { scGnomonStatus.textContent = gnomon ? "Running" : "Stopped"; scGnomonStatus.style.color = gnomon ? "var(--color-success)" : "var(--color-danger)"; }
  } catch (e) {
    console.warn("Status update failed:", e);
  }
}

function formatAge(ms) {
  const secs = Math.floor(ms / 1000);
  if (secs < 5) return "just now";
  if (secs < 60) return secs + "s ago";
  const m = Math.floor(secs / 60);
  if (m < 60) return m + "m ago";
  return Math.floor(m / 60) + "h ago";
}

function formatDuration(secs) {
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  const s = secs % 60;
  if (h > 0) return h + "h " + m + "m";
  if (m > 0) return m + "m " + s + "s";
  return s + "s";
}

function formatHashrate(diff) {
  if (diff >= 1e12) return (diff / 1e12).toFixed(2) + " TH/s";
  if (diff >= 1e9) return (diff / 1e9).toFixed(2) + " GH/s";
  if (diff >= 1e6) return (diff / 1e6).toFixed(2) + " MH/s";
  if (diff >= 1e3) return (diff / 1e3).toFixed(2) + " KH/s";
  return diff.toFixed(0) + " H/s";
}

setInterval(updateStatusIndicators, 5000);
updateStatusIndicators();

async function autoConnect() {
  await loadSettings();
  if (!settings.defaultNode || !settings.autoConnect) return;
  nodeInput.value = settings.defaultNode;
  updateBookmarkButtons();
  pushToast("pending", "Auto-connecting...");
  try {
    const r = await send("set_node", { node: settings.defaultNode });
    if (r.ok) {
      setNodeConnected(true, settings.defaultNode);
      updateStatusIndicators();
      const status = await send("server_status");
      if (status.ok && status.result.heights) {
        updateSyncProgress(status.result.heights.indexed, status.result.heights.chain);
      }
      document.dispatchEvent(new CustomEvent("nodeConnected", { detail: { node: settings.defaultNode } }));
    }
  } catch (e) {
    pushToast("error", "Auto-connect failed");
  }
}

document.addEventListener("DOMContentLoaded", async () => {
  initBookmarks();
  await loadSettings();
  initSettingsUI();
  autoConnect();
  connectWebSocket();
  showOnboardingPopover();
  populateAboutVersion();
});

// Fill the About page version from /api/config so it can never
// drift from the binary build.
async function populateAboutVersion() {
  const el = document.getElementById("hw-version");
  if (!el) return;
  try {
    const resp = await fetch("/api/config");
    const data = await resp.json();
    if (data.ok && data.result?.version) el.textContent = data.result.version;
  } catch (e) {
    /* keep the static fallback */
  }
}

// ================= DEFAULT BOOKMARKS =================
const defaultBookmarks = {
  nodes: {
    "127.0.0.1:10102": { node: "127.0.0.1:10102", label: "Local Node (default)" },
    "dero.rabidmining.com:10102": { node: "dero.rabidmining.com:10102", label: "Public Node" },
    "node.derofoundation.org:11012": { node: "node.derofoundation.org:11012", label: "Public Node" }
  },
  scids: {
    "a6832a5a09b82dc4b1034fd726b118da1df8ca9ad33e76bee4563e3f69d1d99a": {
      scid: "a6832a5a09b82dc4b1034fd726b118da1df8ca9ad33e76bee4563e3f69d1d99a",
      label: "Tela Demo"
    }
  }
};

// ================= BOOKMARKS =================
function saveBookmarks() {
  localStorage.setItem("tela_bookmarks", JSON.stringify(bookmarks));
  renderBookmarks();
  updateBookmarkButtons();
  document.dispatchEvent(new CustomEvent("bookmarksChanged"));
}

function updateBookmarkButtons() {
  const scid = scidInput.value.trim();
  const node = nodeInput.value.trim();
  const scidSaved = !!bookmarks.scids[scid];
  const nodeSaved = !!bookmarks.nodes[node];
  bookmarkScidBtn.textContent = scidSaved ? "★" : "☆";
  bookmarkScidBtn.classList.toggle("saved", scidSaved);
  bookmarkNodeBtn.textContent = nodeSaved ? "★" : "☆";
  bookmarkNodeBtn.classList.toggle("saved", nodeSaved);
  updateSettingNodeBookmark();
}

function updateSettingNodeBookmark() {
  if (!bookmarkSettingNodeBtn) return;
  const defaultNode = document.getElementById("setting-default-node");
  const val = defaultNode ? defaultNode.value.trim() : "";
  const saved = !!bookmarks.nodes[val];
  bookmarkSettingNodeBtn.textContent = saved ? "★" : "☆";
  bookmarkSettingNodeBtn.classList.toggle("saved", saved);
}

scidInput.oninput = updateBookmarkButtons;
nodeInput.oninput = updateBookmarkButtons;

function showBookmarkPopover(type, value, mode) {
  if (!bookmarkPopover || !bookmarkLabelInput) return;
  mode = mode || "save";
  bookmarkTarget = { type, value, mode };
  const existing = type === "scid" ? bookmarks.scids[value] : bookmarks.nodes[value];
  bookmarkLabelInput.value = existing?.label || "";
  bookmarkLabelInput.placeholder = type === "scid" ? "Label (default: " + value.slice(0, 8) + ")" : "Label (default: " + value + ")";
  if (mode === "remove") {
    bookmarkPopoverTitle.textContent = "Remove bookmark?";
    bookmarkLabelInput.style.display = "none";
    bookmarkPopoverSave.textContent = "Yes";
    bookmarkPopoverSave.className = "danger";
    bookmarkPopoverCancel.textContent = "Cancel";
  } else {
    bookmarkPopoverTitle.textContent = "Bookmark";
    bookmarkLabelInput.style.display = "";
    bookmarkPopoverSave.textContent = "Save";
    bookmarkPopoverSave.className = "";
    bookmarkPopoverCancel.textContent = "Cancel";
  }
  bookmarkPopover.classList.remove("hidden");
  if (mode !== "remove") {
    bookmarkLabelInput.focus();
    bookmarkLabelInput.select();
  }
}

function hideBookmarkPopover() {
  if (!bookmarkPopover) return;
  bookmarkPopover.classList.add("hidden");
  bookmarkPopoverTitle.textContent = "Bookmark";
  bookmarkLabelInput.style.display = "";
  bookmarkPopoverSave.textContent = "Save";
  bookmarkPopoverSave.className = "";
  bookmarkPopoverCancel.textContent = "Cancel";
  bookmarkTarget = null;
}

bookmarkScidBtn.onclick = () => {
  const scid = scidInput.value.trim();
  if (!scid) return alert("Enter SCID first");
  if (bookmarks.scids[scid]) { showBookmarkPopover("scid", scid, "remove"); return; }
  showBookmarkPopover("scid", scid, "save");
};

bookmarkNodeBtn.onclick = () => {
  const node = nodeInput.value.trim();
  if (!node) return alert("Enter node first");
  if (bookmarks.nodes[node]) { showBookmarkPopover("node", node, "remove"); return; }
  showBookmarkPopover("node", node, "save");
};

if (bookmarkSettingNodeBtn) {
  bookmarkSettingNodeBtn.onclick = () => {
    const defaultNode = document.getElementById("setting-default-node");
    const node = defaultNode ? defaultNode.value.trim() : "";
    if (!node) return alert("Enter node first");
    if (bookmarks.nodes[node]) { showBookmarkPopover("node", node, "remove"); return; }
    showBookmarkPopover("node", node, "save");
  };
  const settingNodeInput = document.getElementById("setting-default-node");
  if (settingNodeInput) settingNodeInput.addEventListener("input", updateSettingNodeBookmark);
}

bookmarkPopoverSave.onclick = () => {
  if (!bookmarkTarget) return;
  if (bookmarkTarget.mode === "remove") {
    if (bookmarkTarget.type === "scid") { delete bookmarks.scids[bookmarkTarget.value]; }
    else { delete bookmarks.nodes[bookmarkTarget.value]; }
    saveBookmarks();
    pushToast("warning", "Bookmark removed");
    hideBookmarkPopover();
    return;
  }
  const label = bookmarkLabelInput.value.trim() ||
    (bookmarkTarget.type === "scid" ? bookmarkTarget.value.slice(0, 8) : bookmarkTarget.value);
  if (bookmarkTarget.type === "scid") {
    bookmarks.scids[bookmarkTarget.value] = { scid: bookmarkTarget.value, label };
  } else {
    bookmarks.nodes[bookmarkTarget.value] = { node: bookmarkTarget.value, label };
  }
  saveBookmarks();
  pushToast("connected", "Bookmark saved");
  hideBookmarkPopover();
};

bookmarkPopoverCancel.onclick = hideBookmarkPopover;

bookmarkPopover.addEventListener("click", (e) => {
  if (e.target === bookmarkPopover) hideBookmarkPopover();
});

bookmarkLabelInput.addEventListener("keydown", (e) => {
  if (e.key === "Enter") { e.preventDefault(); bookmarkPopoverSave.click(); }
  if (e.key === "Escape") { e.preventDefault(); bookmarkPopoverCancel.click(); }
});

function renderBookmarks() {
  if (!bookmarkedNodesEl || !bookmarkedScidsEl) return;
  bookmarkedNodesEl.replaceChildren();
  bookmarkedScidsEl.replaceChildren();
  const nodes = Object.values(bookmarks.nodes);
  const scids = Object.values(bookmarks.scids);
  if (!nodes.length) bookmarkedNodesEl.appendChild(createNoResults("No bookmarked nodes"));
  else nodes.forEach(b => bookmarkedNodesEl.appendChild(createBookmarkItem(
    b.label, b.node,
    () => loadBookmarkedNode(b.node),
    () => { delete bookmarks.nodes[b.node]; saveBookmarks(); }
  )));
  if (!scids.length) bookmarkedScidsEl.appendChild(createNoResults("No bookmarked SCIDs"));
  else scids.forEach(b => bookmarkedScidsEl.appendChild(createBookmarkItem(
    b.label, b.scid,
    () => { scidInput.value = b.scid; updateBookmarkButtons(); loadBtn.click(); },
    () => { delete bookmarks.scids[b.scid]; saveBookmarks(); }
  )));
  const badge = document.getElementById("bookmark-badge");
  if (badge) badge.textContent = scids.length + nodes.length > 0 ? scids.length + nodes.length : "";
}

function createBookmarkItem(label, value, onLoad, onRemove) {
  // Delegates to the canonical component (web/ui/components/BookmarkItem).
  return UI.BookmarkItem({
    label: label,
    value: value,
    onLoad: onLoad,
    onRemove: onRemove,
    onCommit: (newLabel) => {
      const target = value.length === 64 ? bookmarks.scids : bookmarks.nodes;
      if (target[value]) { target[value].label = newLabel || value.slice(0, 8); saveBookmarks(); }
    }
  }).element;
}

// ================= INIT BOOKMARKS =================
function initBookmarks() {
  const stored = localStorage.getItem("tela_bookmarks");
  if (stored) { bookmarks = JSON.parse(stored); }
  else { bookmarks = JSON.parse(JSON.stringify(defaultBookmarks)); localStorage.setItem("tela_bookmarks", JSON.stringify(bookmarks)); }
  renderBookmarks();
  updateBookmarkButtons();
}

// ================= SETTINGS =================
function saveSettings() {
  localStorage.setItem("hyperwolf_settings", JSON.stringify(settings));
  fetch("/api/settings", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      settings: {
        defaultNode: settings.defaultNode,
        autoConnect: settings.autoConnect,
        directLoad: settings.directLoad,
        openDashboardOnStart: settings.openDashboardOnStart,
        hiddenExtensions: settings.hiddenExtensions,
        showSearchCards: settings.showSearchCards,
        showTopBar: settings.showTopBar,
        searchGradient: settings.searchGradient,
        checkUpdates: settings.checkUpdates,
        rssFeedUrl: settings.rssFeedUrl,
        fontScale: settings.fontScale
      }
    })
  }).catch(e => console.warn("Failed to save settings to server:", e));
}

async function loadSettings() {
  const stored = localStorage.getItem("hyperwolf_settings");
  if (stored) {
    const parsed = JSON.parse(stored);
    settings = { ...settings, ...parsed };
  }
  // Pull from server to fill gaps (e.g. after browser cache clear)
  let serverHadDefault = false;
  try {
    const r = await fetch("/api/settings");
    const d = await r.json();
    if (d.ok && d.result?.settings) {
      const sv = d.result.settings;
      if (sv.defaultNode) {
        serverHadDefault = true;
        if (!settings.defaultNode) settings.defaultNode = sv.defaultNode;
      }
    }
  } catch (e) {}
  // One-time sync: push local default to server if missing
  if (settings.defaultNode && !serverHadDefault) {
    saveSettings();
  }
  const defaultNodeInput = document.getElementById("setting-default-node");
  if (defaultNodeInput) defaultNodeInput.value = settings.defaultNode || "";
  const directLoadInput = document.getElementById("setting-direct-load");
  if (directLoadInput) directLoadInput.checked = settings.directLoad !== false;
  const autoConnectInput = document.getElementById("setting-auto-connect");
  if (autoConnectInput) autoConnectInput.checked = settings.autoConnect !== false;
  const checkUpdatesInput = document.getElementById("setting-check-updates");
  if (checkUpdatesInput) checkUpdatesInput.checked = settings.checkUpdates !== false;
  const rssFeedUrlInput = document.getElementById("setting-rss-feed-url");
  if (rssFeedUrlInput) rssFeedUrlInput.value = settings.rssFeedUrl || "https://dero.world/anotherworld/feed/";
  if (window.renderTags) window.renderTags();
  updateSettingNodeBookmark();
  applyTopBar();
  applySearchGradient();
  applyFontScale();
}

function initToggleSwitch(id, settingKey) {
  const el = document.getElementById(id);
  if (!el) return;
  el.checked = settings[settingKey] !== false;
  el.addEventListener("change", () => {
    settings[settingKey] = el.checked;
    saveSettings();
    pushToast("connected", "Setting saved");
  });
}

function initTagInput() {
  const container = document.getElementById("tag-input-container");
  const list = document.getElementById("tag-list");
  const field = document.getElementById("tag-input-field");
  if (!container || !list || !field) return;
  window.renderTags = () => {
    list.replaceChildren();
    const exts = window.getHiddenExtensions();
    exts.forEach(ext => {
      list.appendChild(UI.TagChip(ext, {
        onRemove: (val) => {
          settings.hiddenExtensions = exts.filter(e => e !== val).join(", ");
          saveSettings();
          window.renderTags();
        },
      }));
    });
  };
  function addTag(val) {
    const v = val.trim().toLowerCase();
    if (!v) return;
    const exts = window.getHiddenExtensions();
    if (!exts.includes(v)) {
      settings.hiddenExtensions = [...exts, v].join(", ");
      saveSettings();
    }
  }
  field.addEventListener("keydown", (e) => {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      addTag(field.value);
      field.value = "";
      window.renderTags();
    }
  });
  container.addEventListener("click", (e) => {
    if (e.target === container) field.focus();
  });
  window.renderTags();
}

function applyTopBar() {
  const hidden = settings.showTopBar === false;
  header.classList.toggle("header-hidden", hidden);
}

const gradientClasses = ["g-default", "g-ocean", "g-sunset", "g-aurora", "g-indigo-rose", "g-instagram"];

function applySearchGradient() {
  gradientClasses.forEach(c => document.body.classList.remove(c));
  const gradient = settings.searchGradient || "default";
  document.body.classList.add("g-" + gradient);
}

function applyFontScale() {
  const scale = Number(settings.fontScale) || 1;
  document.body.style.zoom = String(scale);
  // Compensate the fixed 100vh body height for the zoom factor so the scaled
  // content still fills the viewport — otherwise Small leaves an empty band at
  // the bottom and Large pushes content past the visible area.
  document.body.style.height = (100 / scale) + "vh";
}

function initSettingsUI() {
  initToggleSwitch("setting-direct-load", "directLoad");
  initToggleSwitch("setting-auto-connect", "autoConnect");
  initToggleSwitch("setting-open-dashboard", "openDashboardOnStart");
  initToggleSwitch("setting-show-search-cards", "showSearchCards");
  initToggleSwitch("setting-show-topbar", "showTopBar");
  initToggleSwitch("setting-check-updates", "checkUpdates");
  const topbarToggle = document.getElementById("setting-show-topbar");
  if (topbarToggle) topbarToggle.addEventListener("change", applyTopBar);

  const gradientSelect = document.getElementById("setting-search-gradient");
  if (gradientSelect) {
    gradientSelect.value = settings.searchGradient || "default";
    gradientSelect.addEventListener("change", () => {
      settings.searchGradient = gradientSelect.value;
      saveSettings();
      applySearchGradient();
      pushToast("connected", "Search background updated");
    });
  }
  const fontScaleOptions = document.getElementById("setting-font-size-options");
  if (fontScaleOptions) {
    const currentScale = String(Number(settings.fontScale) || 1);
    fontScaleOptions.querySelectorAll("[data-font-scale]").forEach(btn => {
      btn.classList.toggle("active", btn.dataset.fontScale === currentScale);
      btn.addEventListener("click", () => {
        const scale = Number(btn.dataset.fontScale);
        settings.fontScale = scale;
        saveSettings();
        applyFontScale();
        fontScaleOptions.querySelectorAll("[data-font-scale]").forEach(b => b.classList.toggle("active", b === btn));
        pushToast("connected", "Font size updated");
      });
    });
  }
  const nodeInput = document.getElementById("setting-default-node");
  if (nodeInput) {
    nodeInput.addEventListener("change", () => {
      settings.defaultNode = nodeInput.value.trim();
      saveSettings();
      pushToast("connected", "Setting saved");
    });
  }
  
  // RSS Feed URL setting
  const rssFeedUrlInput = document.getElementById("setting-rss-feed-url");
  if (rssFeedUrlInput) {
    rssFeedUrlInput.value = settings.rssFeedUrl || "https://dero.world/anotherworld/feed/";
    rssFeedUrlInput.addEventListener("change", () => {
      settings.rssFeedUrl = rssFeedUrlInput.value.trim();
      saveSettings();
      // Notify search page to update
      if (window.updateLiveInfoSettings) {
        window.updateLiveInfoSettings({ rssFeedUrl: settings.rssFeedUrl });
      }
      pushToast("connected", "RSS feed URL updated");
    });
  }
  
  initTagInput();
  const resetBtn = document.getElementById("reset-settings-btn");
  if (resetBtn) {
    resetBtn.addEventListener("click", () => {
      showConfirmPopover("Reset all settings?", "This will reset every setting to its original value.", () => {
        settings = { defaultNode: "", autoConnect: true, directLoad: true, openDashboardOnStart: true, hiddenExtensions: "", showSearchCards: true, showTopBar: false, searchGradient: "default", checkUpdates: true, rssFeedUrl: "https://dero.world/anotherworld/feed/", fontScale: 1 };
        localStorage.removeItem("hyperwolf_settings");
        loadSettings();
        pushToast("connected", "Settings reset to defaults");
      });
    });
  }
}

// Confirm popover
let confirmCallback = null;

function showConfirmPopover(title, message, onConfirm) {
  const popover = document.getElementById("confirm-popover");
  const titleEl = document.getElementById("confirm-popover-title");
  const msgEl = document.getElementById("confirm-popover-message");
  if (!popover || !titleEl || !msgEl) return;
  titleEl.textContent = title;
  msgEl.textContent = message;
  confirmCallback = onConfirm;
  popover.classList.remove("hidden");
}
document.getElementById("confirm-popover-yes").addEventListener("click", () => {
  if (confirmCallback) confirmCallback();
  document.getElementById("confirm-popover").classList.add("hidden");
  confirmCallback = null;
});
document.getElementById("confirm-popover-no").addEventListener("click", () => {
  document.getElementById("confirm-popover").classList.add("hidden");
  confirmCallback = null;
});
document.getElementById("confirm-popover").addEventListener("click", (e) => {
  if (e.target === document.getElementById("confirm-popover")) {
    document.getElementById("confirm-popover").classList.add("hidden");
    confirmCallback = null;
  }
});

// ================= ONBOARDING =================
const onboardingPopover = document.getElementById("onboarding-popover");
const onboardingNodeList = document.getElementById("onboarding-node-list");
const onboardingNodeInput = document.getElementById("onboarding-node-input");
const onboardingSkipBtn = document.getElementById("onboarding-skip");
const onboardingConnectBtn = document.getElementById("onboarding-connect");
let selectedOnboardingNode = "";

/**
 * A "fresh install" is a first run: the user has never saved settings,
 * never picked a default node (local or server-side), and has not already
 * dismissed the onboarding prompt.
 * @returns {boolean}
 */
function isFreshInstall() {
  if (localStorage.getItem("hyperwolf_onboarding_done")) return false;
  if (localStorage.getItem("hyperwolf_settings")) return false;
  if (settings.defaultNode) return false;
  return true;
}

function renderOnboardingNodes() {
  if (!onboardingNodeList) return;
  onboardingNodeList.replaceChildren();
  const nodes = bookmarks.nodes || {};
  const entries = Object.entries(nodes);
  if (!entries.length) {
    const empty = document.createElement("div");
    empty.className = "onboarding-empty";
    empty.textContent = "No bookmarked nodes available — enter your own below.";
    onboardingNodeList.appendChild(empty);
    return;
  }
  entries.forEach(([addr, info]) => {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "onboarding-node-option";
    btn.dataset.node = addr;
    const label = document.createElement("span");
    label.className = "onb-label";
    label.textContent = (info && info.label) || "Node";
    const host = document.createElement("span");
    host.className = "onb-addr";
    host.textContent = addr;
    btn.append(label, host);
    btn.addEventListener("click", () => {
      onboardingNodeList.querySelectorAll(".onboarding-node-option").forEach(b => b.classList.remove("selected"));
      btn.classList.add("selected");
      selectedOnboardingNode = addr;
      if (onboardingNodeInput) onboardingNodeInput.value = "";
    });
    onboardingNodeList.appendChild(btn);
  });
}

function showOnboardingPopover() {
  if (!onboardingPopover || !isFreshInstall()) return;
  renderOnboardingNodes();
  selectedOnboardingNode = "";
  if (onboardingNodeInput) onboardingNodeInput.value = "";
  onboardingPopover.classList.remove("hidden");
  setTimeout(() => { if (onboardingNodeInput) onboardingNodeInput.focus(); }, 50);
}

function hideOnboardingPopover() {
  if (!onboardingPopover) return;
  onboardingPopover.classList.add("hidden");
}

/**
 * Applies the chosen node exactly like the Settings "Default Node" field:
 * persists it (localStorage + server), keeps auto-connect on, then connects
 * through the normal header connect flow.
 * @param {string} node - Node address to set as default and connect to
 * @returns {void}
 */
function completeOnboarding(node) {
  localStorage.setItem("hyperwolf_onboarding_done", "1");
  hideOnboardingPopover();
  settings.defaultNode = node;
  settings.autoConnect = true;
  saveSettings();
  const defaultNodeInput = document.getElementById("setting-default-node");
  if (defaultNodeInput) defaultNodeInput.value = node;
  nodeInput.value = node;
  updateBookmarkButtons();
  connectNodeBtn.click();
}

function skipOnboarding() {
  localStorage.setItem("hyperwolf_onboarding_done", "1");
  hideOnboardingPopover();
  pushToast("warning", "Set a default node anytime in Settings");
}

if (onboardingConnectBtn) {
  onboardingConnectBtn.addEventListener("click", () => {
    const custom = onboardingNodeInput ? onboardingNodeInput.value.trim() : "";
    const node = custom || selectedOnboardingNode;
    if (!node) {
      pushToast("warning", "Pick a node or enter one first");
      return;
    }
    completeOnboarding(node);
  });
}
if (onboardingSkipBtn) onboardingSkipBtn.addEventListener("click", skipOnboarding);
if (onboardingPopover) {
  onboardingPopover.addEventListener("click", (e) => {
    if (e.target === onboardingPopover) skipOnboarding();
  });
}
if (onboardingNodeInput) {
  onboardingNodeInput.addEventListener("input", () => {
    if (!onboardingNodeInput.value.trim()) return;
    onboardingNodeList.querySelectorAll(".onboarding-node-option").forEach(b => b.classList.remove("selected"));
    selectedOnboardingNode = "";
  });
  onboardingNodeInput.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      if (onboardingConnectBtn) onboardingConnectBtn.click();
    }
  });
}

function addCopyButtons() {
  document.querySelectorAll("pre").forEach(pre => {
    if (pre.querySelector(".copy-btn")) return;
    const button = document.createElement("button");
    button.textContent = "Copy";
    button.className = "copy-btn";
    button.addEventListener("click", async () => {
      const code = pre.querySelector("code");
      if (!code) return;
      try {
        await navigator.clipboard.writeText(code.innerText);
        button.textContent = "Copied ✓";
        button.classList.add("copied");
        setTimeout(() => { button.textContent = "Copy"; button.classList.remove("copied"); }, 1500);
      } catch (err) { console.error("Copy failed:", err); }
    });
    pre.appendChild(button);
  });
}
document.addEventListener("DOMContentLoaded", addCopyButtons);

// ================= SYNC PROGRESS =================
let syncStartHeight = null;
let chainSynced = false;

function updateSyncProgress(indexed, chain) {
  const syncInfo  = document.getElementById("sync-info");
  const syncLabel = document.getElementById("sync-label");
  const syncBar   = document.getElementById("sync-bar");
  const daemonEl  = document.getElementById("daemon-height");
  const dbEl      = document.getElementById("db-height");
  const etaEl     = document.getElementById("sync-eta");
  if (!syncInfo) return;
  syncInfo.style.display = "block";
  if (daemonEl) daemonEl.textContent = chain > 0 ? chain.toLocaleString() : "—";
  if (dbEl)     dbEl.textContent     = indexed > 0 ? indexed.toLocaleString() : "—";
  if (chainSynced) {
    if (indexed > 0 && indexed < chain - 10 && chain > 20) {
      chainSynced = false;
      syncStartHeight = null;
      syncStartTime = null;
    } else { return; }
  }
  if (indexed >= chain - 3 && chain > 100) {
    markTipSynced();
    if (etaEl) etaEl.textContent = "";
    return;
  }
  if (indexed === 0 && chain > 0) {
    syncStartHeight = null;
    syncStartTime = null;
    if (syncBar) { syncBar.style.width = "0%"; syncBar.classList.add("indeterminate"); }
    if (syncLabel) syncLabel.textContent = "⏳ Syncing chain...";
    if (etaEl) etaEl.textContent = "";
  } else {
    if (syncStartHeight === null) { syncStartHeight = indexed; syncStartTime = Date.now(); }
    const total = chain - syncStartHeight;
    const done  = indexed - syncStartHeight;
    const pct   = total > 0 ? Math.min(100, (done / total) * 100) : 0;
    if (syncBar) { syncBar.classList.remove("indeterminate"); syncBar.style.width = pct.toFixed(1) + "%"; }
    if (syncLabel) syncLabel.textContent = pct.toFixed(1) + "% chain synced";
    if (etaEl && done > 5 && total > 0) {
      const elapsed = (Date.now() - syncStartTime) / 1000;
      const rate = elapsed > 0 ? done / elapsed : 0;
      const remaining = total - done;
      const etaSecs = rate > 0 ? remaining / rate : 0;
      if (etaSecs > 120) { etaEl.textContent = "~" + Math.round(etaSecs / 60) + " min remaining"; }
      else if (etaSecs > 0) { etaEl.textContent = "~" + Math.round(etaSecs) + "s remaining"; }
    }
  }
}

function markTipSynced() {
  chainSynced = true;
  const syncLabel = document.getElementById("sync-label");
  const syncBar   = document.getElementById("sync-bar");
  syncStartHeight = null;
  if (syncBar) { syncBar.classList.remove("indeterminate"); syncBar.style.width = "100%"; }
  if (syncLabel) syncLabel.textContent = "Chain synced";
}

function resetSyncProgress() {
  syncStartHeight = null;
  chainSynced = false;
  lastChainHeight = 0;
  lastBlockTime = null;
  const syncInfo  = document.getElementById("sync-info");
  const syncBar   = document.getElementById("sync-bar");
  const syncLabel = document.getElementById("sync-label");
  const daemonEl  = document.getElementById("daemon-height");
  const dbEl      = document.getElementById("db-height");
  if (syncInfo)  syncInfo.style.display = "block";
  if (syncBar)   { syncBar.classList.remove("indeterminate"); syncBar.style.width = "0%"; }
  if (syncLabel) syncLabel.textContent = "Disconnected";
  if (daemonEl)  daemonEl.textContent = "—";
  if (dbEl)      dbEl.textContent = "—";
  const svNode = document.getElementById("sv-node");
  if (svNode) svNode.textContent = "—";
  const svVersion = document.getElementById("sv-version");
  if (svVersion) svVersion.textContent = "—";
  const svNetwork = document.getElementById("sv-network");
  if (svNetwork) svNetwork.textContent = "—";
  const svTipAge = document.getElementById("sv-tip-age");
  if (svTipAge) svTipAge.textContent = "—";
  const svUptime = document.getElementById("sv-uptime");
  if (svUptime) svUptime.textContent = "—";
  const svDifficulty = document.getElementById("sv-difficulty");
  if (svDifficulty) svDifficulty.textContent = "—";
  const svMempool = document.getElementById("sv-mempool");
  if (svMempool) svMempool.textContent = "—";
  const svTelaCount = document.getElementById("sv-tela-count");
  if (svTelaCount) svTelaCount.textContent = "—";
  const svTelaStatus = document.getElementById("sv-tela-status");
  if (svTelaStatus) svTelaStatus.textContent = "—";
  // Reset search page status card
  const scTelaCount = document.getElementById("sc-tela-count");
  if (scTelaCount) scTelaCount.textContent = "—";
  const scNodeStatus = document.getElementById("sc-node-status");
  if (scNodeStatus) { scNodeStatus.textContent = "—"; scNodeStatus.style.color = ""; }
  const scTelaStatus = document.getElementById("sc-tela-status");
  if (scTelaStatus) { scTelaStatus.textContent = "—"; scTelaStatus.style.color = ""; }
  const scGnomonStatus = document.getElementById("sc-gnomon-status");
  if (scGnomonStatus) { scGnomonStatus.textContent = "—"; scGnomonStatus.style.color = ""; }
  const scXswdStatus = document.getElementById("sc-xswd-status");
  if (scXswdStatus) { scXswdStatus.textContent = "—"; scXswdStatus.style.color = ""; }
}

// ================= WEBSOCKET =================
let ws = null;
let wsReconnectTimer = null;

function connectWebSocket() {
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return;
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  ws = new WebSocket(proto + "//" + location.host + "/ws");

  ws.onopen = () => {
    console.log("WebSocket connected");
    if (wsReconnectTimer) { clearTimeout(wsReconnectTimer); wsReconnectTimer = null; }
  };

  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      handleEvent(msg);
    } catch (e) {
      console.warn("WS message parse error:", e);
    }
  };

ws.onclose = () => {
    console.log("WebSocket disconnected, reconnecting in 3s");
    wsReconnectTimer = setTimeout(connectWebSocket, 3000);
  };

  ws.onerror = (err) => {
    console.warn("WebSocket error:", err);
    ws.close();
  };
}

// =====================
// LOGS PAGE
// =====================

let currentLogLevelFilter = "";

/**
 * Fetches logs from the server and renders them in the log container.
 * @param {string} levelFilter - Optional level to filter by (INFO, WARN, ERROR, SUCCESS)
 */
async function refreshLogs(levelFilter = "") {
  currentLogLevelFilter = levelFilter;
  const logContainer = document.getElementById("log-container");
  if (!logContainer) return;

  try {
    const params = new URLSearchParams();
    if (levelFilter) params.set("level", levelFilter);

    const res = await fetch("/api/logs" + (params.toString() ? "?" + params.toString() : ""));
    const data = await res.json();

    if (!data.ok || !data.result) {
      logContainer.innerHTML = '<div class="log-empty">Failed to load logs</div>';
      return;
    }

    const logs = data.result.logs || [];
    if (!logs.length) {
      logContainer.innerHTML = '<div class="log-empty">No logs to display</div>';
      return;
    }

    // Render newest-first (latest at top) to match live WS appends, which
    // prepend at the top of the list.
    logs.reverse();

    logContainer.innerHTML = logs.map(entry => createLogEntry(entry)).join("");
    applyAutoScroll();
  } catch (e) {
    console.warn("Logs fetch error:", e);
    logContainer.innerHTML = '<div class="log-empty">Error loading logs</div>';
  }
}

/**
 * Creates an HTML string for a single log entry.
 * @param {Object} entry - Log entry object with timestamp, level, and message
 * @returns {string} HTML string for the log entry
 */
function createLogEntry(entry) {
  return UI.LogEntry(entry);
}

/**
 * Prepends a single log entry to the top of the log container as it arrives
 * over the WebSocket. Honors the current level filter, replaces empty-state
 * placeholders, caps the DOM size, and sticks to the top when the page is
 * visible with auto-follow enabled.
 * @param {Object} entry - Log entry object with timestamp, level, and message
 * @returns {void}
 */
function appendLogEntry(entry) {
  const logContainer = document.getElementById("log-container");
  if (!logContainer) return;

  // Respect the active level filter (empty = all levels).
  if (currentLogLevelFilter && entry.level !== currentLogLevelFilter) return;

  const empty = logContainer.querySelector(".log-empty");
  if (empty) empty.remove();

  const pageVisible = document.getElementById("page-logs")?.classList.contains("active");
  const autoFollow = document.getElementById("log-auto-scroll");
  const shouldStick = pageVisible && (!autoFollow || autoFollow.checked);

  const wrapper = document.createElement("div");
  wrapper.innerHTML = createLogEntry(entry).trim();
  const row = wrapper.firstElementChild;
  if (row) logContainer.prepend(row);

  // Cap the number of rendered rows to avoid unbounded DOM growth
  // (prune the oldest rows from the bottom).
  const maxRows = 1000;
  while (logContainer.children.length > maxRows) {
    logContainer.lastChild.remove();
  }

  if (shouldStick) logContainer.scrollTop = 0;
}

/**
 * Sticks the log view to the top (the latest entries) when auto-follow is on.
 */
function applyAutoScroll() {
  const autoFollow = document.getElementById("log-auto-scroll");
  const logContainer = document.getElementById("log-container");
  if (autoFollow && autoFollow.checked && logContainer) {
    logContainer.scrollTop = 0;
  }
}

// Event listeners for log page controls
document.addEventListener("DOMContentLoaded", () => {
  const logLevelFilter = document.getElementById("log-level-filter");
  const logRefreshBtn = document.getElementById("log-refresh-btn");
  const logClearBtn = document.getElementById("log-clear-btn");
  const logAutoScroll = document.getElementById("log-auto-scroll");
  const logContainer = document.getElementById("log-container");

  if (logLevelFilter) {
    logLevelFilter.addEventListener("change", () => {
      refreshLogs(logLevelFilter.value);
    });
  }

  if (logRefreshBtn) {
    logRefreshBtn.addEventListener("click", () => {
      refreshLogs(currentLogLevelFilter);
    });
  }

  if (logClearBtn) {
    logClearBtn.addEventListener("click", () => {
      if (!confirm("Clear all displayed logs from the browser? (This doesn't clear server-side logs.)")) return;
      if (logContainer) {
        logContainer.innerHTML = '<div class="log-empty">Logs cleared</div>';
      }
    });
  }

  // Stop auto-follow when user manually scrolls away from the top
  // (latest entries live at the top).
  if (logContainer && logAutoScroll) {
    logContainer.addEventListener("scroll", () => {
      // If user scrolls down away from the newest entries, disable auto-follow
      if (logContainer.scrollTop > 10) {
        logAutoScroll.checked = false;
      }
    });
  }

  // Load logs when the logs page becomes visible
  document.addEventListener("pageChanged", (e) => {
    if (e.detail.page === "logs") {
      refreshLogs(currentLogLevelFilter);
    }
  });
});

function handleEvent(msg) {
  // Dispatch to any listeners (search.js etc.)
  document.dispatchEvent(new CustomEvent("wsEvent", { detail: msg }));

  if (msg.event === "log_entry" && msg.entry) {
    appendLogEntry(msg.entry);
  } else if (msg.event === "sync_progress") {
    updateSyncProgress(msg.indexed, msg.chain);
  } else if (msg.event === "tip_synced") {
    markTipSynced();
  } else if (msg.event === "catalog_progress") {
    const statusEl = document.getElementById("sv-tela-status");
    if (statusEl && msg.filtered > 0 && msg.filtered < msg.total) {
      statusEl.textContent = "Scanning: " + msg.filtered.toLocaleString() + " / " + msg.total.toLocaleString();
    } else if (statusEl && msg.total > 0) {
      statusEl.textContent = "All discovered";
    }
  } else if (msg.event === "node_unreachable") {
    const nodeStr = typeof msg.node === "string" ? msg.node.replace("http://", "") : "";
    pushToast("warning", "Node unreachable: " + nodeStr);
    if (sidebarTelaStatus) setStatus(sidebarTelaStatus, false);
    if (sidebarGnomonStatus) setStatus(sidebarGnomonStatus, false);
    resetSyncProgress();
  } else if (msg.event === "node_recovered") {
    pushToast("connected", "Node recovered!");
    updateStatusIndicators();
  }
}
