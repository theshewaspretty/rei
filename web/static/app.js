// Infra Scanner 관제 화면.
// 서버가 만든 구성도(/api/v1/topology)를 그리고, 사람이 옮긴 배치만 서버에 저장한다.
(() => {
  'use strict';

  // ---- 상수 ---------------------------------------------------------------

  const PALETTE = {
    sky:     { hex: '#38bdf8', glow: 'rgba(56, 189, 248, 0.32)',  badge: 'rgba(56, 189, 248, 0.14)' },
    emerald: { hex: '#34d399', glow: 'rgba(52, 211, 153, 0.32)',  badge: 'rgba(52, 211, 153, 0.14)' },
    amber:   { hex: '#fbbf24', glow: 'rgba(251, 191, 36, 0.32)',  badge: 'rgba(251, 191, 36, 0.14)' },
    purple:  { hex: '#a78bfa', glow: 'rgba(167, 139, 250, 0.32)', badge: 'rgba(167, 139, 250, 0.14)' },
    rose:    { hex: '#fb7185', glow: 'rgba(251, 113, 133, 0.32)', badge: 'rgba(251, 113, 133, 0.14)' },
    slate:   { hex: '#94a3b8', glow: 'rgba(148, 163, 184, 0.25)', badge: 'rgba(148, 163, 184, 0.14)' },
  };

  // 24x24, 선으로만 그리는 아이콘 (SVG path)
  const ICONS = {
    server: 'M4 3h16a2 2 0 0 1 2 2v4a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zM4 13h16a2 2 0 0 1 2 2v4a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2v-4a2 2 0 0 1 2-2zM6 7h.01M6 17h.01',
    database: 'M3 5c0-1.66 4.03-3 9-3s9 1.34 9 3-4.03 3-9 3-9-1.34-9-3zM3 5v14c0 1.66 4.03 3 9 3s9-1.34 9-3V5M3 12c0 1.66 4.03 3 9 3s9-1.34 9-3',
    layers: 'M12 2 2 7l10 5 10-5-10-5zM2 12l10 5 10-5M2 17l10 5 10-5',
    globe: 'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zM2 12h20M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z',
    help: 'M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zM9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3M12 17h.01',
    users: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75',
    search: 'M11 3a8 8 0 1 0 0 16 8 8 0 0 0 0-16zM21 21l-4.35-4.35',
    download: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3',
    image: 'M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zM9 7a2 2 0 1 0 0 4 2 2 0 0 0 0-4zM21 15l-3.09-3.09a2 2 0 0 0-2.82 0L6 21',
    fit: 'M8 3H5a2 2 0 0 0-2 2v3M21 8V5a2 2 0 0 0-2-2h-3M3 16v3a2 2 0 0 0 2 2h3M16 21h3a2 2 0 0 0 2-2v-3',
    plus: 'M5 12h14M12 5v14',
    minus: 'M5 12h14',
    refresh: 'M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8M21 3v5h-5M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16M8 16H3v5',
    logout: 'M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9',
    grid: 'M3 3h7v7H3zM14 3h7v7h-7zM14 14h7v7h-7zM3 14h7v7H3z',
    x: 'M18 6 6 18M6 6l12 12',
    network: 'M9 2h6v6H9zM2 16h6v6H2zM16 16h6v6h-6zM5 16v-3a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v3M12 12V8',
    trash: 'M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2',
    copy: 'M11 9h9a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2h-9a2 2 0 0 1-2-2v-9a2 2 0 0 1 2-2zM5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1',
    zone: 'M5 3a2 2 0 0 0-2 2M9 3h1M14 3h1M19 3a2 2 0 0 1 2 2M21 9v1M21 14v1M3 9v1M3 14v1M3 19a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2',
    arrow: 'M5 12h14M13 6l6 6-6 6',
  };
  // test.html 편집기(lucide)에서 쓰는 아이콘 이름
  const LUCIDE_NAME = { server: 'server', database: 'database', layers: 'layers', globe: 'globe', help: 'circle-help', users: 'users' };

  const STATUSES = ['online', 'offline', 'pending', 'unmanaged', 'external'];
  const SEVERITIES = ['high', 'medium', 'low'];
  const WINDOWS = ['1h', '6h', '24h', '7d', '30d'];
  const LEGEND = ['sky', 'purple', 'amber', 'rose', 'emerald', 'slate']; // 연결선 색 = 서비스 분류

  // ---- 언어 ---------------------------------------------------------------
  // 화면의 문장은 전부 i18n.js 에 있고, 여기서는 키로만 가리킨다. 새 언어는 i18n.js 에 사전을 더하고
  // LANGS 에 한 줄을 더하면 된다.

  const DICT = window.INFRA_I18N || {};
  const LANGS = [['ko', '한국어', 'ko'], ['en', 'English', 'en'], ['zh', '中文', 'zh-Hans']]; // [코드, 표시 이름, <html lang>]
  let lang = 'en';
  let langPinned = false; // 사용자가 직접 고른 언어인가 (그럴 때만 저장한다)
  const ZONE_TITLE_KEY = { 'z:pending': 'zone.pending', 'z:none': 'zone.none', 'z:unmanaged': 'zone.unmanaged' };

  // {name} 자리를 params 로 채운다. params.n 이 1 이면 "<key>_one"(영어 단수형)을 먼저 찾는다.
  // 선택한 언어에 키가 없으면 영어, 영어에도 없으면 키를 그대로 돌려준다.
  function t(key, params) {
    const d = DICT[lang] || {}, en = DICT.en || {};
    let s;
    if (params && params.n === 1) s = d[key + '_one'];
    if (s === undefined) s = d[key];
    if (s === undefined) s = en[key];
    if (s === undefined) return key;
    return params ? s.replace(/\{(\w+)\}/g, (m, k) => (params[k] !== undefined ? String(params[k]) : m)) : s;
  }
  const T = (key, params) => esc(t(key, params)); // HTML 안에 넣을 때
  const hasKey = key => (DICT[lang] || {})[key] !== undefined || (DICT.en || {})[key] !== undefined;

  // 브라우저의 선호 언어 중 지원하는 첫 번째. 없으면 영어.
  function detectLang() {
    for (const l of navigator.languages || [navigator.language || '']) {
      const code = String(l).toLowerCase().slice(0, 2);
      if (DICT[code]) return code;
    }
    return 'en';
  }

  const NODE_W = 240, NODE_H = 108;
  const GAP_X = 64, GAP_Y = 72, ZONE_PAD = 28, ZONE_HEAD = 44, ZONE_GAP = 80;
  const ZOOM_MIN = 0.15, ZOOM_MAX = 2;
  const REFRESH_MS = 30000;
  const MAX_ANIMATED_EDGES = 150; // 이보다 많으면 흐름 애니메이션을 끈다 (브라우저 부하)
  const MAX_LABELED_EDGES = 80;   // 이보다 많으면 선택한 것과 이어진 라벨만 보인다
  const PREF_KEY = 'infra-scanner-ui';
  const PREF_TOGGLES = ['showExternal', 'showUnmanaged', 'showInfra', 'showLabels', 'animate'];

  // ---- 상태 ---------------------------------------------------------------

  const state = {
    topo: null,
    saved: {},      // 서버에 저장된 배치 (node id -> {x, y})
    pos: {},        // 지금 화면의 좌표
    vis: { nodes: [], edges: [], zones: [], nodeById: new Map(), edgeById: new Map(), members: new Map() },
    view: { x: 40, y: 40, k: 1 },
    titles: new Map(), // node id -> 선택된 언어의 이름 (점검 문장에 쓴다)
    sel: null,      // { type: 'node' | 'edge', id }
    detail: null,   // 선택한 서버의 상세
    window: '24h',
    showExternal: true, showUnmanaged: true, showInfra: true, showLabels: true, animate: true,
    search: '',
    sevFilter: 'all',
    open: { listeners: true, outbound: true, inbound: true },
    fitted: false,
    installInfo: null,
    installForm: { server: '', zone: '' },
  };

  const $ = id => document.getElementById(id);
  const el = {};
  let drag = null;
  let frame = 0;
  let timer = 0;

  // ---- 도구 ---------------------------------------------------------------

  const ESC = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' };
  // 에이전트가 보낸 문자열(호스트명, 프로세스명 등)은 전부 이 함수를 거쳐야 한다.
  const esc = v => String(v ?? '').replace(/[&<>"']/g, c => ESC[c]);
  const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));
  const accent = name => PALETTE[name] || PALETTE.slate;
  const icon = name => `<svg class="ico" viewBox="0 0 24 24"><path d="${ICONS[name] || ICONS.server}"/></svg>`;
  const statusKey = s => (STATUSES.includes(s) ? s : 'external');
  const statusLabel = s => t('status.' + statusKey(s));
  const windowLabel = () => t('window.' + state.window);

  function fmtBytes(n) {
    if (!n) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
    const v = n / Math.pow(1024, i);
    return `${v >= 100 || i === 0 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`;
  }
  function fmtAgo(iso) {
    if (!iso) return '-';
    const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
    if (s < 60) return t('ago.now');
    if (s < 3600) return t('ago.min', { n: Math.floor(s / 60) });
    if (s < 172800) return t('ago.hour', { n: Math.floor(s / 3600) });
    return t('ago.day', { n: Math.floor(s / 86400) });
  }
  // 초 단위 길이를 가장 큰 단위 하나로 줄여 말한다 ("3시간", "3 h").
  function fmtDuration(sec) {
    sec = Math.max(0, Number(sec) || 0);
    if (sec < 60) return t('dur.sec', { n: Math.floor(sec) });
    if (sec < 3600) return t('dur.min', { n: Math.floor(sec / 60) });
    if (sec < 172800) return t('dur.hour', { n: Math.floor(sec / 3600) });
    return t('dur.day', { n: Math.floor(sec / 86400) });
  }
  function fmtTime(iso) {
    if (!iso) return '-';
    const d = new Date(iso);
    const p = n => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
  }
  function fmtUptime(sec) {
    if (!sec) return '-';
    const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
    return d ? t('uptime.dh', { d, h }) : h ? t('uptime.hm', { h, m }) : t('uptime.m', { m });
  }

  function toast(msg, isError) {
    const t = document.createElement('div');
    t.className = 'toast' + (isError ? ' error' : '');
    t.textContent = msg;
    el.toasts.appendChild(t);
    setTimeout(() => t.remove(), isError ? 5000 : 2600);
  }

  async function api(path, opts = {}) {
    const headers = { 'X-Requested-With': 'infra-scanner' };
    if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
    const res = await fetch(path, {
      method: opts.method || 'GET', headers, credentials: 'same-origin',
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    });
    if (res.status === 401 && !opts.login) {
      showLogin();
      throw new Error(t('err.login_required'));
    }
    if (res.status === 204) return null;
    const data = await res.json().catch(() => null);
    if (!res.ok) throw new Error(errorText(data, res.status));
    return data;
  }

  // 서버는 오류마다 언어와 무관한 code 를 준다. 아는 code 면 선택된 언어의 문장으로, 모르면 상태 코드만 알린다.
  function errorText(data, status) {
    const key = data && data.code ? 'err.' + data.code : '';
    if (key && hasKey(key)) return t(key, { wait: fmtDuration(data.retry_after_sec) });
    return t('err.request_failed', { status });
  }

  function download(name, blob) {
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = name;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  const stamp = () => fmtTime(new Date().toISOString()).replace(/[-: ]/g, '').slice(0, 12);

  async function copyText(text) {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      ta.remove();
    }
    toast(t('toast.copied'));
  }

  function loadPrefs() {
    lang = detectLang();
    try {
      const p = JSON.parse(localStorage.getItem(PREF_KEY) || '{}');
      if (DICT[p.lang]) { // 직접 고른 언어가 브라우저 설정보다 우선이다
        lang = p.lang;
        langPinned = true;
      }
      if (WINDOWS.includes(p.window)) state.window = p.window;
      for (const k of PREF_TOGGLES) {
        if (typeof p[k] === 'boolean') state[k] = p[k];
      }
    } catch { /* 저장된 설정이 깨졌으면 기본값을 쓴다 */ }
  }
  function savePrefs() {
    const p = { window: state.window };
    if (langPinned) p.lang = lang;
    for (const k of PREF_TOGGLES) p[k] = state[k];
    try { localStorage.setItem(PREF_KEY, JSON.stringify(p)); } catch { /* 저장이 막힌 브라우저에서는 설정을 기억하지 않는다 */ }
  }

  // ---- 언어 적용 ----------------------------------------------------------

  // index.html 의 고정 문구를 선택된 언어로 채운다.
  function applyStatic() {
    document.documentElement.lang = (LANGS.find(l => l[0] === lang) || LANGS[1])[2];
    document.querySelectorAll('[data-i18n]').forEach(e => { e.textContent = t(e.dataset.i18n); });
    document.querySelectorAll('[data-i18n-title]').forEach(e => { e.title = t(e.dataset.i18nTitle); });
    document.querySelectorAll('[data-i18n-placeholder]').forEach(e => { e.placeholder = t(e.dataset.i18nPlaceholder); });
    document.querySelectorAll('select.lang-select').forEach(s => { s.value = lang; });
    $('legend').innerHTML = LEGEND.map(c => `<span><i style="background:${PALETTE[c].hex}"></i>${esc(t('legend.' + c))}</span>`).join('') +
      `<span><b style="color:${PALETTE.amber.hex}">!</b> ${esc(t('legend.plain'))}</span>`;
    renderBrandSub();
  }

  // 사용자가 언어를 바꿨을 때: 고정 문구, 구성도, 상세 패널, 열려 있는 설치 안내를 모두 다시 그린다.
  function setLang(next) {
    if (!DICT[next]) return;
    lang = next;
    langPinned = true;
    savePrefs();
    applyStatic();
    $('login-error').hidden = true;
    if (state.topo) {
      localizeTopo();
      renderAll();
    }
    if (!el.modal.hidden && state.installInfo) renderInstallForm();
  }

  // 서버는 문장을 영어 기본값으로만 준다. 묶음 노드와 특수 영역의 이름처럼 언어에 따라 달라지는 것을 여기서 바꾼다.
  function localizeTopo() {
    const topo = state.topo;
    for (const n of topo.nodes) {
      if (n.kind === 'internet' || n.kind === 'clients') {
        n.title = t(`node.${n.kind}.title`);
        n.subtitle = t(`node.${n.kind}.sub`);
        n.tags = [t(`node.${n.kind}.tag`)];
      } else if (n.kind === 'unmanaged') {
        n.subtitle = t('node.unmanaged.sub');
      } else if (n.status === 'pending') {
        n.subtitle = t('node.pending.sub');
      } else if (n.role && hasKey('role.' + n.role)) {
        n.subtitle = t('role.' + n.role);
      }
    }
    for (const z of topo.zones) {
      const key = ZONE_TITLE_KEY[z.id];
      if (key) z.title = t(key);
      z.subtitle = t('zone.nodes', { n: z.count });
    }
    state.titles = new Map(topo.nodes.map(n => [n.id, n.title]));
  }

  // 점검 항목 1건의 제목과 설명을 선택된 언어로 만든다. 돌려주는 값은 HTML 이 아니므로 그릴 때 esc() 를 거친다.
  function findingText(f) {
    const a = { ...(f.args || {}) };
    const name = id => state.titles.get(id) || id || '';
    if (Array.isArray(a.services)) a.services = a.services.join(', ');
    if (a.age_sec !== undefined) a.age = fmtDuration(a.age_sec);
    if (a.source) a.source = name(a.source);
    if (a.target) a.target = name(a.target);
    let detailKey = `f.${f.code}.detail`;
    if (f.code === 'internet_exposed') a.host = a.target;
    if (f.code === 'unmanaged_host' && a.services) detailKey += '_svc';
    if (f.code === 'agent_warning') {
      // 에이전트 경고는 "<코드>: <원인>" 형식이다. 아는 코드는 번역하고 원인(OS 가 준 문장)은 그대로 둔다.
      const m = /^([a-z_]+): (.*)$/s.exec(String(a.message ?? ''));
      if (m && hasKey('warn.' + m[1])) a.message = `${t('warn.' + m[1])}: ${m[2]}`;
    }
    const titleKey = `f.${f.code}.title`;
    return {
      title: hasKey(titleKey) ? t(titleKey, a) : String(f.code),
      detail: hasKey(detailKey) ? t(detailKey, a) : '',
    };
  }

  // ---- 로그인 -------------------------------------------------------------

  function showLogin() {
    clearInterval(timer);
    el.app.hidden = true;
    el.modal.hidden = true;
    el.login.hidden = false;
    $('login-password').focus();
  }

  async function showApp() {
    el.login.hidden = true;
    el.app.hidden = false;
    $('login-password').value = '';
    state.fitted = false;
    await refresh();
    clearInterval(timer);
    timer = setInterval(() => { if (!document.hidden) refresh(); }, REFRESH_MS);
  }

  async function onLogin(e) {
    e.preventDefault();
    const err = $('login-error');
    err.hidden = true;
    try {
      await api('/api/v1/login', { method: 'POST', body: { password: $('login-password').value }, login: true });
      await showApp();
    } catch (ex) {
      err.textContent = ex.message;
      err.hidden = false;
    }
  }

  // ---- 데이터 -------------------------------------------------------------

  async function refresh() {
    if (drag) return;
    let topo;
    try {
      topo = await api(`/api/v1/topology?window=${state.window}`);
    } catch (ex) {
      if (!el.app.hidden) toast(ex.message, true);
      return;
    }
    if (drag) return;
    state.topo = topo;
    state.saved = topo.layout || {};
    localizeTopo();
    computeVisible();
    if (state.sel) {
      const map = state.sel.type === 'node' ? state.vis.nodeById : state.vis.edgeById;
      if (!map.has(state.sel.id)) { state.sel = null; state.detail = null; }
    }
    state.pos = computeLayout();
    renderAll();
    if (!state.fitted && state.vis.nodes.length) {
      fit();
      state.fitted = true;
    }
    if (state.sel && state.sel.type === 'node' && state.sel.id.startsWith('a:')) loadDetail(state.sel.id);
  }

  async function loadDetail(nodeId) {
    try {
      const d = await api(`/api/v1/hosts/${encodeURIComponent(nodeId.slice(2))}?window=${state.window}`);
      if (state.sel && state.sel.type === 'node' && state.sel.id === nodeId) {
        state.detail = d;
        renderInspector(true);
      }
    } catch (ex) {
      toast(ex.message, true);
    }
  }

  function computeVisible() {
    const t = state.topo;
    const nodes = t.nodes.filter(n =>
      (state.showExternal || (n.kind !== 'internet' && n.kind !== 'clients')) &&
      (state.showUnmanaged || n.kind !== 'unmanaged'));
    const nodeById = new Map(nodes.map(n => [n.id, n]));
    // "공통"을 끄면 모니터링·DNS·LDAP 같은 공통 인프라 서비스만 오가는 연결을 숨겨 업무 흐름만 남긴다
    const edges = t.edges.filter(e => nodeById.has(e.source) && nodeById.has(e.target) &&
      (state.showInfra || e.services.some(s => s.cat !== 'infra')));
    const members = new Map();
    for (const n of nodes) {
      if (!n.zone) continue;
      if (!members.has(n.zone)) members.set(n.zone, []);
      members.get(n.zone).push(n);
    }
    const zones = t.zones.filter(z => members.has(z.id));
    const pairs = new Set(edges.map(e => `${e.source}>${e.target}`));
    for (const e of edges) e._rev = pairs.has(`${e.target}>${e.source}`);
    state.vis = { nodes, edges, zones, nodeById, edgeById: new Map(edges.map(e => [e.id, e])), members };
  }

  // ---- 자동 배치 ----------------------------------------------------------
  // 영역(서브넷)마다 격자로 놓고, 영역들을 줄 단위로 채운다. 사람이 옮겨 저장한 좌표가 있으면 그것이 우선이다.

  function computeLayout() {
    const { nodes, zones, members } = state.vis;
    const pos = {};
    for (const n of nodes) {
      const p = state.saved[n.id];
      if (p) pos[n.id] = { x: p.x, y: p.y };
    }
    const anySaved = Object.keys(pos).length > 0;
    const tier = n => (n.kind === 'unmanaged' ? 9 : ({ sky: 0, amber: 2, purple: 3 }[n.accent] ?? 1));
    const zoneIds = new Set(zones.map(z => z.id));

    const fresh = [];
    for (const z of zones) {
      const list = members.get(z.id).slice().sort((a, b) => tier(a) - tier(b) || a.title.localeCompare(b.title));
      const placed = list.filter(n => pos[n.id]);
      if (!placed.length) {
        fresh.push(list);
        continue;
      }
      // 배치가 저장된 영역에 새로 나타난 서버는 그 영역 아랫줄에 둔다
      const minX = Math.min(...placed.map(n => pos[n.id].x));
      const maxY = Math.max(...placed.map(n => pos[n.id].y));
      list.filter(n => !pos[n.id]).forEach((n, i) => {
        pos[n.id] = { x: minX + i * (NODE_W + GAP_X), y: maxY + NODE_H + GAP_Y };
      });
    }
    const ext = nodes.filter(n => (!n.zone || !zoneIds.has(n.zone)) && !pos[n.id]);

    let ox = 0, oy = 0;
    if (anySaved) {
      const all = Object.values(pos);
      ox = Math.min(...all.map(p => p.x));
      oy = Math.max(...all.map(p => p.y)) + NODE_H + ZONE_GAP + ZONE_HEAD + ZONE_PAD;
    }
    packFresh(fresh, ext, ox, oy, pos);
    return pos;
  }

  function packFresh(groups, ext, ox, oy, pos) {
    const boxes = groups.map(list => {
      const cols = clamp(Math.ceil(Math.sqrt(list.length)), 1, 4);
      const rows = Math.ceil(list.length / cols);
      return {
        list, cols,
        w: cols * NODE_W + (cols - 1) * GAP_X + 2 * ZONE_PAD,
        h: ZONE_HEAD + 2 * ZONE_PAD + rows * NODE_H + (rows - 1) * GAP_Y,
      };
    });
    const area = boxes.reduce((s, b) => s + (b.w + ZONE_GAP) * (b.h + ZONE_GAP), 0);
    const maxRow = Math.max(1500, Math.sqrt(area) * 1.6);

    let x = 0, y = 0, rowH = 0, totalW = 0;
    for (const b of boxes) {
      if (x > 0 && x + b.w > maxRow) {
        x = 0;
        y += rowH + ZONE_GAP;
        rowH = 0;
      }
      b.x = x;
      b.y = y;
      x += b.w + ZONE_GAP;
      rowH = Math.max(rowH, b.h);
      totalW = Math.max(totalW, x - ZONE_GAP);
    }
    const extStep = NODE_W + GAP_X * 2;
    const extW = ext.length ? (ext.length - 1) * extStep + NODE_W : 0;
    const width = Math.max(totalW, extW);
    const top = ext.length ? NODE_H + 120 : 0;

    ext.forEach((n, i) => {
      pos[n.id] = { x: ox + (width - extW) / 2 + i * extStep, y: oy };
    });
    for (const b of boxes) {
      b.list.forEach((n, i) => {
        pos[n.id] = {
          x: ox + (width - totalW) / 2 + b.x + ZONE_PAD + (i % b.cols) * (NODE_W + GAP_X),
          y: oy + top + b.y + ZONE_HEAD + ZONE_PAD + Math.floor(i / b.cols) * (NODE_H + GAP_Y),
        };
      });
    }
  }

  function zoneRect(zoneId) {
    const list = state.vis.members.get(zoneId) || [];
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const n of list) {
      const p = state.pos[n.id];
      if (!p) continue;
      minX = Math.min(minX, p.x);
      minY = Math.min(minY, p.y);
      maxX = Math.max(maxX, p.x);
      maxY = Math.max(maxY, p.y);
    }
    return {
      x: minX - ZONE_PAD, y: minY - ZONE_PAD - ZONE_HEAD,
      w: maxX - minX + NODE_W + 2 * ZONE_PAD, h: maxY - minY + NODE_H + 2 * ZONE_PAD + ZONE_HEAD,
    };
  }

  function contentBounds() {
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const n of state.vis.nodes) {
      const p = state.pos[n.id];
      minX = Math.min(minX, p.x);
      minY = Math.min(minY, p.y);
      maxX = Math.max(maxX, p.x + NODE_W);
      maxY = Math.max(maxY, p.y + NODE_H);
    }
    for (const z of state.vis.zones) {
      const r = zoneRect(z.id);
      minX = Math.min(minX, r.x);
      minY = Math.min(minY, r.y);
      maxX = Math.max(maxX, r.x + r.w);
      maxY = Math.max(maxY, r.y + r.h);
    }
    return { x: minX, y: minY, w: maxX - minX, h: maxY - minY };
  }

  // ---- 연결선 기하 ---------------------------------------------------------

  function portPoint(p, port) {
    switch (port) {
      case 'top': return { x: p.x + NODE_W / 2, y: p.y };
      case 'right': return { x: p.x + NODE_W, y: p.y + NODE_H / 2 };
      case 'bottom': return { x: p.x + NODE_W / 2, y: p.y + NODE_H };
      default: return { x: p.x, y: p.y + NODE_H / 2 };
    }
  }

  // 두 노드가 같은 줄(또는 같은 열)에 있고 그 사이에 다른 노드가 놓여 있는가
  function blockedBetween(e, a, b, horizontal) {
    if (horizontal ? Math.abs(b.y - a.y) > NODE_H / 2 : Math.abs(b.x - a.x) > NODE_W / 2) return false;
    for (const n of state.vis.nodes) {
      if (n.id === e.source || n.id === e.target) continue;
      const p = state.pos[n.id];
      if (horizontal
        ? p.x > Math.min(a.x, b.x) && p.x < Math.max(a.x, b.x) && Math.abs(p.y - a.y) < NODE_H * 0.75
        : p.y > Math.min(a.y, b.y) && p.y < Math.max(a.y, b.y) && Math.abs(p.x - a.x) < NODE_W * 0.75) return true;
    }
    return false;
  }

  // 두 노드의 상대 위치로 나가는 면/들어오는 면을 고르고, test.html 과 같은 베지어 곡선을 만든다.
  function edgeGeom(e) {
    const a = state.pos[e.source], b = state.pos[e.target];
    if (!a || !b) return null;
    const dx = b.x - a.x, dy = b.y - a.y;
    const horizontal = Math.abs(dx) >= Math.abs(dy);
    let sp, tp, maxOff = 160;
    if (blockedBetween(e, a, b, horizontal)) {
      // 같은 줄(열)에서 사이에 다른 노드가 끼어 있으면 가로지르지 않고 노드 사이의 빈 띠로 돌아간다.
      // 가는 방향과 오는 방향은 서로 반대쪽으로 돈다.
      sp = tp = horizontal ? (dx >= 0 ? 'top' : 'bottom') : (dy >= 0 ? 'right' : 'left');
      maxOff = horizontal ? GAP_Y : GAP_X;
    } else {
      sp = horizontal ? (dx >= 0 ? 'right' : 'left') : (dy >= 0 ? 'bottom' : 'top');
      tp = horizontal ? (dx >= 0 ? 'left' : 'right') : (dy >= 0 ? 'top' : 'bottom');
    }
    const p1 = portPoint(a, sp), p2 = portPoint(b, tp);
    if (e._rev && sp !== tp) { // 양방향이면 두 선이 겹치지 않게 나란히 띄운다
      const o = e.source < e.target ? 11 : -11;
      if (horizontal) { p1.y += o; p2.y += o; } else { p1.x += o; p2.x += o; }
    }
    const off = clamp(Math.hypot(p2.x - p1.x, p2.y - p1.y) * 0.42, 45, maxOff);
    const vec = { top: [0, -off], bottom: [0, off], left: [-off, 0], right: [off, 0] };
    const c1 = { x: p1.x + vec[sp][0], y: p1.y + vec[sp][1] };
    const c2 = { x: p2.x + vec[tp][0], y: p2.y + vec[tp][1] };
    return {
      p1, p2, c1, c2, sp, tp,
      d: `M ${p1.x} ${p1.y} C ${c1.x} ${c1.y}, ${c2.x} ${c2.y}, ${p2.x} ${p2.y}`,
      mx: 0.125 * p1.x + 0.375 * c1.x + 0.375 * c2.x + 0.125 * p2.x,
      my: 0.125 * p1.y + 0.375 * c1.y + 0.375 * c2.y + 0.125 * p2.y,
    };
  }

  // ---- 그리기 -------------------------------------------------------------

  function renderAll() {
    renderStats();
    renderSidebar();
    renderCanvas();
    renderInspector(true);
    el.empty.hidden = state.topo.stats.hosts > 0;
  }

  function findingCounts() {
    const c = { high: 0, medium: 0, low: 0 };
    for (const f of state.topo.findings) c[f.severity] = (c[f.severity] || 0) + 1;
    return c;
  }

  function renderStats() {
    const s = state.topo.stats, c = findingCounts();
    const chip = (label, value, color, cls) =>
      `<span class="stat ${cls}">${color ? `<span class="dot" style="background:${color}"></span>` : ''}${label} <b>${value}</b></span>`;
    // 자리가 모자라면 뒤에 있는 칩부터 다음 줄로 밀려 숨는다 (app.css 의 .stats).
    // p1~p4 는 좁은 화면에서 남길 순서다: 점검 > 응답 없음 > 서버 > 온라인.
    el.stats.innerHTML =
      chip(esc(t('stats.hosts')), s.hosts, '', 'p3') +
      chip(esc(t('status.online')), s.online, PALETTE.emerald.hex, 'p4') +
      chip(esc(t('status.offline')), s.offline, PALETTE.rose.hex, 'p2') +
      `<button class="stat p1" id="stat-findings" title="${esc(t('stats.findings.title'))}"><span class="dot" style="background:${c.high ? PALETTE.rose.hex : c.medium ? PALETTE.amber.hex : PALETTE.emerald.hex}"></span>${esc(t('stats.findings'))} <b>${c.high}</b>/<b>${c.medium}</b>/<b>${c.low}</b></button>` +
      chip(esc(t('stats.unmanaged')), s.unmanaged, '', 'opt') +
      chip(esc(t('stats.edges')), s.edges, '', 'opt');
    renderBrandSub();
  }

  const asOf = () => t('stats.asof', { time: fmtTime(state.topo.generated_at), window: windowLabel() });
  function renderBrandSub() {
    const text = state.topo ? asOf() : t('brand.sub');
    $('brand-sub').textContent = text;
    $('brand-sub').closest('.brand').title = text; // 좁은 화면에서는 이 줄이 숨으므로 툴팁으로도 볼 수 있게 한다
  }

  function matches(n, q) {
    if (!q) return true;
    return [n.title, n.subtitle, n.ip, ...(n.tags || [])].some(v => v && String(v).toLowerCase().includes(q));
  }

  function renderSidebar() {
    const q = state.search.trim().toLowerCase();
    const { nodes, zones, members } = state.vis;
    const zoneIds = new Set(zones.map(z => z.id));
    const groups = zones.map(z => [z.title, members.get(z.id)]);
    const ext = nodes.filter(n => !n.zone || !zoneIds.has(n.zone));
    if (ext.length) groups.push([t('side.external'), ext]);

    let html = '', shown = 0;
    for (const [title, list] of groups) {
      const rows = list.filter(n => matches(n, q)).sort((a, b) => a.title.localeCompare(b.title));
      if (!rows.length) continue;
      shown += rows.length;
      html += `<div class="group-title"><span title="${esc(title)}">${esc(title)}</span><span>${rows.length}</span></div>`;
      for (const n of rows) {
        const a = accent(n.accent);
        const selected = state.sel && state.sel.type === 'node' && state.sel.id === n.id;
        html += `<button class="host${selected ? ' selected' : ''}" data-node="${esc(n.id)}" style="--acc:${a.hex}">
          <span class="badge-ico" style="background:${a.badge};color:${a.hex}">${icon(n.icon)}</span>
          <span class="meta"><span class="name" title="${esc(n.title)}">${esc(n.title)}</span><span class="sub">${esc(n.kind === 'host' ? (n.ip || n.subtitle) : n.subtitle)}</span></span>
          <span class="status-dot st-${statusKey(n.status)}" title="${esc(statusLabel(n.status))}"></span>
        </button>`;
      }
    }
    el.hostList.innerHTML = html || `<div class="list-empty">${esc(t(q ? 'side.noResults' : 'side.noHosts'))}</div>`;
    el.sidebarFoot.textContent = t('side.foot', { shown, total: nodes.length });
  }

  // 선택한 것과 이어진 노드·연결선만 또렷하게 남긴다
  function focusSets() {
    const { edges } = state.vis;
    if (!state.sel) return null;
    const nodes = new Set(), edgeIds = new Set();
    if (state.sel.type === 'node') {
      nodes.add(state.sel.id);
      for (const e of edges) {
        if (e.source === state.sel.id || e.target === state.sel.id) {
          edgeIds.add(e.id);
          nodes.add(e.source);
          nodes.add(e.target);
        }
      }
    } else {
      const e = state.vis.edgeById.get(state.sel.id);
      if (e) {
        edgeIds.add(e.id);
        nodes.add(e.source);
        nodes.add(e.target);
      }
    }
    return { nodes, edges: edgeIds };
  }

  function renderCanvas() {
    renderZones();
    renderNodes();
    renderEdges();
  }

  function renderZones() {
    el.zones.innerHTML = state.vis.zones.map(z => {
      const r = zoneRect(z.id);
      return `<div class="arch-zone" data-zone="${esc(z.id)}" style="left:${r.x}px;top:${r.y}px;width:${r.w}px;height:${r.h}px">
        <div class="zone-head" title="${esc(t('zone.drag'))}">
          <div class="zone-title">${icon('zone')}<span>${esc(z.title)}</span></div>
          <span class="tag">${esc(z.subtitle)}</span>
        </div>
      </div>`;
    }).join('');
  }

  function renderNodes() {
    const focus = focusSets();
    const q = state.search.trim().toLowerCase();
    const alerts = {};
    for (const f of state.topo.findings) {
      if (!f.node_id || f.severity === 'low') continue;
      const a = alerts[f.node_id] || (alerts[f.node_id] = { high: 0, medium: 0 });
      a[f.severity]++;
    }
    el.nodes.innerHTML = state.vis.nodes.map(n => {
      const a = accent(n.accent), p = state.pos[n.id];
      const cls = ['arch-node'];
      if (state.sel && state.sel.type === 'node' && state.sel.id === n.id) cls.push('selected');
      if (focus ? !focus.nodes.has(n.id) : !matches(n, q)) cls.push('dim');
      if (n.status === 'offline') cls.push('offline');
      if (n.kind === 'unmanaged') cls.push('unmanaged');
      const al = alerts[n.id];
      const alertHtml = al ? `<span class="alert ${al.high ? 'high' : 'medium'}" title="${esc(t('node.alert', { n: al.high + al.medium }))}">${al.high + al.medium}</span>` : '';
      const tags = (n.tags || []).slice(0, 3).map(t => `<span class="tag">${esc(t)}</span>`).join('');
      return `<div class="${cls.join(' ')}" data-node="${esc(n.id)}" style="left:${p.x}px;top:${p.y}px;--acc:${a.hex};--glow:${a.glow};--badge:${a.badge};--acc-line:${a.hex}33">
        <div class="bar"></div>
        <div class="body">
          <div class="head">
            <div class="icon-box">${icon(n.icon)}</div>
            <div class="text">
              <div class="title-row"><h3 title="${esc(n.title)}">${esc(n.title)}</h3><span class="status-dot st-${statusKey(n.status)}" title="${esc(statusLabel(n.status))}"></span></div>
              <p class="subtitle">${esc(n.subtitle)}</p>
            </div>
          </div>
          <div class="tags">${tags}</div>
        </div>${alertHtml}
      </div>`;
    }).join('');
  }

  function renderEdges() {
    const { edges } = state.vis;
    const focus = focusSets();
    const animate = state.animate && edges.length <= MAX_ANIMATED_EDGES;
    const labelAll = state.showLabels && edges.length <= MAX_LABELED_EDGES;
    let paths = '', labels = '';
    for (const e of edges) {
      const g = edgeGeom(e);
      if (!g) continue;
      const colorName = PALETTE[e.color] ? e.color : 'slate';
      const hex = PALETTE[colorName].hex;
      const selected = state.sel && state.sel.type === 'edge' && state.sel.id === e.id;
      const lit = focus && focus.edges.has(e.id);
      const dim = focus && !lit;
      const id = esc(e.id);
      paths += `<g class="edge${dim ? ' dim' : ''}">
        <path d="${g.d}" fill="none" stroke="${hex}" stroke-width="${selected ? 3 : 2}" stroke-opacity="${selected || lit ? 1 : 0.55}" marker-end="url(#arrow-${colorName})"/>
        ${animate && !dim ? `<path class="edge-flow" d="${g.d}" fill="none" stroke="#fff" stroke-width="${selected ? 2.5 : 1.8}" stroke-opacity="0.75"/>` : ''}
        <path class="edge-hit" data-edge="${id}" d="${g.d}" fill="none" stroke="transparent" stroke-width="16"/>
      </g>`;
      if (state.showLabels && (labelAll || lit)) {
        const style = selected
          ? `background:${hex};color:#070b14;border-color:#fff`
          : `border-color:${hex}66`;
        labels += `<div class="edge-label${dim ? ' dim' : ''}" data-edge="${id}" style="left:${g.mx}px;top:${g.my}px;${style}"${e.plain ? ` title="${esc(t('edge.plainTitle'))}"` : ''}>${e.plain && !selected ? '<span class="warn">!</span>' : ''}${esc(e.label)}</div>`;
      }
    }
    el.edgePaths.innerHTML = paths;
    el.labels.innerHTML = labels;
  }

  function applyView() {
    const { x, y, k } = state.view;
    el.world.style.transform = `translate(${x}px, ${y}px) scale(${k})`;
    el.viewport.style.backgroundPosition = `${x}px ${y}px`;
    el.viewport.style.backgroundSize = `${24 * k}px ${24 * k}px`;
    el.zoomLabel.textContent = `${Math.round(k * 100)}%`;
  }

  function fit() {
    if (!state.vis.nodes.length) {
      state.view = { x: 40, y: 40, k: 1 };
      applyView();
      return;
    }
    const b = contentBounds();
    const r = el.viewport.getBoundingClientRect();
    const h = r.height - 56; // 아래쪽의 확대 버튼과 범례가 구성도를 가리지 않게 비워 둔다
    const k = clamp(Math.min(r.width / (b.w + 120), h / (b.h + 100)), ZOOM_MIN, 1.1);
    state.view = { k, x: (r.width - b.w * k) / 2 - b.x * k, y: (h - b.h * k) / 2 - b.y * k };
    applyView();
  }

  function zoomAt(cx, cy, factor) {
    const v = state.view;
    const k = clamp(v.k * factor, ZOOM_MIN, ZOOM_MAX);
    v.x = cx - (cx - v.x) * (k / v.k);
    v.y = cy - (cy - v.y) * (k / v.k);
    v.k = k;
    applyView();
  }

  function centerOn(nodeId) {
    const p = state.pos[nodeId];
    if (!p) return;
    const r = el.viewport.getBoundingClientRect();
    const k = Math.max(state.view.k, 0.7);
    state.view = { k, x: r.width / 2 - (p.x + NODE_W / 2) * k, y: r.height / 2 - (p.y + NODE_H / 2) * k };
    applyView();
  }

  // ---- 선택 ---------------------------------------------------------------

  function select(sel, center) {
    const same = sel && state.sel && sel.type === state.sel.type && sel.id === state.sel.id;
    state.sel = sel;
    if (!same) state.detail = null;
    renderSidebar();
    renderNodes();
    renderEdges();
    renderInspector(same);
    if (sel && sel.type === 'node') {
      if (center) centerOn(sel.id);
      if (sel.id.startsWith('a:') && !same) loadDetail(sel.id);
    }
  }

  // ---- 상세 패널 ----------------------------------------------------------

  function renderInspector(keepScroll) {
    const top = keepScroll ? el.inspector.scrollTop : 0;
    const sel = state.sel;
    let html;
    if (!sel) html = overviewHtml();
    else if (sel.type === 'edge') html = edgeHtml(state.vis.edgeById.get(sel.id));
    else {
      const n = state.vis.nodeById.get(sel.id);
      html = n.kind === 'host' ? hostHtml(n) : pseudoHtml(n);
    }
    el.inspector.innerHTML = html;
    el.inspector.scrollTop = top;
  }

  function findingHtml(f) {
    const sev = SEVERITIES.includes(f.severity) ? f.severity : 'low';
    const target = f.node_id && state.vis.nodeById.has(f.node_id) ? ` data-node="${esc(f.node_id)}"`
      : f.edge_id && state.vis.edgeById.has(f.edge_id) ? ` data-edge="${esc(f.edge_id)}"` : '';
    const tag = target ? 'button' : 'div';
    const text = findingText(f);
    return `<${tag} class="finding ${sev}"${target}>
      <span class="badge ${sev}">${esc(t('sev.' + sev))}</span>
      <span><span class="ft">${esc(text.title)}</span><br><span class="fd">${esc(text.detail)}</span></span>
    </${tag}>`;
  }

  function overviewHtml() {
    const topo = state.topo, s = topo.stats, c = findingCounts();
    const list = topo.findings.filter(f => state.sevFilter === 'all' || f.severity === state.sevFilter);
    const fbtn = (key, label) => `<button class="btn sm${state.sevFilter === key ? ' primary' : ''}" data-sev="${key}">${label}</button>`;
    return `
      <div class="insp-head">
        <div class="icon-box" style="background:${PALETTE.sky.badge};color:${PALETTE.sky.hex}">${icon('network')}</div>
        <div class="t"><h2>${T('ov.title')}</h2><div class="s">${esc(asOf())}</div></div>
      </div>
      <div class="insp-body">
        <div class="section">
          <div class="cards">
            <div class="card"><div class="k">${T('stats.hosts')}</div><div class="v">${s.hosts}</div></div>
            <div class="card"><div class="k">${T('status.online')}</div><div class="v" style="color:${PALETTE.emerald.hex}">${s.online}</div></div>
            <div class="card"><div class="k">${T('status.offline')}</div><div class="v" style="color:${s.offline ? PALETTE.rose.hex : '#fff'}">${s.offline}</div></div>
            <div class="card"><div class="k">${T('status.unmanaged')}</div><div class="v">${s.unmanaged}</div></div>
            <div class="card"><div class="k">${T('stats.edges')}</div><div class="v">${s.edges}</div></div>
            <div class="card"><div class="k">${T('stats.zones')}</div><div class="v">${s.zones}</div></div>
          </div>
        </div>
        <div class="section">
          <h3>${T('ov.findings')} <span class="count">${topo.findings.length}</span></h3>
          <div class="filter">${fbtn('all', T('ov.all'))}${SEVERITIES.map(k => fbtn(k, `${T('sev.' + k)} ${c[k]}`)).join('')}</div>
          ${list.length ? list.map(findingHtml).join('') : `<div class="none">${T('ov.none')}</div>`}
        </div>
        <div class="section">
          <h3>${T('ov.how.title')}</h3>
          <p class="note">${T('ov.how.p1')}</p>
          <p class="note">${T('ov.how.p2')}</p>
        </div>
      </div>`;
  }

  function closeBtn() {
    return `<button class="btn icon" data-act="close" title="${T('insp.deselect')}">${icon('x')}</button>`;
  }

  function meter(label, used, total, text) {
    const pct = total ? clamp(used / total * 100, 0, 100) : 0;
    const cls = pct >= 90 ? 'crit' : pct >= 75 ? 'warn' : '';
    return `<div class="meter"><div class="row"><span>${esc(label)}</span><span>${text} · ${pct.toFixed(0)}%</span></div><div class="track"><div class="fill ${cls}" style="width:${pct.toFixed(1)}%"></div></div></div>`;
  }

  function sectionHtml(key, title, count, body) {
    return `<details class="section" data-sec="${key}"${state.open[key] ? ' open' : ''}><summary>${title}<span class="count">${count}</span></summary>${body}</details>`;
  }

  // 묶음 노드(인터넷, 내부 클라이언트)는 서버가 준 영어 이름 대신 선택된 언어의 이름을 쓴다.
  const peerName = f => (f.peer_name && String(f.peer_node_id || '').startsWith('x:') && state.titles.get(f.peer_node_id)) || f.peer_name || '';

  function peerCell(f) {
    f = { ...f, peer_name: peerName(f) };
    const name = f.peer_name || f.peer_ip || '-';
    const sub = f.peer_name && f.peer_ip ? `<br><span class="mono muted">${esc(f.peer_ip)}</span>` : '';
    if (f.peer_node_id && state.vis.nodeById.has(f.peer_node_id)) {
      return `<button class="link" data-node="${esc(f.peer_node_id)}">${esc(name)}</button>${sub}`;
    }
    return `<span class="${f.peer_name ? '' : 'mono'}">${esc(name)}</span>${sub}`;
  }

  function flowTable(rows, procLabel, withPeerProc) {
    if (!rows.length) return `<div class="none">${T('flow.none')}</div>`;
    const sortName = f => peerName(f) || f.peer_ip || '';
    const sorted = rows.slice().sort((a, b) => sortName(a).localeCompare(sortName(b)) || a.port - b.port);
    return `<table class="grid"><colgroup><col style="width:34%"><col style="width:28%"><col style="width:22%"><col style="width:16%"></colgroup>
      <thead><tr><th>${T(withPeerProc ? 'th.peerProc' : 'th.peer')}</th><th>${T('th.service')}</th><th>${esc(procLabel)}</th><th class="r">${T('th.recent')}</th></tr></thead><tbody>
      ${sorted.map(f => `<tr title="${T('flow.rowTitle', { n: f.max_conns, time: fmtTime(f.last_seen) })}">
        <td>${withPeerProc ? `<span class="mono">${esc(f.peer_process || '-')}</span>` : peerCell(f)}</td>
        <td><span class="mono">${esc(f.service)} :${f.port}${f.proto === 'udp' ? '/udp' : ''}</span>${f.plain ? ` <span class="badge plain" title="${T('badge.plainQ.title')}">${T('badge.plainQ')}</span>` : ''}</td>
        <td class="mono">${esc(f.process || '-')}</td>
        <td class="r">${fmtAgo(f.last_seen)}</td>
      </tr>`).join('')}</tbody></table>`;
  }

  function hostHtml(n) {
    const a = accent(n.accent), d = state.detail;
    const st = statusKey(n.status);
    let body = '';
    if (!d) {
      body = `<div class="none">${T('insp.loading')}</div>`;
    } else {
      const rep = d.report, h = rep ? rep.host : null;
      if (d.findings.length) {
        body += `<div class="section"><h3>${T('insp.findings')} <span class="count">${d.findings.length}</span></h3>${d.findings.map(f => findingHtml({ ...f, node_id: '' })).join('')}</div>`;
      }
      body += `<div class="section"><h3>${T('host.system')}</h3><dl class="kv">
        <dt>${T('kv.status')}</dt><dd><span class="badge ${st}">${esc(statusLabel(st))}</span></dd>
        <dt>${T('kv.lastReport')}</dt><dd>${d.last_seen ? `${esc(fmtAgo(d.last_seen))} <span class="muted">(${fmtTime(d.last_seen)})</span>` : T('kv.none')}</dd>
        ${h ? `
        <dt>IP</dt><dd class="mono">${esc(h.primary_ip || '-')}${h.primary_cidr ? ` <span class="muted">(${esc(h.primary_cidr)})</span>` : ''}</dd>
        <dt>OS</dt><dd>${esc(h.platform)} ${esc(h.platform_version)}</dd>
        <dt>${T('kv.kernel')}</dt><dd class="mono">${esc(h.kernel_version || '-')} · ${esc(h.arch)}</dd>
        ${h.virtualization ? `<dt>${T('kv.virt')}</dt><dd>${esc(h.virtualization)}</dd>` : ''}
        <dt>CPU</dt><dd>${esc(h.cpu_model || '-')} · ${T('kv.cores', { n: h.cpu_cores })}</dd>
        <dt>${T('kv.uptime')}</dt><dd>${esc(fmtUptime(h.uptime_sec))}</dd>
        ${h.zone ? `<dt>${T('kv.zone')}</dt><dd>${esc(h.zone)}</dd>` : ''}
        <dt>${T('kv.agent')}</dt><dd class="mono">${esc(rep.agent_version || '-')} · ${T('kv.interval', { n: rep.interval_sec })}</dd>` : ''}
        <dt>${T('kv.remote')}</dt><dd class="mono">${esc(d.remote_addr || '-')}</dd>
        <dt>${T('kv.enrolled')}</dt><dd>${fmtTime(d.first_seen)}</dd>
      </dl></div>`;
      if (h) {
        body += `<div class="section"><h3>${T('host.resources')} <span class="muted" style="text-transform:none;letter-spacing:0;font-weight:400">${n.status === 'offline' ? T('host.asOfLast') : ''}</span></h3>
          ${meter('CPU', h.cpu_percent, 100, T('kv.cores', { n: h.cpu_cores }))}
          ${meter(t('res.memory'), h.mem_used, h.mem_total, `${fmtBytes(h.mem_used)} / ${fmtBytes(h.mem_total)}`)}
          ${(h.disks || []).map(k => meter(t('res.disk', { mount: k.mount }), k.used, k.total, `${fmtBytes(k.used)} / ${fmtBytes(k.total)}`)).join('')}
        </div>`;

        const ifs = rep.interfaces || [];
        body += sectionHtml('interfaces', T('sec.interfaces'), ifs.length, ifs.length ? `<table class="grid"><colgroup><col style="width:38%"><col style="width:62%"></colgroup>
          <thead><tr><th>${T('th.name')}</th><th>${T('th.addr')}</th></tr></thead><tbody>
          ${ifs.map(i => `<tr class="${i.up ? '' : 'faint'}"><td>${esc(i.name)}${i.up ? '' : ' (down)'}<br><span class="mono muted">${esc(i.mac || '')}</span></td><td class="mono">${(i.addrs || []).map(esc).join('<br>') || '-'}</td></tr>`).join('')}
          </tbody></table>` : `<div class="none">${T('if.none')}</div>`);

        const ls = d.listeners.slice().sort((x, y) => x.port - y.port || x.proto.localeCompare(y.proto));
        body += sectionHtml('listeners', T('sec.listeners'), ls.length, ls.length ? `<table class="grid"><colgroup><col style="width:22%"><col style="width:24%"><col style="width:30%"><col style="width:24%"></colgroup>
          <thead><tr><th>${T('th.port')}</th><th>${T('th.service')}</th><th>${T('th.process')}</th><th>${T('th.bind')}</th></tr></thead><tbody>
          ${ls.map(l => `<tr class="${l.loopback ? 'faint' : ''}"${l.loopback ? ` title="${T('lis.loopTitle')}"` : ''}>
            <td class="mono">${l.port}/${esc(l.proto)}</td><td>${esc(l.service || '-')}</td><td class="mono">${esc(l.process || '-')}</td><td class="mono">${esc(l.ip)}</td></tr>`).join('')}
          </tbody></table><p class="note">${T('lis.note')}</p>` : `<div class="none">${T('lis.none')}</div>`);
      }
      body += sectionHtml('outbound', T('sec.outbound'), d.outbound.length, flowTable(d.outbound, t('th.process'), false));
      body += sectionHtml('inbound', T('sec.inbound'), d.inbound.length, flowTable(d.inbound, t('th.recvProc'), false));
      body += sectionHtml('local', T('sec.local'), d.local.length, flowTable(d.local, t('th.callProc'), true));
      body += `<div class="section"><button class="btn danger wide" data-act="delete">${icon('trash')}<span>${T('host.delete')}</span></button>
        <p class="note">${T('host.delete.note')}</p></div>`;
    }
    return `
      <div class="insp-head">
        <div class="icon-box" style="background:${a.badge};color:${a.hex}">${icon(n.icon)}</div>
        <div class="t"><h2 title="${esc(n.title)}">${esc(n.title)}</h2><div class="s">${esc(n.subtitle)}${n.ip ? ` · <span class="mono">${esc(n.ip)}</span>` : ''}</div></div>
        ${closeBtn()}
      </div>
      <div class="insp-body">${body}</div>`;
  }

  function linksTable(nodeId) {
    const rows = state.vis.edges.filter(e => e.source === nodeId || e.target === nodeId);
    if (!rows.length) return `<div class="none">${T('links.none')}</div>`;
    return `<table class="grid"><colgroup><col style="width:12%"><col style="width:40%"><col style="width:48%"></colgroup>
      <thead><tr><th></th><th>${T('th.peer')}</th><th>${T('th.service')}</th></tr></thead><tbody>
      ${rows.map(e => {
        const out = e.source === nodeId;
        const peer = state.vis.nodeById.get(out ? e.target : e.source);
        return `<tr><td class="mono">${out ? '→' : '←'}</td>
          <td><button class="link" data-node="${esc(peer.id)}">${esc(peer.title)}</button></td>
          <td><button class="link mono" data-edge="${esc(e.id)}">${esc(e.label)}</button></td></tr>`;
      }).join('')}</tbody></table><p class="note">${T('links.note')}</p>`;
  }

  function pseudoHtml(n) {
    const a = accent(n.accent);
    const about = ['unmanaged', 'internet', 'clients'].includes(n.kind) ? T('about.' + n.kind) : '';
    const findings = state.topo.findings.filter(f => f.node_id === n.id);
    return `
      <div class="insp-head">
        <div class="icon-box" style="background:${a.badge};color:${a.hex}">${icon(n.icon)}</div>
        <div class="t"><h2>${esc(n.title)}</h2><div class="s">${esc(n.subtitle)}</div></div>
        ${closeBtn()}
      </div>
      <div class="insp-body">
        <div class="section"><p class="note">${about}</p></div>
        ${findings.length ? `<div class="section"><h3>${T('insp.findings')}</h3>${findings.map(f => findingHtml({ ...f, node_id: '' })).join('')}</div>` : ''}
        <div class="section"><h3>${T('links.title')}</h3>${linksTable(n.id)}</div>
      </div>`;
  }

  function edgeHtml(e) {
    const a = accent(e.color);
    const src = state.vis.nodeById.get(e.source), dst = state.vis.nodeById.get(e.target);
    const findings = state.topo.findings.filter(f => f.edge_id === e.id);
    return `
      <div class="insp-head">
        <div class="icon-box" style="background:${a.badge};color:${a.hex}">${icon('arrow')}</div>
        <div class="t"><h2>${esc(src.title)} → ${esc(dst.title)}</h2><div class="s">${T('edge.services', { n: e.services.length })}</div></div>
        ${closeBtn()}
      </div>
      <div class="insp-body">
        <div class="section"><dl class="kv">
          <dt>${T('edge.from')}</dt><dd><button class="link" data-node="${esc(src.id)}">${esc(src.title)}</button></dd>
          <dt>${T('edge.to')}</dt><dd><button class="link" data-node="${esc(dst.id)}">${esc(dst.title)}</button></dd>
        </dl></div>
        ${findings.length ? `<div class="section"><h3>${T('insp.findings')}</h3>${findings.map(f => findingHtml({ ...f, edge_id: '' })).join('')}</div>` : ''}
        <div class="section"><h3>${T('th.service')} <span class="count">${e.services.length}</span></h3>
          ${e.services.map(s => `<div class="card" style="margin-bottom:8px">
            <div style="display:flex;justify-content:space-between;gap:8px;margin-bottom:6px">
              <b class="mono">${esc(s.name)} :${s.port}/${esc(s.proto)}</b>
              ${s.plain ? `<span class="badge plain" title="${T('edge.plainBadge.title')}">${T('edge.plainBadge')}</span>` : ''}
            </div>
            <dl class="kv">
              <dt>${T('edge.clientProc')}</dt><dd class="mono">${(s.client_procs || []).map(esc).join(', ') || '-'}</dd>
              <dt>${T('edge.serverProc')}</dt><dd class="mono">${(s.server_procs || []).map(esc).join(', ') || '-'}</dd>
              <dt>${T('edge.conns')}</dt><dd>${T('edge.connsMax', { n: s.max_conns })}</dd>
              <dt>${T('edge.lastSeen')}</dt><dd>${esc(fmtAgo(s.last_seen))} <span class="muted">(${fmtTime(s.last_seen)})</span></dd>
            </dl></div>`).join('')}
          <p class="note">${T('edge.procNote')}</p>
        </div>
      </div>`;
  }

  async function deleteHost() {
    const n = state.sel && state.vis.nodeById.get(state.sel.id);
    if (!n || n.kind !== 'host') return;
    if (!confirm(t('host.delete.confirm', { name: n.title }))) return;
    try {
      await api(`/api/v1/hosts/${encodeURIComponent(n.id.slice(2))}`, { method: 'DELETE' });
      state.sel = null;
      state.detail = null;
      toast(t('toast.deleted'));
      await refresh();
    } catch (ex) {
      toast(ex.message, true);
    }
  }

  // ---- 끌기 / 이동 / 확대 -------------------------------------------------

  function onPointerDown(e) {
    if (e.button !== 0 || e.target.closest('.zoom, .legend, .empty')) return;
    const edgeEl = e.target.closest('[data-edge]');
    if (edgeEl) {
      select({ type: 'edge', id: edgeEl.dataset.edge });
      return;
    }
    const nodeEl = e.target.closest('.arch-node');
    const zoneHead = e.target.closest('.zone-head');
    drag = { kind: 'pan', sx: e.clientX, sy: e.clientY, moved: false, ox: state.view.x, oy: state.view.y, ids: [], orig: {} };
    if (nodeEl) {
      drag.kind = 'node';
      drag.ids = [nodeEl.dataset.node];
    } else if (zoneHead) {
      drag.kind = 'zone';
      drag.ids = (state.vis.members.get(zoneHead.parentElement.dataset.zone) || []).map(n => n.id);
    }
    for (const id of drag.ids) drag.orig[id] = { ...state.pos[id] };
    // 캔버스 밖으로 끌고 나가도 이동과 놓기를 계속 받는다. 포인터가 이미 떼어진 경우에는 실패해도 된다.
    try { el.viewport.setPointerCapture(e.pointerId); } catch { /* 캡처 없이도 캔버스 안에서는 동작한다 */ }
  }

  function onPointerMove(e) {
    if (!drag) return;
    const dx = e.clientX - drag.sx, dy = e.clientY - drag.sy;
    if (!drag.moved) {
      if (Math.hypot(dx, dy) < 4) return;
      drag.moved = true;
      if (drag.kind === 'pan') el.viewport.classList.add('panning');
    }
    if (drag.kind === 'pan') {
      state.view.x = drag.ox + dx;
      state.view.y = drag.oy + dy;
      applyView();
      return;
    }
    const k = state.view.k;
    for (const id of drag.ids) {
      state.pos[id] = { x: Math.round(drag.orig[id].x + dx / k), y: Math.round(drag.orig[id].y + dy / k) };
    }
    if (!frame) {
      frame = requestAnimationFrame(() => {
        frame = 0;
        renderCanvas();
      });
    }
  }

  function onPointerUp() {
    if (!drag) return;
    const d = drag;
    drag = null;
    el.viewport.classList.remove('panning');
    if (!d.moved) {
      if (d.kind === 'node') select({ type: 'node', id: d.ids[0] });
      else if (d.kind === 'pan' && state.sel) select(null);
      return;
    }
    if (d.kind !== 'pan') saveLayout();
  }

  // 하나라도 옮기면 지금 보이는 배치 전체를 저장한다. 일부만 저장하면 나머지가 다음 자동 배치에서 움직이기 때문이다.
  async function saveLayout() {
    const points = {};
    for (const n of state.vis.nodes) points[n.id] = state.pos[n.id];
    Object.assign(state.saved, points);
    try {
      await api('/api/v1/layout', { method: 'PUT', body: points });
    } catch (ex) {
      toast(t('layout.saveFail', { msg: ex.message }), true);
    }
  }

  async function resetLayout() {
    if (Object.keys(state.saved).length && !confirm(t('layout.resetConfirm'))) return;
    try {
      await api('/api/v1/layout', { method: 'DELETE' });
      state.saved = {};
      state.topo.layout = {};
      state.pos = computeLayout();
      renderCanvas();
      fit();
      toast(t('toast.relayout'));
    } catch (ex) {
      toast(ex.message, true);
    }
  }

  function onWheel(e) {
    if (e.target.closest('.zoom, .legend')) return;
    e.preventDefault();
    const r = el.viewport.getBoundingClientRect();
    zoomAt(e.clientX - r.left, e.clientY - r.top, Math.exp(-e.deltaY * 0.0015));
  }

  function zoomButton(factor) {
    const r = el.viewport.getBoundingClientRect();
    zoomAt(r.width / 2, r.height / 2, factor);
  }

  // ---- 내보내기 -----------------------------------------------------------

  // test.html 편집기에서 "JSON 불러오기"로 열 수 있는 형식이다.
  function exportJSON() {
    if (!state.vis.nodes.length) {
      toast(t('export.empty'));
      return;
    }
    const zones = state.vis.zones.map(z => {
      const r = zoneRect(z.id);
      return { id: z.id, isZone: true, x: r.x, y: r.y, width: r.w, height: r.h, title: z.title, subtitle: z.subtitle, icon: 'square-dashed-bottom', accent: 'slate', tags: [], description: '' };
    });
    const nodes = state.vis.nodes.map(n => ({
      id: n.id, x: state.pos[n.id].x, y: state.pos[n.id].y, title: n.title, subtitle: n.subtitle,
      icon: LUCIDE_NAME[n.icon] || 'server', accent: PALETTE[n.accent] ? n.accent : 'slate', tags: n.tags || [],
      description: `${statusLabel(n.status)}${n.ip ? ` · ${n.ip}` : ''}`,
      kind: n.kind, status: n.status, ip: n.ip || '',
    }));
    const edges = state.vis.edges.map(e => {
      const g = edgeGeom(e);
      return {
        id: e.id, source: e.source, sourcePort: g.sp, target: e.target, targetPort: g.tp,
        label: e.label, color: PALETTE[e.color] ? e.color : 'slate', dashed: false, animated: true, bidir: false,
        plain: e.plain, services: e.services,
      };
    });
    const payload = {
      version: '1.0', source: 'infra-scanner', exportedAt: new Date().toISOString(),
      generatedAt: state.topo.generated_at, window: state.window, language: lang,
      nodes: [...zones, ...nodes], edges,
      // code / args 는 언어와 무관한 원본이고, title / detail 은 내보낼 때 선택돼 있던 언어의 문장이다
      findings: state.topo.findings.map(f => ({ ...f, ...findingText(f) })),
    };
    download(`infra-topology-${stamp()}.json`, new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' }));
    toast(t('toast.jsonSaved'));
  }

  function roundRect(ctx, x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
  }

  function fitText(ctx, text, maxW) {
    text = String(text ?? '');
    if (ctx.measureText(text).width <= maxW) return text;
    let lo = 0, hi = text.length;
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1;
      if (ctx.measureText(text.slice(0, mid) + '…').width <= maxW) lo = mid; else hi = mid - 1;
    }
    return text.slice(0, lo) + '…';
  }

  function exportPNG() {
    const { nodes, zones, edges } = state.vis;
    if (!nodes.length) {
      toast(t('export.empty'));
      return;
    }
    const css = getComputedStyle(document.body);
    const FONT = css.fontFamily, MONO = getComputedStyle(document.documentElement).getPropertyValue('--mono') || 'monospace';
    const b = contentBounds();
    const pad = 60, headH = 46;
    const W = b.w + pad * 2, H = b.h + pad * 2 + headH;
    // 브라우저의 캔버스 크기 한계를 넘지 않게 배율을 낮춘다
    const scale = Math.min(2, 14000 / W, 14000 / H, Math.sqrt(120e6 / (W * H)));
    const canvas = document.createElement('canvas');
    canvas.width = Math.round(W * scale);
    canvas.height = Math.round(H * scale);
    const ctx = canvas.getContext('2d');
    ctx.scale(scale, scale);

    ctx.fillStyle = '#070b14';
    ctx.fillRect(0, 0, W, H);
    const dot = document.createElement('canvas');
    dot.width = dot.height = 24;
    const dctx = dot.getContext('2d');
    dctx.fillStyle = '#1e293b';
    dctx.beginPath();
    dctx.arc(12, 12, 1.25, 0, Math.PI * 2);
    dctx.fill();
    ctx.fillStyle = ctx.createPattern(dot, 'repeat');
    ctx.fillRect(0, 0, W, H);

    ctx.textBaseline = 'middle';
    ctx.fillStyle = '#f8fafc';
    ctx.font = `700 16px ${FONT}`;
    ctx.fillText(t('png.title'), pad, 30);
    ctx.fillStyle = '#94a3b8';
    ctx.font = `12px ${FONT}`;
    const s = state.topo.stats;
    ctx.fillText(t('png.sub', {
      time: fmtTime(state.topo.generated_at), window: windowLabel(),
      hosts: s.hosts, online: s.online, offline: s.offline, edges: edges.length,
    }), pad, 52);

    ctx.translate(pad - b.x, pad + headH - b.y);

    for (const z of zones) {
      const r = zoneRect(z.id);
      ctx.save();
      roundRect(ctx, r.x, r.y, r.w, r.h, 16);
      ctx.fillStyle = 'rgba(15, 23, 42, 0.45)';
      ctx.fill();
      ctx.setLineDash([7, 5]);
      ctx.strokeStyle = 'rgba(100, 116, 139, 0.7)';
      ctx.lineWidth = 1.5;
      ctx.stroke();
      ctx.setLineDash([]);
      ctx.fillStyle = '#f1f5f9';
      ctx.font = `700 12px ${FONT}`;
      ctx.fillText(fitText(ctx, z.title, r.w - 120), r.x + 16, r.y + 22);
      ctx.fillStyle = '#94a3b8';
      ctx.font = `10px ${MONO}`;
      ctx.textAlign = 'right';
      ctx.fillText(z.subtitle, r.x + r.w - 16, r.y + 22);
      ctx.textAlign = 'left';
      ctx.strokeStyle = 'rgba(51, 65, 85, 0.5)';
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(r.x, r.y + ZONE_HEAD);
      ctx.lineTo(r.x + r.w, r.y + ZONE_HEAD);
      ctx.stroke();
      ctx.restore();
    }

    const pills = [];
    for (const e of edges) {
      const g = edgeGeom(e);
      if (!g) continue;
      const hex = accent(e.color).hex;
      ctx.save();
      ctx.strokeStyle = hex;
      ctx.globalAlpha = 0.75;
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(g.p1.x, g.p1.y);
      ctx.bezierCurveTo(g.c1.x, g.c1.y, g.c2.x, g.c2.y, g.p2.x, g.p2.y);
      ctx.stroke();
      const ang = Math.atan2(g.p2.y - g.c2.y, g.p2.x - g.c2.x);
      ctx.globalAlpha = 1;
      ctx.fillStyle = hex;
      ctx.beginPath();
      ctx.moveTo(g.p2.x, g.p2.y);
      ctx.lineTo(g.p2.x - 10 * Math.cos(ang - 0.45), g.p2.y - 10 * Math.sin(ang - 0.45));
      ctx.lineTo(g.p2.x - 10 * Math.cos(ang + 0.45), g.p2.y - 10 * Math.sin(ang + 0.45));
      ctx.closePath();
      ctx.fill();
      ctx.restore();
      if (state.showLabels && edges.length <= MAX_LABELED_EDGES) pills.push({ g, hex, text: (e.plain ? '! ' : '') + e.label });
    }
    ctx.font = `500 10px ${MONO}`;
    ctx.textAlign = 'center';
    for (const p of pills) {
      const w = ctx.measureText(p.text).width + 20;
      roundRect(ctx, p.g.mx - w / 2, p.g.my - 9, w, 18, 9);
      ctx.fillStyle = '#0f172a';
      ctx.fill();
      ctx.strokeStyle = p.hex + '88';
      ctx.lineWidth = 1;
      ctx.stroke();
      ctx.fillStyle = '#e2e8f0';
      ctx.fillText(p.text, p.g.mx, p.g.my + 0.5);
    }
    ctx.textAlign = 'left';

    const STATUS_COLOR = { online: PALETTE.emerald.hex, offline: PALETTE.rose.hex, pending: PALETTE.amber.hex, unmanaged: '#64748b', external: '#64748b' };
    for (const n of nodes) {
      const p = state.pos[n.id], a = accent(n.accent);
      const offline = n.status === 'offline';
      ctx.save();
      roundRect(ctx, p.x, p.y, NODE_W, NODE_H, 14);
      ctx.fillStyle = '#0f172a';
      ctx.fill();
      ctx.strokeStyle = offline ? 'rgba(251, 113, 133, 0.7)' : '#334155';
      ctx.lineWidth = 1.5;
      if (n.kind === 'unmanaged') ctx.setLineDash([5, 4]);
      ctx.stroke();
      ctx.setLineDash([]);
      ctx.save();
      roundRect(ctx, p.x, p.y, NODE_W, NODE_H, 14);
      ctx.clip();
      ctx.fillStyle = offline ? PALETTE.rose.hex : a.hex;
      ctx.fillRect(p.x, p.y, NODE_W, 4);
      ctx.restore();

      roundRect(ctx, p.x + 14, p.y + 17, 36, 36, 12);
      ctx.fillStyle = a.badge;
      ctx.fill();
      ctx.save();
      ctx.translate(p.x + 23, p.y + 26);
      ctx.scale(0.75, 0.75);
      ctx.strokeStyle = a.hex;
      ctx.lineWidth = 2;
      ctx.lineCap = 'round';
      ctx.lineJoin = 'round';
      ctx.stroke(new Path2D(ICONS[n.icon] || ICONS.server));
      ctx.restore();

      ctx.fillStyle = '#ffffff';
      ctx.font = `700 13px ${FONT}`;
      ctx.fillText(fitText(ctx, n.title, NODE_W - 92), p.x + 60, p.y + 27);
      ctx.fillStyle = '#94a3b8';
      ctx.font = `11px ${FONT}`;
      ctx.fillText(fitText(ctx, n.subtitle, NODE_W - 76), p.x + 60, p.y + 45);
      ctx.fillStyle = STATUS_COLOR[statusKey(n.status)];
      ctx.beginPath();
      ctx.arc(p.x + NODE_W - 19, p.y + 27, 4, 0, Math.PI * 2);
      ctx.fill();

      ctx.strokeStyle = 'rgba(30, 41, 59, 0.9)';
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.moveTo(p.x + 14, p.y + 64);
      ctx.lineTo(p.x + NODE_W - 14, p.y + 64);
      ctx.stroke();
      ctx.font = `10px ${MONO}`;
      let tx = p.x + 14;
      const maxX = p.x + NODE_W - 14;
      for (const tag of (n.tags || []).slice(0, 3)) {
        if (maxX - tx < 30) break;
        const text = fitText(ctx, tag, maxX - tx - 12);
        const w = ctx.measureText(text).width + 12;
        roundRect(ctx, tx, p.y + 74, w, 18, 6);
        ctx.fillStyle = '#0b1220';
        ctx.fill();
        ctx.strokeStyle = 'rgba(51, 65, 85, 0.8)';
        ctx.stroke();
        ctx.fillStyle = '#cbd5e1';
        ctx.fillText(text, tx + 6, p.y + 83.5);
        tx += w + 4;
      }
      ctx.restore();
    }

    canvas.toBlob(blob => {
      if (!blob) {
        toast(t('export.pngFail'), true);
        return;
      }
      download(`infra-topology-${stamp()}.png`, blob);
      toast(t('toast.pngSaved'));
    }, 'image/png');
  }

  // ---- 에이전트 설치 안내 --------------------------------------------------

  async function openInstall() {
    el.modal.hidden = false;
    state.installInfo = null;
    el.modalBody.innerHTML = `<div class="none">${T('insp.loading')}</div>`;
    try {
      state.installInfo = await api('/api/v1/install-info');
    } catch (ex) {
      el.modalBody.innerHTML = `<div class="form-error">${esc(ex.message)}</div>`;
      return;
    }
    if (!state.installForm.server) state.installForm.server = location.origin;
    renderInstallForm();
  }

  // 입력란을 그린다. 언어를 바꾸면 다시 불리므로 입력하던 값은 state.installForm 에 둔다.
  function renderInstallForm() {
    const form = state.installForm;
    el.modalBody.innerHTML = `
      <h3>${T('install.step1')}</h3>
      <p>${T('install.step1.p')}</p>
      <div class="field"><label for="ins-server">${T('install.server')}</label><input id="ins-server" value="${esc(form.server)}" spellcheck="false"></div>
      <div class="field"><label for="ins-zone">${T('install.zone')}</label><input id="ins-zone" value="${esc(form.zone)}" placeholder="${T('install.zone.ph')}" spellcheck="false"></div>
      <div id="ins-out"></div>`;
    for (const key of ['server', 'zone']) {
      $('ins-' + key).addEventListener('input', e => {
        form[key] = e.target.value;
        renderInstall();
      });
    }
    renderInstall();
  }

  function renderInstall() {
    const info = state.installInfo;
    const server = state.installForm.server.trim().replace(/[\s"'`$\\]/g, '').replace(/\/+$/, '');
    const zone = state.installForm.zone.trim().replace(/["'`$\\]/g, '');
    const selfsigned = info.tls_mode === 'selfsigned' && info.cert_sha256;
    const k = selfsigned ? 'k' : '';
    const pin = selfsigned ? ` -server-cert-sha256 ${info.cert_sha256}` : '';
    const http = server.startsWith('http://') ? ' -allow-http' : '';
    const block = text => `<pre class="code">${esc(text)}<button class="btn sm copy" data-copy="${esc(text)}">${icon('copy')}</button></pre>`;

    // install.* 가운데 일부는 <span class='mono'> 가 든 HTML 이다. i18n.js 에 우리가 적어 둔 고정 문장이고
    // 사용자 입력이 섞이지 않으므로 그대로 넣는다.
    let html = `<h3>${T('install.step2')}</h3>`;
    const files = info.downloads || [];
    if (!files.length) {
      html += `<p>${T('install.nofiles')}</p>`;
    }
    for (const f of files) {
      const win = f.name.endsWith('.exe');
      const url = `${server}/downloads/${f.name}`;
      const cmd = win
        ? `curl.exe -f${k}Lo infra-agent.exe ${url}\n` +
          `if ((Get-FileHash .\\infra-agent.exe -Algorithm SHA256).Hash -ne "${f.sha256.toUpperCase()}") { throw "SHA-256 mismatch" }\n` +
          `.\\infra-agent.exe install -server ${server} -token "<ENROLL_TOKEN>"${pin}${zone ? ` -zone "${zone}"` : ''}${http}`
        : `curl -f${k}Lo infra-agent ${url}\n` +
          `echo "${f.sha256}  infra-agent" | sha256sum -c - && chmod +x infra-agent\n` +
          `sudo ./infra-agent install -server ${server} -token '<ENROLL_TOKEN>'${pin}${zone ? ` -zone '${zone}'` : ''}${http}`;
      html += `<p><b class="mono" style="color:#e2e8f0">${esc(f.name)}</b> · ${fmtBytes(f.size)} · ${T(win ? 'install.win' : 'install.root')}</p>${block(cmd)}`;
    }
    html += `
      <p>${t('install.token')}</p>
      <h3>${T('install.step3')}</h3>
      <p>${t(selfsigned ? 'install.tls.selfsigned' : info.tls_mode === 'off' ? 'install.tls.off' : 'install.tls.files')}</p>
      ${info.cert_sha256 ? block(info.cert_sha256) : ''}
      <h3>${T('install.step4')}</h3>
      <p>${t('install.verify')}</p>`;
    $('ins-out').innerHTML = html;
  }

  // ---- 초기화 -------------------------------------------------------------

  function setToggle(id, key) {
    const b = $(id);
    b.classList.toggle('on', state[key]);
    b.addEventListener('click', () => {
      state[key] = !state[key];
      b.classList.toggle('on', state[key]);
      savePrefs();
      if (!state.topo) return;
      if (key === 'showExternal' || key === 'showUnmanaged' || key === 'showInfra') {
        computeVisible();
        if (state.sel) {
          const map = state.sel.type === 'node' ? state.vis.nodeById : state.vis.edgeById;
          if (!map.has(state.sel.id)) { state.sel = null; state.detail = null; }
        }
        state.pos = computeLayout();
        renderAll();
      } else {
        renderEdges();
      }
    });
  }

  function init() {
    for (const id of ['login', 'app', 'modal', 'toasts', 'stats', 'viewport', 'world', 'zones', 'nodes', 'labels', 'inspector', 'empty']) el[id] = $(id);
    el.hostList = $('host-list');
    el.sidebarFoot = $('sidebar-foot');
    el.edgePaths = $('edge-paths');
    el.zoomLabel = $('zoom-label');
    el.modalBody = $('modal-body');

    document.querySelectorAll('svg[data-icon]').forEach(svg => {
      svg.setAttribute('viewBox', '0 0 24 24');
      svg.innerHTML = `<path d="${ICONS[svg.dataset.icon]}"/>`;
    });
    $('edge-defs').innerHTML = Object.entries(PALETTE).map(([name, p]) =>
      `<marker id="arrow-${name}" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="10" markerHeight="10" markerUnits="userSpaceOnUse" orient="auto"><path d="M0 0.5 10 5 0 9.5z" fill="${p.hex}"/></marker>`).join('');

    loadPrefs();
    document.querySelectorAll('select.lang-select').forEach(s => {
      s.innerHTML = LANGS.map(([code, name]) => `<option value="${code}">${esc(name)}</option>`).join('');
      s.addEventListener('change', e => setLang(e.target.value));
    });
    applyStatic();
    $('window-select').value = state.window;
    setToggle('tg-external', 'showExternal');
    setToggle('tg-unmanaged', 'showUnmanaged');
    setToggle('tg-infra', 'showInfra');
    setToggle('tg-labels', 'showLabels');
    setToggle('tg-anim', 'animate');

    $('login-form').addEventListener('submit', onLogin);
    $('btn-logout').addEventListener('click', async () => {
      try { await api('/api/v1/logout', { method: 'POST', login: true }); } catch { /* 세션이 이미 없어도 로그인 화면으로 간다 */ }
      showLogin();
    });
    $('window-select').addEventListener('change', e => {
      state.window = e.target.value;
      savePrefs();
      renderBrandSub();
      refresh();
    });
    $('search').addEventListener('input', e => {
      state.search = e.target.value;
      renderSidebar();
      renderNodes();
    });
    $('btn-relayout').addEventListener('click', resetLayout);
    $('btn-png').addEventListener('click', exportPNG);
    $('btn-json').addEventListener('click', exportJSON);
    $('btn-install').addEventListener('click', openInstall);
    $('btn-install-empty').addEventListener('click', openInstall);
    $('btn-modal-close').addEventListener('click', () => { el.modal.hidden = true; });
    el.modal.addEventListener('click', e => {
      if (e.target === el.modal) el.modal.hidden = true;
      const c = e.target.closest('[data-copy]');
      if (c) copyText(c.dataset.copy);
    });
    $('btn-zoom-in').addEventListener('click', () => zoomButton(1.2));
    $('btn-zoom-out').addEventListener('click', () => zoomButton(1 / 1.2));
    $('btn-fit').addEventListener('click', fit);
    $('btn-refresh').addEventListener('click', async () => {
      await refresh();
      toast(t('toast.refreshed'));
    });
    el.stats.addEventListener('click', e => {
      if (e.target.closest('#stat-findings')) select(null);
    });

    el.hostList.addEventListener('click', e => {
      const b = e.target.closest('[data-node]');
      if (b) select({ type: 'node', id: b.dataset.node }, true);
    });
    el.inspector.addEventListener('click', e => {
      const t = e.target.closest('[data-node], [data-edge], [data-act], [data-sev]');
      if (!t) return;
      if (t.dataset.node) select({ type: 'node', id: t.dataset.node }, true);
      else if (t.dataset.edge) select({ type: 'edge', id: t.dataset.edge });
      else if (t.dataset.act === 'close') select(null);
      else if (t.dataset.act === 'delete') deleteHost();
      else if (t.dataset.sev) {
        state.sevFilter = t.dataset.sev;
        renderInspector(true);
      }
    });
    el.inspector.addEventListener('toggle', e => {
      if (e.target.dataset && e.target.dataset.sec) state.open[e.target.dataset.sec] = e.target.open;
    }, true);

    el.viewport.addEventListener('pointerdown', onPointerDown);
    el.viewport.addEventListener('pointermove', onPointerMove);
    el.viewport.addEventListener('pointerup', onPointerUp);
    el.viewport.addEventListener('pointercancel', onPointerUp);
    el.viewport.addEventListener('wheel', onWheel, { passive: false });
    document.addEventListener('keydown', e => {
      if (e.key !== 'Escape') return;
      if (!el.modal.hidden) el.modal.hidden = true;
      else if (state.sel) select(null);
    });
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden && !el.app.hidden) refresh();
    });

    applyView();
    api('/api/v1/me', { login: true }).then(showApp, showLogin);
  }

  init();
})();
