import './style.css';
import {
  AddServer,
  GetStatus,
  GetClientToken,
  GetUIPath,
  GetLocale,
  GetI18nCatalog,
  GetNotificationHistory,
  GetHistoryMode,
  SetHistoryMode,
  ClearNotificationHistory,
  LeaveNotificationHistory,
  FormatHistoryEntryMarkdown,
  ListApplicationsForServer,
  SendApplicationMessageToServer,
  SetActiveServer,
  RemoveServer,
  RenameServer,
  LeaveSendMessage,
  SaveWindowState,
} from '../bindings/github.com/elpatron68/gotify-desktop/app.js';
import {Clipboard, Events} from '@wailsio/runtime';
import {applyDom, loadCatalog, t} from './i18n.js';

// OS toasts are native ("Gotify Desktop"). Block WebView Notification so
// nothing is attributed as "wails.localhost".
try {
  const Denied = function () {};
  Denied.permission = 'denied';
  Denied.requestPermission = () => Promise.resolve('denied');
  window.Notification = Denied;
} catch (_) {
  /* ignore */
}

const setupView = document.getElementById('setup-view');
const gotifyView = document.getElementById('gotify-view');
const historyView = document.getElementById('history-view');
const sendView = document.getElementById('send-view');
const historyList = document.getElementById('history-list');
const historyEmpty = document.getElementById('history-empty');
const historyToast = document.getElementById('history-toast');
const historyModeAll = document.getElementById('history-mode-all');
const sendForm = document.getElementById('send-form');
const sendServer = document.getElementById('send-server');
const sendApp = document.getElementById('send-app');
const sendTitle = document.getElementById('send-message-title');
const sendBody = document.getElementById('send-message-body');
const sendPriority = document.getElementById('send-priority');
const sendError = document.getElementById('send-error');
const sendSubmit = document.getElementById('send-submit');
const sendAppsEmpty = document.getElementById('send-apps-empty');
const sendNotConfigured = document.getElementById('send-not-configured');
const sendToast = document.getElementById('send-toast');
const frame = document.getElementById('gotify-frame');
const form = document.getElementById('setup-form');
const errorEl = document.getElementById('error');
const statusLine = document.getElementById('status-line');
const connectBtn = document.getElementById('connect-btn');
const serverInput = document.getElementById('server-url');
const usernameInput = document.getElementById('username');
const serverLabelInput = document.getElementById('server-label');
const serverListSection = document.getElementById('server-list-section');
const serverListEl = document.getElementById('server-list');

/** @type {'setup'|'gotify'|'history'|'send'|null} */
let currentView = null;
let bootstrapped = false;
/** @type {string|null} */
let activeGotifyServerId = null;
/** @type {Array<{id:number,name:string,description?:string,defaultPriority?:number}>} */
let sendAppsCache = [];
/** @type {Array<{id:string,label:string,serverUrl:string,active?:boolean}>} */
let serversCache = [];

function showError(msg) {
  if (!msg) {
    errorEl.hidden = true;
    errorEl.textContent = '';
    return;
  }
  errorEl.hidden = false;
  errorEl.textContent = msg;
}

function applySession(token) {
  if (!token) return;
  try {
    document.cookie = `gotify-client-token=${encodeURIComponent(token)}; path=/; max-age=604800; SameSite=Lax`;
  } catch (_) {
    /* ignore */
  }
}

function updateConnectButtonLabel(configured) {
  if (!connectBtn) return;
  connectBtn.textContent = configured ? t('setup.add_server') : t('setup.connect');
}

