import './style.css';
import {
  Connect,
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
  ListApplications,
  SendApplicationMessage,
  LeaveSendMessage,
  SaveWindowState,
} from '../wailsjs/go/main/App';
import {ClipboardSetText, EventsOn} from '../wailsjs/runtime/runtime';
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

/** @type {'setup'|'gotify'|'history'|'send'|null} */
let currentView = null;
let bootstrapped = false;
/** @type {Array<{id:number,name:string,description?:string,defaultPriority?:number}>} */
let sendAppsCache = [];

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

function showSetup(status) {
  currentView = 'setup';
  setupView.hidden = false;
  gotifyView.hidden = true;
  historyView.hidden = true;
  sendView.hidden = true;
  if (status?.serverUrl) {
    serverInput.value = status.serverUrl;
  }
  if (status?.username && usernameInput) {
    usernameInput.value = status.username;
  }
  frame.src = 'about:blank';
  updateStatusLine(status);
}

async function showGotify(payload, {forceReload = false} = {}) {
  let token = payload?.token;
  let uiPath = payload?.uiPath;
  if (!token) {
    token = await GetClientToken();
  }
  if (!uiPath) {
    uiPath = await GetUIPath();
  }
  applySession(token);

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
    await showGotify(null, {forceReload: false});
  } else if (status.configured && bootstrapped && currentView === 'gotify') {
    updateStatusLine(status);
    return;
  } else if (!status.configured || status.view === 'setup') {
    if (currentView !== 'setup') {
      showSetup(status);
    } else {
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
    console.warn('navigator.clipboard failed, falling back to Wails ClipboardSetText', err);
  }
  const ok = await ClipboardSetText(text);
  if (ok === false) {
    throw new Error('ClipboardSetText returned false');
  }
}

function flashHistoryToast(msg) {
  historyToast.textContent = msg;
  historyToast.hidden = false;
  clearTimeout(historyToastTimer);
  historyToastTimer = setTimeout(() => {
    historyToast.hidden = true;
  }, 1500);
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

    const title = document.createElement('p');
    title.className = 'history-title';
    title.textContent = entry.title || '';

    const body = document.createElement('p');
    body.className = 'history-body';
    body.textContent = entry.body || '';

    main.append(time, title, body);

    const copyBtn = document.createElement('button');
    copyBtn.type = 'button';
    copyBtn.textContent = t('history.copy');
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

async function loadSendApplications() {
  sendNotConfigured.hidden = true;
  sendAppsEmpty.hidden = true;
  showSendError('');
  try {
    const apps = await ListApplications();
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

document.getElementById('send-back').addEventListener('click', () => {
  LeaveSendMessage().catch(console.error);
});

sendForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  showSendError('');
  const appID = Number(sendApp.value);
  const title = sendTitle.value.trim();
  const message = sendBody.value.trim();
  const priority = clampPriority(sendPriority.value);
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
    await SendApplicationMessage(appID, title, message, priority);
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
  try {
    await Connect(serverUrl, username, password);
    document.getElementById('password').value = '';
    bootstrapped = true;
    await showGotify(null, {forceReload: true});
  } catch (err) {
    showError(String(err));
  } finally {
    connectBtn.disabled = false;
    connectBtn.textContent = t('setup.connect');
  }
});

EventsOn('status', (status) => {
  applyStatus(status).catch(console.error);
});
EventsOn('show-setup', () => {
  bootstrapped = false;
  GetStatus().then(showSetup).catch(console.error);
});
EventsOn('show-history', () => {
  showHistory().catch(console.error);
});
EventsOn('show-send', () => {
  showSend().catch(console.error);
});
EventsOn('history-updated', () => {
  if (currentView === 'history') {
    refreshHistory().catch(console.error);
  }
});
EventsOn('show-gotify', (payload) => {
  bootstrapped = true;
  showGotify(payload, {forceReload: false}).catch(console.error);
});
// Native OS toasts come from Go only. This event only refreshes the iframe list.
EventsOn('gotify-message', () => {
  postToGotify({type: 'gotify-desktop-refresh'});
});
EventsOn('locale-changed', (payload) => {
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
