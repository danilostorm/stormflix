const $ = (selector) => document.querySelector(selector);
const api = '/api/v1';

async function request(url, options = {}) {
  const response = await fetch(url, {
    ...options,
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
  return data;
}

function message(text, error = false) {
  const el = $('#message');
  el.textContent = text;
  el.className = `message ${error ? 'error' : 'success'}`;
  if (text) setTimeout(() => { el.textContent = ''; el.className = 'message'; }, 5000);
}

async function loadLibraries() {
  const libraries = await request(`${api}/libraries`);
  const root = $('#libraries');
  root.innerHTML = '';
  if (!libraries.length) {
    root.innerHTML = '<div class="empty">Nenhuma biblioteca cadastrada ainda.</div>';
    return;
  }

  for (const lib of libraries) {
    const card = document.createElement('div');
    card.className = 'library-card';
    card.innerHTML = `
      <div><strong>${escapeHTML(lib.name)}</strong><small>${escapeHTML(lib.kind)} · ${escapeHTML(lib.path)}</small></div>
      <button data-scan="${lib.id}">Escanear</button>`;
    root.appendChild(card);
  }

  root.querySelectorAll('[data-scan]').forEach((button) => {
    button.addEventListener('click', async () => {
      button.disabled = true;
      button.textContent = 'Escaneando...';
      try {
        const result = await request(`${api}/libraries/${button.dataset.scan}/scan`, { method: 'POST' });
        message(`${result.files} arquivos encontrados.`);
        await loadMedia();
      } catch (err) {
        message(err.message, true);
      } finally {
        button.disabled = false;
        button.textContent = 'Escanear';
      }
    });
  });
}

async function loadMedia() {
  const q = $('#search').value.trim();
  const items = await request(`${api}/media?limit=200&q=${encodeURIComponent(q)}`);
  const root = $('#media');
  root.innerHTML = '';
  if (!items.length) {
    root.innerHTML = '<div class="empty">Nenhuma mídia encontrada. Escaneie uma biblioteca.</div>';
    return;
  }

  for (const item of items) {
    const fragment = $('#media-template').content.cloneNode(true);
    fragment.querySelector('.media-title').textContent = item.title;
    fragment.querySelector('.media-meta').textContent = `${item.extension.replace('.', '').toUpperCase()} · ${formatBytes(item.size_bytes)}`;
    fragment.querySelector('.play').href = `${api}/media/${item.id}/stream`;
    root.appendChild(fragment);
  }
}

$('#library-form').addEventListener('submit', async (event) => {
  event.preventDefault();
  try {
    await request(`${api}/libraries`, {
      method: 'POST',
      body: JSON.stringify({ name: $('#name').value, kind: $('#kind').value, path: $('#path').value }),
    });
    event.target.reset();
    message('Biblioteca adicionada. Agora clique em Escanear.');
    await loadLibraries();
  } catch (err) {
    message(err.message, true);
  }
});

$('#reload').addEventListener('click', async () => {
  await Promise.all([loadLibraries(), loadMedia()]);
});

let searchTimer;
$('#search').addEventListener('input', () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(loadMedia, 250);
});

function formatBytes(bytes) {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 2 ? 2 : 1)} ${units[i]}`;
}

function escapeHTML(value) {
  return String(value).replace(/[&<>'"]/g, (char) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;'
  })[char]);
}

Promise.all([loadLibraries(), loadMedia()]).catch((err) => message(err.message, true));