function renderServerList(servers) {
  if (!serverListEl || !serverListSection) return;
  const list = servers || [];
  serverListSection.hidden = list.length === 0;
  serverListEl.textContent = '';
  for (const srv of list) {
    const li = document.createElement('li');
    li.className = 'server-item' + (srv.active ? ' is-active' : '');

    const meta = document.createElement('div');
    meta.className = 'server-item-meta';

    const nameRow = document.createElement('div');
    nameRow.className = 'server-item-name';
    const dot = document.createElement('span');
    dot.className = 'server-status-dot' + (srv.connected ? ' is-connected' : srv.error ? ' is-error' : '');
    dot.title = srv.error || (srv.connected ? t('status.stream_connected') : t('status.stream_disconnected'));
    const name = document.createElement('strong');
    name.textContent = srv.label || srv.serverUrl || srv.id;
    nameRow.append(dot, name);
    if (srv.active) {
      const badge = document.createElement('span');
      badge.className = 'server-active-badge';
      badge.textContent = t('setup.active_badge');
      nameRow.append(badge);
    }

    const url = document.createElement('div');
    url.className = 'server-item-url';
    url.textContent = srv.serverUrl || '';

    meta.append(nameRow, url);

    const actions = document.createElement('div');
    actions.className = 'server-item-actions';

    if (!srv.active) {
      const useBtn = document.createElement('button');
      useBtn.type = 'button';
      useBtn.textContent = t('setup.set_active');
      useBtn.addEventListener('click', () => {
        SetActiveServer(srv.id).catch((err) => showError(String(err)));
      });
      actions.append(useBtn);
    }

    const renameBtn = document.createElement('button');
    renameBtn.type = 'button';
    renameBtn.className = 'btn-secondary';
    renameBtn.textContent = t('setup.rename');
    renameBtn.addEventListener('click', async () => {
      const next = window.prompt(t('setup.rename'), srv.label || '');
      if (next == null) return;
      try {
        await RenameServer(srv.id, next.trim());
      } catch (err) {
        showError(String(err));
      }
    });
    actions.append(renameBtn);

    const removeBtn = document.createElement('button');
    removeBtn.type = 'button';
    removeBtn.className = 'btn-danger';
    removeBtn.textContent = t('setup.remove');
    removeBtn.addEventListener('click', async () => {
      try {
        await RemoveServer(srv.id);
      } catch (err) {
        showError(String(err));
      }
    });
    actions.append(removeBtn);

    li.append(meta, actions);
    serverListEl.append(li);
  }
}

function showSetup(status) {
  currentView = 'setup';
  setupView.hidden = false;
  gotifyView.hidden = true;
  historyView.hidden = true;
  sendView.hidden = true;
  if (status?.serverUrl && !status?.configured) {
    serverInput.value = status.serverUrl;
  }
  if (status?.username && usernameInput && !status?.configured) {
    usernameInput.value = status.username;
  }
  renderServerList(status?.servers || []);
  updateConnectButtonLabel(Boolean(status?.configured));
  frame.src = 'about:blank';
  activeGotifyServerId = null;
  updateStatusLine(status);
}

async function showGotify(payload, {forceReload = false} = {}) {
  let token = payload?.token;
  let uiPath = payload?.uiPath;
  const serverId = payload?.serverId || null;
  if (!token) {
    token = await GetClientToken();
  }
  if (!uiPath) {
    uiPath = await GetUIPath();
  }
  applySession(token);

  if (serverId && serverId !== activeGotifyServerId) {
    forceReload = true;
  }
  if (serverId) {
    activeGotifyServerId = serverId;
  }

  currentView = 'gotify';
  setupView.hidden = true;
  gotifyView.hidden = false;
  historyView.hidden = true;
  sendView.hidden = true;

  // Do not reload the iframe on every stream status tick — that wiped the Gotify session.
  // Also skip when returning from history: the iframe is still loaded (showSetup blanks it).
  if (!forceReload && frame.src && !frame.src.endsWith('about:blank')) {
    return;
  }
  frame.src = `${uiPath}${uiPath.includes('?') ? '&' : '?'}_=${Date.now()}`;
}

function updateStatusLine(status) {
  if (!status) {
    statusLine.textContent = '';
    return;
  }
  const parts = [];
  if (status.configured) {
    parts.push(status.connected ? t('status.stream_connected') : t('status.stream_disconnected'));
  } else {
    parts.push(t('status.not_configured'));
  }
  if (status.notifyReady === false) {
    parts.push(t('status.notify_fallback'));
  }
  if (status.notifyError) {
    parts.push(t('status.notify_error', {Error: status.notifyError}));
  }
  if (status.error) {
    parts.push(status.error);
  }
  statusLine.textContent = parts.join(' · ');
}

