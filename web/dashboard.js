/**
 * @module dashboard
 * @description Core dashboard logic for HyperWolf. Manages SPA navigation,
 * WebSocket connection, node connection/disconnection, SCID loading,
 * bookmarks, settings persistence, sync progress visualization, and
 * toast notifications. All state is module-scoped within this IIFE.
 */

// ================= ELEMENTS =================
const sidebar = document.getElementById("sidebar");
const sidebarBackdrop = document.getElementById("sidebar-backdrop");
const menuToggleBtn = document.getElementById("menu-toggle");
const sidebarCloseBtn = document.getElementById("sidebar-close");
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
let settings = { defaultNode: "", autoConnect: true, directLoad: true, openDashboardOnStart: true, hiddenExtensions: "" };
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

window.getHiddenExtensions = () => {
  return (settings.hiddenExtensions || "")
    .split(",")
    .map(e => e.trim().toLowerCase())
    .filter(Boolean);
};

// ================= THEME =================
const savedTheme = localStorage.getItem("theme") || "dark";
document.documentElement.setAttribute("data-theme", savedTheme);

/** Toggles between light and dark theme, persisting to localStorage. */
themeToggle.onclick = () => {
  const next = document.documentElement.getAttribute("data-theme") === "light" ? "dark" : "light";
  document.documentElement.setAttribute("data-theme", next);
  localStorage.setItem("theme", next);
};

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
if (sidebarCloseBtn) sidebarCloseBtn.onclick = closeSidebar;
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
  const span = document.createElement("span");
  span.className = "status-dot " + state;
  return span;
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
  const div = document.createElement("div");
  div.className = "no-results";
  div.textContent = text;
  return div;
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
  const toast = document.createElement("div");
  toast.className = "toast toast-" + state;
  const dot = createDot(state);
  const msg = document.createElement("span");
  msg.className = "toast-msg";
  msg.textContent = message;
  const close = document.createElement("button");
  close.className = "toast-close";
  close.textContent = "✕";
  close.onclick = () => dismissToast(toast);
  toast.append(dot, msg, close);
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
});

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
    () => { nodeInput.value = b.node; updateBookmarkButtons(); if (connectNodeBtn.textContent === "Connect") connectNodeBtn.click(); },
    () => { delete bookmarks.nodes[b.node]; saveBookmarks(); }
  )));
  if (!scids.length) bookmarkedScidsEl.appendChild(createNoResults("No bookmarked SCIDs"));
  else scids.forEach(b => bookmarkedScidsEl.appendChild(createBookmarkItem(
    b.label, b.scid,
    () => { scidInput.value = b.scid; updateBookmarkButtons(); },
    () => { delete bookmarks.scids[b.scid]; saveBookmarks(); }
  )));
  const badge = document.getElementById("bookmark-badge");
  if (badge) badge.textContent = scids.length + nodes.length > 0 ? scids.length + nodes.length : "";
}

function createBookmarkItem(label, value, onLoad, onRemove) {
  const root = document.createElement("div");
  root.className = "bookmark-item";
  const info = document.createElement("div");
  info.className = "bookmark-info";
  const l = document.createElement("div");
  l.className = "bookmark-label";
  l.textContent = label;
  const v = document.createElement("div");
  v.className = "bookmark-value";
  v.textContent = value;
  const actions = document.createElement("div");
  actions.className = "bookmark-actions";
  const load = document.createElement("button");
  load.className = "small";
  load.textContent = "Load";
  load.onclick = onLoad;
  const edit = document.createElement("button");
  edit.className = "small";
  edit.textContent = "✏";
  edit.title = "Edit label";
  edit.onclick = (e) => {
    e.stopPropagation();
    l.contentEditable = "true";
    l.focus();
    const range = document.createRange();
    range.selectNodeContents(l);
    const sel = window.getSelection();
    sel.removeAllRanges();
    sel.addRange(range);
  };
  l.addEventListener("blur", () => {
    l.contentEditable = "false";
    const target = value.length === 64 ? bookmarks.scids : bookmarks.nodes;
    if (target[value]) { target[value].label = l.textContent || value.slice(0, 8); saveBookmarks(); }
  });
  l.addEventListener("keydown", (e) => {
    if (e.key === "Enter") { e.preventDefault(); l.blur(); }
  });
  const remove = document.createElement("button");
  remove.className = "small danger";
  remove.textContent = "Remove";
  remove.onclick = onRemove;
  info.append(l, v);
  actions.append(load, edit, remove);
  root.append(info, actions);
  return root;
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
        hiddenExtensions: settings.hiddenExtensions
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
  if (window.renderTags) window.renderTags();
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
      const chip = document.createElement("span");
      chip.className = "tag-chip";
      chip.textContent = ext;
      const remove = document.createElement("button");
      remove.className = "tag-remove";
      remove.textContent = "✕";
      remove.onclick = () => {
        settings.hiddenExtensions = exts.filter(e => e !== ext).join(", ");
        saveSettings();
        window.renderTags();
      };
      chip.appendChild(remove);
      list.appendChild(chip);
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

function initSettingsUI() {
  initToggleSwitch("setting-direct-load", "directLoad");
  initToggleSwitch("setting-auto-connect", "autoConnect");
  initToggleSwitch("setting-open-dashboard", "openDashboardOnStart");
  const nodeInput = document.getElementById("setting-default-node");
  if (nodeInput) {
    nodeInput.addEventListener("change", () => {
      settings.defaultNode = nodeInput.value.trim();
      saveSettings();
      pushToast("connected", "Setting saved");
    });
  }
  initTagInput();
  const resetBtn = document.getElementById("reset-settings-btn");
  if (resetBtn) {
    resetBtn.addEventListener("click", () => {
      showConfirmPopover("Reset all settings?", "This will reset every setting to its original value.", () => {
        settings = { defaultNode: "", autoConnect: true, directLoad: true, openDashboardOnStart: true, hiddenExtensions: "" };
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

function handleEvent(msg) {
  // Dispatch to any listeners (search.js etc.)
  document.dispatchEvent(new CustomEvent("wsEvent", { detail: msg }));

  if (msg.event === "sync_progress") {
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
