import './style.css';
import {Connect, GetStatus, GetClientToken, GetUIPath} from '../wailsjs/go/main/App';
import {EventsOn} from '../wailsjs/runtime/runtime';

const setupView = document.getElementById('setup-view');
const gotifyView = document.getElementById('gotify-view');
const frame = document.getElementById('gotify-frame');
const form = document.getElementById('setup-form');
const errorEl = document.getElementById('error');
const statusLine = document.getElementById('status-line');
const connectBtn = document.getElementById('connect-btn');
const serverInput = document.getElementById('server-url');

/** @type {'setup'|'gotify'|null} */
let currentView = null;

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
    localStorage.setItem('gotify-login-key', token);
  } catch (_) {
    /* ignore */
  }
  try {
    document.cookie = `gotify-client-token=${encodeURIComponent(token)}; path=/; SameSite=Lax`;
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
  if (status.view === 'gotify' && status.configured) {
    // Only mount Gotify once; later status events only refresh the status line.
    await showGotify(null, {forceReload: false});
  } else if (currentView !== 'setup') {
    showSetup(status);
  } else {
    updateStatusLine(status);
    return;
  }
  updateStatusLine(status);
}

form.addEventListener('submit', async (event) => {
  event.preventDefault();
  showError('');
  connectBtn.disabled = true;
  connectBtn.textContent = 'Connecting…';
  const serverUrl = serverInput.value.trim();
  const username = document.getElementById('username').value;
  const password = document.getElementById('password').value;
  try {
    await Connect(serverUrl, username, password);
    document.getElementById('password').value = '';
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
  GetStatus().then(showSetup).catch(console.error);
});
EventsOn('show-gotify', (payload) => {
  showGotify(payload, {forceReload: true}).catch(console.error);
});

GetStatus()
  .then((status) => applyStatus(status))
  .catch((err) => {
    console.error(err);
    showSetup(null);
  });