async function applyStatus(status) {
  if (!status) return;
  if (currentView === 'setup') {
    renderServerList(status.servers || []);
    updateConnectButtonLabel(Boolean(status.configured));
  }
  // History/send are left only via Back (show-gotify / show-setup events), never by a status tick.
  if (currentView === 'history' || currentView === 'send') {
    updateStatusLine(status);
    return;
  }
  if (status.view === 'history') {
    updateStatusLine(status);
    await showHistory();
    return;
  }
  if (status.view === 'send') {
    updateStatusLine(status);
    await showSend();
    return;
  }
  // Once we know a saved session exists, never fall back to the Connect form
  // from a late/racy status tick — Settings is the only way back.
  if (status.view === 'gotify' && status.configured) {
    bootstrapped = true;
    await showGotify({
      serverId: status.activeServerId,
      serverUrl: status.serverUrl,
    }, {forceReload: false});
  } else if (status.configured && bootstrapped && currentView === 'gotify') {
    updateStatusLine(status);
    return;
  } else if (!status.configured || status.view === 'setup') {
    if (currentView !== 'setup') {
      showSetup(status);
    } else {
      renderServerList(status.servers || []);
      updateConnectButtonLabel(Boolean(status.configured));
      updateStatusLine(status);
      return;
    }
  } else {
    updateStatusLine(status);
    return;
  }
  updateStatusLine(status);
}

let historyToastTimer = null;

// Prefer the web clipboard; fall back to the Wails runtime when the webview blocks it.
async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    return;
  } catch (err) {
    console.warn('navigator.clipboard failed, falling back to Wails Clipboard.SetText', err);
  }
  await Clipboard.SetText(text);
}

function flashHistoryToast(msg) {
  historyToast.textContent = msg;
  historyToast.hidden = false;
  clearTimeout(historyToastTimer);
  historyToastTimer = setTimeout(() => {
    historyToast.hidden = true;
  }, 1500);
}

function copyIcon() {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('aria-hidden', 'true');
  svg.setAttribute('fill', 'none');
  svg.setAttribute('stroke', 'currentColor');
  svg.setAttribute('stroke-width', '2');
  svg.setAttribute('stroke-linecap', 'round');
  svg.setAttribute('stroke-linejoin', 'round');

  const back = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  back.setAttribute('d', 'M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2');

  const front = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
  front.setAttribute('width', '14');
  front.setAttribute('height', '14');
  front.setAttribute('x', '8');
  front.setAttribute('y', '8');
  front.setAttribute('rx', '2');

  svg.append(back, front);
  return svg;
}

function renderHistory(entries) {
  historyList.textContent = '';
  const list = entries || [];
  historyEmpty.hidden = list.length > 0;
  for (const entry of list) {
    const li = document.createElement('li');
    li.className = 'history-item';

    const main = document.createElement('div');
    main.className = 'history-item-main';

    const time = document.createElement('div');
    time.className = 'history-time';
    const when = new Date(entry.received_at);
    time.textContent = Number.isNaN(when.getTime()) ? '' : when.toLocaleString();

    const label = document.createElement('span');
    label.className = 'history-server-badge';
    label.textContent = entry.server_label || t('history.unknown_server');

    const title = document.createElement('p');
    title.className = 'history-title';
    title.textContent = entry.title || '';

    const body = document.createElement('p');
    body.className = 'history-body';
    body.textContent = entry.body || '';

    const meta = document.createElement('div');
    meta.className = 'history-meta';
    meta.append(time, label);

    main.append(meta, title, body);

    const copyBtn = document.createElement('button');
    copyBtn.type = 'button';
    copyBtn.className = 'history-copy';
    const copyLabel = t('history.copy');
    copyBtn.setAttribute('aria-label', copyLabel);
    copyBtn.title = copyLabel;
    copyBtn.append(copyIcon());
    copyBtn.addEventListener('click', async () => {
      try {
        const md = await FormatHistoryEntryMarkdown(entry.id);
        await copyText(md);
        flashHistoryToast(t('history.copied'));
      } catch (err) {
        console.error(err);
        flashHistoryToast(t('history.copy_failed'));
      }
    });

    li.append(main, copyBtn);
    historyList.append(li);
  }
}

