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
} from '../wailsjs/go/main/App';
import {EventsOn} from '../wailsjs/runtime/runtime';
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
const historyList = document.getElementById('history-list');
const historyEmpty = document.getElementById('history-empty');
const historyToast = document.getElementById('history-toast');
const historyModeAll = document.getElementById('history-mode-all');
const frame = document.getElementById('gotify-frame');
const form = document.getElementById('setup-form');
const errorEl = document.getElementById('error');
const statusLine = document.getElementById('status-line');
const connectBtn = document.getElementById('connect-btn');
const serverInput = document.getElementById('server-url');
const usernameInput = document.getElementById('username');

/** @type {'setup'|'gotify'|'history'|null} */
let currentView = null;
let bootstrapped = false;

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

  const alreadyShowing = currentView === 'gotify' && !gotifyView.hidden;
  currentView = 'gotify';
  setupView.hidden = true;
  gotifyView.hidden = false;
  historyView.hidden = true;

  // Do not reload the iframe on every stream status tick — that wiped the Gotify session.
  if (!forceReload && alreadyShowing && frame.src && !frame.src.endsWith('about:blank')) {
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
  // History is left only via Back (show-gotify / show-setup events), never by a status tick.
  if (currentView === 'history') {
    updateStatusLine(status);
    return;
  }
  if (status.view === 'history') {
    updateStatusLine(status);
    await showHistory();
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
        await navigator.clipboard.writeText(md);
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

async function showHistory() {
  currentView = 'history';
  setupView.hidden = true;
  gotifyView.hidden = true;
  historyView.hidden = false;
  try {
    historyModeAll.checked = (await GetHistoryMode()) === 'all';
    await refreshHistory();
  } catch (err) {
    console.error(err);
  }
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
EventsOn('history-updated', () => {
  if (currentView === 'history') {
    refreshHistory().catch(console.error);
  }
});
EventsOn('show-gotify', (payload) => {
  bootstrapped = true;
  showGotify(payload, {forceReload: true}).catch(console.error);
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

bootstrap().catch(console.error);
