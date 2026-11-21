// Infra Scanner static demo.
// This page has no collector behind it. The console (app.js) runs unchanged, and this script answers
// its API calls from the data embedded in the page (<script id="demo-data">), which cmd/demo-site
// produced by feeding the fictional servers of internal/demo to the real collector code.
(() => {
  'use strict';

  const data = JSON.parse(document.getElementById('demo-data').textContent);

  const TEXT = {
    ko: {
      note: '데모 화면입니다. 서버와 주소는 모두 가상의 값이고, 바꾼 내용은 저장되지 않습니다.',
      source: '소스 보기',
      close: '닫기',
      readonly: '데모에서는 서버를 삭제할 수 없습니다',
    },
    en: {
      note: 'This is a demo. All servers and addresses are fictional, and changes are not saved.',
      source: 'View source',
      close: 'Close',
      readonly: 'Servers cannot be removed in the demo',
    },
    zh: {
      note: '这是演示页面。所有服务器和地址均为虚构，所做更改不会保存。',
      source: '查看源码',
      close: '关闭',
      readonly: '演示中无法删除服务器',
    },
  };

  // The console turns an error code into a sentence with the key "err.<code>".
  const dict = window.INFRA_I18N || {};
  for (const [lang, text] of Object.entries(TEXT)) {
    if (dict[lang]) dict[lang]['err.demo_readonly'] = text.readonly;
  }

  // ---- API -----------------------------------------------------------------

  const WINDOW_SEC = { '1h': 3600, '6h': 21600, '24h': 86400, '7d': 604800, '30d': 2592000 };
  const ISO_TIME = /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?Z$/;
  const generatedAt = Date.parse(data.topology.generated_at);
  let layout = {}; // node positions moved by the visitor; kept only until the page is reloaded

  // The embedded data carries the fixed timestamps of the moment it was generated.
  // Shift every timestamp by the same amount so that the topology reads as generated just now.
  function live(value) {
    const delta = Date.now() - generatedAt;
    return JSON.parse(JSON.stringify(value), (key, v) =>
      (typeof v === 'string' && ISO_TIME.test(v) ? new Date(Date.parse(v) + delta).toISOString() : v));
  }

  const json = (body, status = 200) =>
    new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json; charset=utf-8' } });
  const noContent = () => new Response(null, { status: 204 });

  function handle(method, path, query, body) {
    const windowSec = WINDOW_SEC[query.get('window')] || WINDOW_SEC['24h'];
    if (path === 'me' || path === 'login' || path === 'logout') return json({ ok: true });
    if (path === 'install-info') return json(data.install);
    if (path === 'topology') {
      // Every flow of the demo is a few minutes old, so all periods show the same topology.
      return json({ ...live(data.topology), window_sec: windowSec, layout: { ...layout } });
    }
    if (path === 'layout') {
      if (method === 'PUT') Object.assign(layout, body || {});
      else if (method === 'DELETE') layout = {};
      return noContent();
    }
    const m = /^hosts\/([^/]+)$/.exec(path);
    if (m) {
      const detail = data.hosts[decodeURIComponent(m[1])];
      if (!detail) return json({ code: 'host_not_found' }, 404);
      if (method === 'DELETE') return json({ code: 'demo_readonly' }, 403);
      return json({ ...live(detail), window_sec: windowSec });
    }
    return json({ code: 'bad_request' }, 404);
  }

  const realFetch = window.fetch ? window.fetch.bind(window) : null;
  window.fetch = async (input, init = {}) => {
    const url = new URL(typeof input === 'string' ? input : input.url, location.href);
    const m = /\/api\/v1\/(.+)$/.exec(url.pathname);
    if (!m) {
      if (!realFetch) throw new TypeError('fetch is not available');
      return realFetch(input, init);
    }
    let body;
    try { body = init.body ? JSON.parse(init.body) : undefined; } catch { body = undefined; }
    return handle((init.method || 'GET').toUpperCase(), m[1], url.searchParams, body);
  };

  // ---- Notice --------------------------------------------------------------

  const style = document.createElement('style');
  style.textContent = `
    #btn-logout { display: none; } /* there is nothing to sign out of */
    .demo-note {
      position: absolute; left: 50%; top: 12px; transform: translateX(-50%); z-index: 45;
      display: flex; align-items: center; gap: 10px; max-width: calc(100% - 24px); padding: 6px 6px 6px 8px;
      background: rgba(13, 19, 34, .96); border: 1px solid var(--s700); border-radius: 9999px;
      box-shadow: 0 12px 28px -10px rgba(0, 0, 0, .8); font-size: 12px; color: var(--s300); cursor: default;
    }
    .demo-note b {
      flex: none; padding: 2px 8px; border-radius: 9999px; background: var(--amber); color: var(--s950);
      font-family: var(--mono); font-size: 10px; letter-spacing: .08em;
    }
    .demo-note span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .demo-note a { flex: none; color: var(--sky); text-decoration: none; white-space: nowrap; }
    .demo-note a:hover { text-decoration: underline; }
    .demo-note button {
      flex: none; width: 22px; height: 22px; padding: 0; border: 0; border-radius: 50%;
      background: transparent; color: var(--s400); font-size: 15px; line-height: 1; cursor: pointer;
    }
    .demo-note button:hover { background: var(--s800); color: #fff; }
  `;
  document.head.appendChild(style);

  // On GitHub Pages (https://<owner>.github.io/<repository>/) the address itself tells where the source is.
  const sourceURL = (() => {
    const m = /^([a-z0-9-]+)\.github\.io$/i.exec(location.hostname);
    if (!m) return '';
    const first = location.pathname.split('/').filter(Boolean)[0] || '';
    return `https://github.com/${m[1]}${first && !first.includes('.') ? '/' + first : ''}`;
  })();

  let note = null;
  let closed = false;

  function renderNote() {
    if (closed) return;
    const viewport = document.getElementById('viewport');
    if (!viewport) return;
    const text = TEXT[(document.documentElement.lang || 'en').slice(0, 2)] || TEXT.en;
    if (!note) {
      note = document.createElement('div');
      note.className = 'demo-note';
      note.setAttribute('role', 'note');
      // Keep clicks on the notice from panning or deselecting in the topology underneath.
      for (const type of ['pointerdown', 'wheel']) note.addEventListener(type, e => e.stopPropagation());
      viewport.appendChild(note);
    }
    note.textContent = '';
    const badge = document.createElement('b');
    badge.textContent = 'DEMO';
    const message = document.createElement('span');
    message.textContent = text.note;
    message.title = text.note;
    note.append(badge, message);
    if (sourceURL) {
      const link = document.createElement('a');
      link.href = sourceURL;
      link.target = '_blank';
      link.rel = 'noopener';
      link.textContent = text.source;
      note.append(link);
    }
    const close = document.createElement('button');
    close.type = 'button';
    close.title = text.close;
    close.setAttribute('aria-label', text.close);
    close.textContent = '×';
    close.addEventListener('click', () => {
      closed = true;
      note.remove();
    });
    note.append(close);
  }

  // The console sets <html lang> when it starts and whenever the visitor switches language.
  new MutationObserver(renderNote).observe(document.documentElement, { attributes: true, attributeFilter: ['lang'] });
  document.addEventListener('DOMContentLoaded', renderNote);
})();
