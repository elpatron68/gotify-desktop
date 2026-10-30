import './style.css';
import {Connect, GetStatus, GetServerURL} from '../wailsjs/go/main/App';
import {EventsOn} from '../wailsjs/runtime/runtime';

const setupView = document.getElementById('setup-view');
const gotifyView = document.getElementById('gotify-view');
const frame = document.getElementById('gotify-frame');
const form = document.getElementById('setup-form');
const errorEl = document.getElementById('error');
const statusLine = document.getElementById('status-line');
const connectBtn = document.getElementById('connect-btn');
const serverInput = document.getElementById('server-url');

function showError(msg) {
  if (!msg) {
    errorEl.hidden = true;
    errorEl.textContent = '';
    return;
  }
  errorEl.hidden = false;
  errorEl.textContent = msg;
}

function showSetup(status) {
  setupView.hidden = false;
  gotifyView.hidden = true;
  if (status?.serverUrl) {
    serverInput.value = status.serverUrl;
  }
  updateStatusLine(status);
}

function showGotify(url) {
  if (url) {
    frame.src = url;
  }
  setupView.hidden = true;
  gotifyView.hidden = false;
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
  if (status.error) {
    parts.push(status.error);
  }
  statusLine.textContent = parts.join(' · ');
}

function applyStatus(status) {
  if (!status) return;
  if (status.view === 'gotify' && status.serverUrl) {
    showGotify(status.serverUrl);
  } else {
    showSetup(status);
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
    const url = await GetServerURL();
    showGotify(url);
  } catch (err) {
    showError(String(err));
  } finally {
    connectBtn.disabled = false;
    connectBtn.textContent = 'Connect';
  }
});

EventsOn('status', applyStatus);
EventsOn('show-setup', () => {
  GetStatus().then(showSetup).catch(console.error);
});
EventsOn('show-gotify', (url) => {
  showGotify(url);
});

GetStatus()
  .then(applyStatus)
  .catch((err) => {
    console.error(err);
    showSetup(null);
  });