async function refreshHistory() {
  renderHistory(await GetNotificationHistory());
}

/** Park the Gotify iframe so its SPA timers cannot steal shell input focus. */
function parkGotifyFrame() {
  if (!frame) return;
  try {
    if (frame.src && !frame.src.endsWith('about:blank')) {
      frame.src = 'about:blank';
    }
  } catch (_) {
    /* ignore */
  }
}

async function showHistory() {
  currentView = 'history';
  setupView.hidden = true;
  gotifyView.hidden = true;
  historyView.hidden = false;
  sendView.hidden = true;
  parkGotifyFrame();
  try {
    historyModeAll.checked = (await GetHistoryMode()) === 'all';
    await refreshHistory();
  } catch (err) {
    console.error(err);
  }
}

let sendToastTimer = null;

function showSendError(msg) {
  if (!msg) {
    sendError.hidden = true;
    sendError.textContent = '';
    return;
  }
  sendError.hidden = false;
  sendError.textContent = msg;
}

function flashSendToast(msg) {
  sendToast.textContent = msg;
  sendToast.hidden = false;
  clearTimeout(sendToastTimer);
  sendToastTimer = setTimeout(() => {
    sendToast.hidden = true;
  }, 1500);
}

function clampPriority(n) {
  const v = Number(n);
  if (!Number.isFinite(v)) return 0;
  return Math.min(10, Math.max(0, Math.round(v)));
}

function applyDefaultPriorityForSelectedApp() {
  const id = Number(sendApp.value);
  const app = sendAppsCache.find((a) => Number(a.id) === id);
  const prio = clampPriority(app?.defaultPriority ?? 0);
  sendPriority.value = String(prio);
}

function fillSendApps(apps) {
  sendAppsCache = apps || [];
  const prev = sendApp.value;
  sendApp.textContent = '';
  for (const app of sendAppsCache) {
    const opt = document.createElement('option');
    opt.value = String(app.id);
    opt.textContent = app.name || `#${app.id}`;
    sendApp.append(opt);
  }
  if (prev && [...sendApp.options].some((o) => o.value === prev)) {
    sendApp.value = prev;
  }
  applyDefaultPriorityForSelectedApp();
  const empty = sendAppsCache.length === 0;
  sendAppsEmpty.hidden = !empty;
  sendApp.disabled = empty;
  sendSubmit.disabled = empty;
}

function fillSendServers(servers, activeId) {
  serversCache = servers || [];
  if (!sendServer) return;
  const prev = sendServer.value;
  sendServer.textContent = '';
  for (const srv of serversCache) {
    const opt = document.createElement('option');
    opt.value = srv.id;
    opt.textContent = srv.label || srv.serverUrl || srv.id;
    sendServer.append(opt);
  }
  const prefer = prev || activeId || '';
  if (prefer && [...sendServer.options].some((o) => o.value === prefer)) {
    sendServer.value = prefer;
  } else if (sendServer.options.length > 0) {
    sendServer.selectedIndex = 0;
  }
  sendServer.disabled = serversCache.length === 0;
}

async function loadSendApplications() {
  sendNotConfigured.hidden = true;
  sendAppsEmpty.hidden = true;
  showSendError('');
  const serverID = sendServer?.value || '';
  if (!serverID) {
    fillSendApps([]);
    sendNotConfigured.hidden = false;
    return;
  }
  try {
    const apps = await ListApplicationsForServer(serverID);
    fillSendApps(apps);
  } catch (err) {
    const msg = String(err || '');
    fillSendApps([]);
    if (/not configured/i.test(msg)) {
      sendNotConfigured.hidden = false;
    } else {
      showSendError(t('send.load_failed') + (msg ? `: ${msg}` : ''));
    }
  }
}

