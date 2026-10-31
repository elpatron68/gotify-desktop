import './style.css';
import {Connect, GetStatus, GetClientToken, GetUIPath} from '../wailsjs/go/main/App';
import {EventsOn} from '../wailsjs/runtime/runtime';

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
const frame = document.getElementById('gotify-frame');
const form = document.getElementById('setup-form');
const errorEl = document.getElementById('error');
const statusLine = document.getElementById('status-line');
const connectBtn = document.getElementById('connect-btn');
const serverInput = document.getElementById('server-url');
const usernameInput = document.getElementById('username');

/** @type {'setup'|'gotify'|null} */
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
    parts.push(status.connected ? 'Stream connected' : 'Stream disconnected');
  } else {
    parts.push('Not configured');
  }
  if (status.notifyReady === false) {
    parts.push('Notify: fallback');
  }
  if (status.notifyError) {
    parts.push('Notify error: ' + status.notifyError);
  }
  if (status.error) {
    parts.push(status.error);
  }
  statusLine.textContent = parts.join(' · ');
}

async function applyStatus(status) {
  if (!status) return;
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
  connectBtn.textContent = 'Connecting…';
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
    connectBtn.textContent = 'Connect';
  }
});

EventsOn('status', (status) => {
  applyStatus(status).catch(console.error);
});
EventsOn('show-setup', () => {
  bootstrapped = false;
  GetStatus().then(showSetup).catch(console.error);
});
EventsOn('show-gotify', (payload) => {
  bootstrapped = true;
  showGotify(payload, {forceReload: true}).catch(console.error);
});
// Native OS toasts come from Go only. This event only refreshes the iframe list.
EventsOn('gotify-message', () => {
  postToGotify({type: 'gotify-desktop-refresh'});
});

async function bootstrap() {
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