async function showSend() {
  currentView = 'send';
  setupView.hidden = true;
  gotifyView.hidden = true;
  historyView.hidden = true;
  sendView.hidden = false;
  parkGotifyFrame();
  try {
    const status = await GetStatus();
    fillSendServers(status?.servers || [], status?.activeServerId || '');
  } catch (err) {
    console.error(err);
    fillSendServers([], '');
  }
  await loadSendApplications();
}

historyModeAll.addEventListener('change', async () => {
  try {
    await SetHistoryMode(historyModeAll.checked ? 'all' : 'toasts');
    await refreshHistory();
  } catch (err) {
    console.error(err);
  }
});
document.getElementById('history-clear').addEventListener('click', () => {
  ClearNotificationHistory().then(refreshHistory).catch(console.error);
});
document.getElementById('history-back').addEventListener('click', () => {
  LeaveNotificationHistory().catch(console.error);
});

sendApp.addEventListener('change', () => {
  applyDefaultPriorityForSelectedApp();
});

sendServer?.addEventListener('change', () => {
  loadSendApplications().catch(console.error);
});

document.getElementById('send-back').addEventListener('click', () => {
  LeaveSendMessage().catch(console.error);
});

sendForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  showSendError('');
  const serverID = sendServer?.value || '';
  const appID = Number(sendApp.value);
  const title = sendTitle.value.trim();
  const message = sendBody.value.trim();
  const priority = clampPriority(sendPriority.value);
  if (!serverID) {
    showSendError(t('send.not_configured'));
    return;
  }
  if (!appID) {
    showSendError(t('send.apps_empty'));
    return;
  }
  if (!message) {
    showSendError(t('send.message'));
    return;
  }
  sendSubmit.disabled = true;
  sendSubmit.textContent = t('send.sending');
  try {
    await SendApplicationMessageToServer(serverID, appID, title, message, priority);
    sendBody.value = '';
    flashSendToast(t('send.sent'));
  } catch (err) {
    showSendError(String(err));
  } finally {
    sendSubmit.disabled = sendAppsCache.length === 0;
    sendSubmit.textContent = t('send.submit');
  }
});

function postToGotify(msg) {
  try {
    frame?.contentWindow?.postMessage(msg, '*');
  } catch (_) {
    /* ignore */
  }
}

form.addEventListener('submit', async (event) => {
  event.preventDefault();
  showError('');
  connectBtn.disabled = true;
  connectBtn.textContent = t('setup.connecting');
  const serverUrl = serverInput.value.trim();
  const username = usernameInput.value;
  const password = document.getElementById('password').value;
  const label = serverLabelInput?.value?.trim() || '';
  let wasConfigured = false;
  try {
    const before = await GetStatus();
    wasConfigured = Boolean(before?.configured);
    await AddServer(serverUrl, username, password, label);
    document.getElementById('password').value = '';
    if (serverLabelInput) serverLabelInput.value = '';
    const status = await GetStatus();
    if (!wasConfigured) {
      bootstrapped = true;
      await showGotify({
        serverId: status?.activeServerId,
        serverUrl: status?.serverUrl,
      }, {forceReload: true});
    } else {
      showSetup(status);
    }
  } catch (err) {
    showError(String(err));
  } finally {
    connectBtn.disabled = false;
    updateConnectButtonLabel(wasConfigured || currentView === 'setup');
    GetStatus().then((s) => updateConnectButtonLabel(Boolean(s?.configured))).catch(() => {});
  }
});

Events.On('status', (event) => {
  applyStatus(event?.data).catch(console.error);
});
Events.On('show-setup', () => {
  bootstrapped = false;
  GetStatus().then(showSetup).catch(console.error);
});
Events.On('show-history', () => {
  showHistory().catch(console.error);
});
Events.On('show-send', () => {
  showSend().catch(console.error);
});
Events.On('history-updated', () => {
  if (currentView === 'history') {
    refreshHistory().catch(console.error);
  }
});
Events.On('show-gotify', (event) => {
  bootstrapped = true;
  // showGotify force-reloads when serverId changes (active switch).
  showGotify(event?.data, {forceReload: false}).catch(console.error);
});
// Native OS toasts come from Go only. This event only refreshes the iframe list.
Events.On('gotify-message', () => {
  postToGotify({type: 'gotify-desktop-refresh'});
});
Events.On('locale-changed', (event) => {
  const payload = event?.data || {};
  let cat = {};
  try {
    cat = typeof payload?.catalog === 'string' ? JSON.parse(payload.catalog) : payload?.catalog || {};
  } catch (_) {
    cat = {};
  }
  loadCatalog({locale: payload?.locale || 'en', catalog: cat});
  // Refresh dynamic status line in the new language.
  GetStatus().then(updateStatusLine).catch(console.error);
  if (currentView === 'history') {
    refreshHistory().catch(console.error);
  }
  if (currentView === 'send' && sendSubmit && !sendSubmit.disabled) {
    sendSubmit.textContent = t('send.submit');
  }
});

async function initI18n() {
  try {
    const [loc, raw] = await Promise.all([GetLocale(), GetI18nCatalog()]);
    let cat = {};
    try {
      cat = JSON.parse(raw || '{}');
    } catch (_) {
      cat = {};
    }
    loadCatalog({locale: loc, catalog: cat});
  } catch (err) {
    console.warn('i18n init failed, keeping English markup', err);
    applyDom();
  }
}

async function bootstrap() {
  await initI18n();
  // Hide the Connect form until we know whether a session already exists.
  setupView.hidden = true;
  for (let i = 0; i < 8; i++) {
    try {
      const status = await GetStatus();
      if (status?.configured && status.view === 'gotify') {
        bootstrapped = true;
        await showGotify(null, {forceReload: true});
        updateStatusLine(status);
        return;
      }
      if (status && !status.configured) {
        showSetup(status);
        return;
      }
      // Startup still loading config — wait briefly.
      await new Promise((r) => setTimeout(r, 100));
    } catch (err) {
      console.error(err);
    }
  }
  try {
    const status = await GetStatus();
    await applyStatus(status);
  } catch (err) {
    console.error(err);
    showSetup(null);
  }
}

let saveGeoTimer = null;
let saveGeoPollId = null;

function scheduleSaveWindowState() {
  clearTimeout(saveGeoTimer);
  saveGeoTimer = setTimeout(() => {
    SaveWindowState().catch((err) => console.warn('SaveWindowState', err));
  }, 400);
}

function stopWindowGeometryPolling() {
  if (saveGeoPollId != null) {
    clearInterval(saveGeoPollId);
    saveGeoPollId = null;
  }
  clearTimeout(saveGeoTimer);
  saveGeoTimer = null;
}

function shellInputFocused() {
  const ae = document.activeElement;
  if (!ae || ae === document.body) return false;
  const tag = ae.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT';
}

function startWindowGeometryPolling() {
  if (saveGeoPollId != null) {
    return;
  }
  // Wails has no window-moved DOM event: poll while visible. Go no-ops if unchanged.
  // Skip while typing — GTK WindowGet* via Wails can steal WebView focus on Linux.
  saveGeoPollId = setInterval(() => {
    if (document.visibilityState === 'visible' && !shellInputFocused()) {
      SaveWindowState().catch(() => {});
    }
  }, 2000);
}

window.addEventListener('resize', scheduleSaveWindowState);
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'hidden') {
    clearTimeout(saveGeoTimer);
    saveGeoTimer = null;
    stopWindowGeometryPolling();
    SaveWindowState().catch(() => {});
    return;
  }
  startWindowGeometryPolling();
});
window.addEventListener('pagehide', stopWindowGeometryPolling);
window.addEventListener('beforeunload', stopWindowGeometryPolling);

startWindowGeometryPolling();
bootstrap().catch(console.error);
