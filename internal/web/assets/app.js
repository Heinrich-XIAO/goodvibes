// SPDX-License-Identifier: AGPL-3.0-or-later
const state = {
  me: null,
  authConfigured: true,
  activeUsername: null,
  windowDays: 90,
  sse: null,
  sseUsername: null,
  activeSubmissionId: null,
  submissionRequest: 0,
  canReview: false,
  lastApiKey: null,
  keyPending: false,
};
const SIGN_IN = '/api/v1/auth/hackclub';
const SETUP_KEY_STORE = 'goodvibes.setupKey';
let copyTimer = null;
const STATUS_PILL = { approved: 'pill-green', deflated: 'pill-warn', flagged: 'pill-red' };
const CAT_META = {
  prompt_typing: { label: 'Prompt typing', color: '#ff5c8a' },
  prompt_vtt: { label: 'Voice-to-text (VTT)', color: '#a633d6' },
  human_editing: { label: 'Hand editing', color: '#33d6a6' },
  coding: { label: 'Coding', color: '#5fd3f3' },
  code_review: { label: 'Code review', color: '#338eda' },
  testing: { label: 'Testing & debug', color: '#ff8c37' },
};
const TITLES = {
  profile: 'GoodVibes',
  leaderboard: 'Builders · GoodVibes',
  audit: 'Audit portal · GoodVibes',
  setup: 'Setup & plugins · GoodVibes',
};

const $ = (id) => document.getElementById(id);

const fmtDur = (seconds) => {
  const s = Math.round(seconds || 0);
  if (s < 60) return `${s}s`;
  const m = Math.round(s / 60);
  return m < 60 ? `${m}m` : `${(s / 3600).toFixed(1)}h`;
};
const esc = (s) => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
const fmtRel = (ts) => {
  if (!ts) return '—';
  const d = Math.max(0, Math.floor(Date.now() / 1000) - Math.floor(ts));
  return d < 60 ? `${d}s ago` : d < 3600 ? `${Math.floor(d / 60)}m ago` : d < 86400 ? `${Math.floor(d / 3600)}h ago` : `${Math.floor(d / 86400)}d ago`;
};
const categoryLabel = (c) => CAT_META[c]?.label ?? (c === 'agent_working' ? 'Agent working' : c);

async function api(path, opts = {}) {
  const res = await fetch(path, { headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) }, ...opts });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

function emptyHtml(text, error = false) {
  return `<div class="empty-state${error ? ' is-error' : ''}">${esc(text)}</div>`;
}
function emptyRow(cols, text, error = false) {
  return `<tr><td colspan="${cols}">${emptyHtml(text, error)}</td></tr>`;
}
function showMessage(container, text, { cols = 0, error = false } = {}) {
  container.innerHTML = cols ? emptyRow(cols, text, error) : emptyHtml(text, error);
}

function setStatus(id, text, error = false) {
  const el = $(id);
  el.textContent = text;
  el.classList.toggle('is-error', error);
}

// Sign-in is a link when auth works, and a disabled control when the server has no OAuth app configured.
function setSignIn(el) {
  if (state.authConfigured) {
    el.href = SIGN_IN;
    el.removeAttribute('role');
    el.removeAttribute('aria-disabled');
    el.textContent = 'Sign in with Hack Club';
  } else {
    el.removeAttribute('href');
    el.setAttribute('role', 'link');
    el.setAttribute('aria-disabled', 'true');
    el.textContent = 'Sign-in unavailable';
  }
}

function setDecisionsEnabled(enabled) {
  document.querySelectorAll('[data-decision]').forEach((b) => { b.disabled = !enabled; });
}

function switchView(name) {
  document.querySelectorAll('main > .view').forEach((v) => v.classList.add('hidden'));
  $(`view-${name}`).classList.remove('hidden');
  document.querySelectorAll('.nav-btn').forEach((b) => {
    const active = b.dataset.view === name;
    b.classList.toggle('active', active);
    if (active) {
      b.setAttribute('aria-current', 'page');
      b.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    } else {
      b.removeAttribute('aria-current');
    }
  });
  document.title = TITLES[name];
  const heading = [...$(`view-${name}`).querySelectorAll('h1')].find((h) => h.offsetParent !== null);
  if (heading) {
    heading.tabIndex = -1;
    heading.focus();
  }
  if (name === 'leaderboard') loadLeaderboard({ silent: $('leaderboard-tbody').querySelector('a') !== null });
  if (name === 'audit') loadSubmissions({ silent: $('submissions-list').querySelector('[data-sub-id]') !== null });
  if (name === 'setup') ensureSetupKey();
}

function noDataMessage() {
  return state.me
    ? 'Connect the local daemon to start logging real productive struggle time.'
    : 'Sign in with Hack Club Auth and connect the local daemon to start logging real productive struggle time.';
}

function showProfileState(kind) {
  $('profile-loading').classList.toggle('hidden', kind !== 'loading');
  $('profile-content').classList.toggle('hidden', kind !== 'content');
  $('empty-app-state').classList.toggle('hidden', kind !== 'empty');
}

function showProfileEmpty(title, message) {
  showProfileState('empty');
  $('empty-app-title').textContent = title;
  $('empty-app-msg').textContent = message;
  const cta = $('empty-cta');
  cta.classList.toggle('hidden', Boolean(state.me));
  setSignIn(cta);
}

async function loadMe() {
  const data = await api('/api/v1/auth/me');
  state.me = data.user;
  state.authConfigured = data.auth_configured !== false;
  const slot = $('auth-slot');
  if (!state.me) {
    slot.innerHTML = `<a class="btn btn-primary" id="topbar-signin" href="${SIGN_IN}">Sign in with Hack Club</a>`;
    setSignIn($('topbar-signin'));
  } else {
    slot.innerHTML = `<div class="row-gap"><a class="plain-link" href="/u/${encodeURIComponent(state.me.username)}">@${esc(state.me.username)}</a><button class="btn btn-sm btn-ghost" id="logout-btn" type="button">Log out</button><span class="muted-sm is-error" id="logout-status" role="status"></span></div>`;
    $('logout-btn').onclick = logOut;
  }
  const banner = $('auth-banner');
  if (!state.authConfigured) {
    banner.classList.remove('hidden');
    banner.innerHTML = `<strong>Hack Club Auth setup required:</strong> Set <code>HACKCLUB_CLIENT_ID</code> and <code>HACKCLUB_CLIENT_SECRET</code> with redirect URI <code>${esc(location.origin)}/oauth/callback</code>.`;
  } else {
    banner.classList.add('hidden');
  }
  renderSetupAccount();
}

async function logOut(e) {
  const btn = e.currentTarget;
  btn.disabled = true;
  try {
    await api('/api/v1/auth/logout', { method: 'POST' });
    location.href = '/';
  } catch (err) {
    btn.disabled = false;
    setStatus('logout-status', `Log out failed: ${err.message}`, true);
  }
}

async function loadProfileDirectory() {
  const data = await api('/api/v1/leaderboard?days=90');
  const sel = $('profile-switcher');
  if (!data.users || !data.users.length) {
    sel.classList.add('hidden');
    return [];
  }
  sel.classList.remove('hidden');
  sel.innerHTML = data.users.map((u) => `<option value="${esc(u.username)}">@${esc(u.username)}</option>`).join('');
  return data.users;
}

// The switcher lists the top builders only, so a builder outside that list is added as an option.
function syncSwitcher(username) {
  const sel = $('profile-switcher');
  if (![...sel.options].some((o) => o.value === username)) sel.add(new Option(`@${username}`, username));
  sel.value = username;
}

// A silent reload keeps the current content on screen, which is used for window changes and live heartbeats.
async function loadProfile(username, { silent = false } = {}) {
  if (!username) {
    showProfileEmpty('No builder data yet', noDataMessage());
    return;
  }
  state.activeUsername = username;
  syncSwitcher(username);
  const content = $('profile-content');
  const focusedId = document.activeElement?.id || null;
  if (silent) content.setAttribute('aria-busy', 'true');
  else showProfileState('loading');
  try {
    const data = await api(`/api/v1/users/${encodeURIComponent(username)}/stats?days=${state.windowDays}`);
    if (state.activeUsername !== username) return;
    content.removeAttribute('aria-busy');
    showProfileState('content');
    renderProfile(data.user, data.stats);
    if (focusedId && $(focusedId) && !$(focusedId).disabled) $(focusedId).focus({ preventScroll: true });
    connectSSE(username);
  } catch (err) {
    content.removeAttribute('aria-busy');
    if (state.activeUsername !== username) return;
    if (silent && !content.classList.contains('hidden')) {
      setLive('offline');
      return;
    }
    showProfileEmpty("Couldn't load this profile", `@${username}: ${err.message}`);
  }
}

function setLive(status) {
  const text = { live: 'Live', offline: 'Offline', connecting: 'Connecting' }[status];
  if ($('live-pill-text').textContent === text) return;
  $('live-pill').classList.toggle('is-offline', status !== 'live');
  $('live-pill-text').textContent = text;
}

function connectSSE(username) {
  if (state.sse && state.sseUsername === username) return;
  if (state.sse) state.sse.close();
  setLive('connecting');
  const es = new EventSource(`/api/v1/users/${encodeURIComponent(username)}/stream`);
  es.onopen = () => setLive('live');
  es.onerror = () => setLive('offline');
  es.addEventListener('heartbeat', () => loadProfile(username, { silent: true }));
  state.sse = es;
  state.sseUsername = username;
}

function renderProfile(user, stats) {
  $('profile-avatar').textContent = (user.display_name || user.username)[0].toUpperCase();
  $('profile-display-name').textContent = user.display_name || user.username;
  $('profile-username').textContent = `@${user.username}`;
  $('profile-bio').textContent = user.bio || '';
  $('profile-verified-badge').classList.toggle('hidden', user.verification_status !== 'verified');
  $('kpi-human').textContent = fmtDur(stats.totals.human_seconds);
  const hasScore = stats.totals.heartbeat_count > 0;
  $('kpi-score').textContent = hasScore ? `${Math.round(stats.totals.avg_struggle_score)}/100` : '—';
  $('kpi-score-sub').textContent = hasScore ? 'Measures spec depth & verification vs. slop' : 'No heartbeats in this window yet.';
  $('kpi-streak').textContent = `${stats.streaks.current}d`;
  $('kpi-streak-sub').textContent = `Longest: ${stats.streaks.longest}d`;
  $('kpi-agent').textContent = fmtDur(stats.totals.agent_seconds);

  const totalHuman = Object.keys(CAT_META).reduce((a, k) => a + (stats.categories[k] || 0), 0);
  const bar = $('craft-bar');
  bar.classList.toggle('hidden', totalHuman === 0);
  bar.innerHTML = totalHuman > 0
    ? Object.entries(CAT_META).map(([k, m]) => `<div class="stacked-seg" style="width:${(((stats.categories[k] || 0) / totalHuman) * 100).toFixed(1)}%;background:${m.color}"></div>`).join('')
    : '';
  $('craft-legend').innerHTML = totalHuman > 0
    ? Object.entries(CAT_META).map(([k, m]) => `<div class="legend-item"><span class="swatch" style="background:${m.color}"></span><span>${m.label}: <strong>${fmtDur(stats.categories[k] || 0)}</strong></span></div>`).join('')
    : emptyHtml('No human activity in this window.');

  $('project-list').innerHTML = (stats.projects || []).length
    ? stats.projects.map((p) => `<div class="project-item"><div><strong>${esc(p.name)}</strong></div><div><strong>${fmtDur(p.human_seconds)}</strong> <span class="muted-sm">(${fmtDur(p.agent_seconds)} agent time, not counted)</span></div></div>`).join('')
    : emptyHtml('No projects logged yet.');

  const badgeUrl = `/api/v1/badge/${encodeURIComponent(user.username)}.svg`;
  const badge = $('badge-preview').querySelector('img');
  if (!badge || badge.getAttribute('src') !== badgeUrl) {
    $('badge-preview').innerHTML = `<img src="${badgeUrl}" alt="GoodVibes badge for @${esc(user.username)}" />`;
  }
  const copyBtn = $('copy-badge-btn');
  const copyStatus = $('copy-status');
  copyBtn.onclick = async () => {
    const markdown = `[![GoodVibes](${location.origin}${badgeUrl})](${location.origin}/u/${encodeURIComponent(user.username)})`;
    try {
      await navigator.clipboard.writeText(markdown);
      copyStatus.textContent = 'Copied';
      copyStatus.classList.remove('is-error');
    } catch {
      copyStatus.textContent = 'Copy failed';
      copyStatus.classList.add('is-error');
    }
    clearTimeout(copyTimer);
    copyTimer = setTimeout(() => { copyStatus.textContent = ''; }, 1500);
  };

  $('activity-tbody').innerHTML = (stats.recent_activity || []).length
    ? stats.recent_activity.map((a) => {
      const swatch = CAT_META[a.category] ? `<span class="swatch" style="background:${CAT_META[a.category].color}" aria-hidden="true"></span>` : '';
      const when = a.timestamp ? new Date(a.timestamp * 1000).toLocaleString() : '';
      return `<tr><td title="${esc(when)}">${fmtRel(a.timestamp)}</td><td><strong>${esc(a.project)}</strong></td><td><span class="pill">${swatch}${esc(categoryLabel(a.category))}</span></td><td>${fmtDur(a.human_seconds)}</td><td class="muted-sm">${a.agent_seconds ? fmtDur(a.agent_seconds) : '—'}</td><td>${esc(a.harness || a.editor || '—')}</td><td>${a.category === 'agent_working' ? '—' : `${a.struggle_score}/100`}</td></tr>`;
    }).join('')
    : emptyRow(7, 'No heartbeats logged yet.');
}

async function loadLeaderboard({ silent = false } = {}) {
  const tbody = $('leaderboard-tbody');
  const table = tbody.closest('table');
  if (!silent) {
    table.setAttribute('aria-busy', 'true');
    showMessage(tbody, 'Loading builders…', { cols: 6 });
  }
  try {
    const data = await api('/api/v1/leaderboard?days=30');
    tbody.innerHTML = (data.users || []).length
      ? data.users.map((u, i) => `<tr><td>${i + 1}</td><td><a class="plain-link plain-link--bold" href="/u/${encodeURIComponent(u.username)}">@${esc(u.username)}</a></td><td><strong>${fmtDur(u.human_seconds)}</strong></td><td class="muted-sm">${u.agent_seconds ? fmtDur(u.agent_seconds) : '—'}</td><td>${Math.round(u.avg_struggle || 0)}/100</td><td>${u.project_count}</td></tr>`).join('')
      : emptyRow(6, 'No public builders yet.');
  } catch (err) {
    showMessage(tbody, `Couldn't load builders: ${err.message}`, { cols: 6, error: true });
  } finally {
    table.removeAttribute('aria-busy');
  }
}

function submissionRow(s) {
  return `<button type="button" class="submission-item" data-sub-id="${s.id}"><span class="submission-main"><strong>${esc(s.project_name)}</strong> <span class="muted-sm">by @${esc(s.username)} · ${esc(s.program)}</span></span><span class="submission-meta"><span class="pill ${STATUS_PILL[s.status] || ''}">${esc(s.status)}</span><span class="pill pill-blue">Trust ${s.trust_score}/100</span><strong>${fmtDur(s.claimed_human_seconds)} human</strong></span></button>`;
}

function markSubmissionSelection(id) {
  document.querySelectorAll('[data-sub-id]').forEach((el) => {
    const active = Number(el.dataset.subId) === id;
    el.classList.toggle('is-active', active);
    if (active) el.setAttribute('aria-current', 'true');
    else el.removeAttribute('aria-current');
  });
}

async function loadSubmissions({ silent = false } = {}) {
  const list = $('submissions-list');
  if (!silent) showMessage(list, 'Loading audit bundles…');
  try {
    const data = await api('/api/v1/submissions');
    list.innerHTML = (data.submissions || []).length
      ? data.submissions.map(submissionRow).join('')
      : emptyHtml('No audit bundles submitted yet.');
    markSubmissionSelection(state.activeSubmissionId);
  } catch (err) {
    if (silent) setStatus('review-status', `Couldn't refresh the list: ${err.message}`, true);
    else showMessage(list, `Couldn't load audit bundles: ${err.message}`, { error: true });
  }
}

// Clears the previous bundle at once, so a decision can never be sent for the wrong bundle.
async function openSubmissionDetail(id) {
  const switched = state.activeSubmissionId !== id;
  const request = ++state.submissionRequest;
  state.activeSubmissionId = id;
  markSubmissionSelection(id);
  const detail = $('submission-detail');
  detail.classList.remove('hidden');
  setDecisionsEnabled(false);
  if (switched) {
    state.canReview = false;
    $('sub-detail-title').textContent = 'Loading audit bundle…';
    $('sub-detail-meta').textContent = '';
    $('sub-trust-grid').replaceChildren();
    $('sub-review-controls').classList.add('hidden');
    $('review-approved-sec').value = '';
    $('review-claimed-hint').textContent = '';
    $('review-notes').value = '';
    setStatus('review-status', '');
    showMessage($('sub-entries-tbody'), 'Loading evidence…', { cols: 7 });
    $('sub-entries-note').textContent = '';
    detail.scrollIntoView({ block: 'start' });
    $('sub-detail-title').focus({ preventScroll: true });
  }
  try {
    const { submission: s } = await api(`/api/v1/submissions/${id}`);
    if (request !== state.submissionRequest) return;
    const v = s.verification;
    const canReview = Boolean(state.me && state.me.admin_level >= 1);
    state.canReview = canReview;
    $('sub-detail-title').textContent = `Audit #${s.id}: ${s.project_name} (@${s.username})`;
    const statusPill = STATUS_PILL[s.status] || '';
    $('sub-detail-meta').innerHTML = `${esc(s.program)} · <span class="pill ${statusPill}">${esc(s.status)}</span> · ${esc(v.recommendation)}`;
    $('sub-trust-grid').innerHTML = `
      <div class="kpi-card kpi-tile"><div class="kpi-label">Trust score</div><div class="kpi-value">${v.trust_score}/100</div></div>
      <div class="kpi-card kpi-tile"><div class="kpi-label">Heartbeat match</div><div class="kpi-value">${v.heartbeat_match_rate}%</div></div>
      <div class="kpi-card kpi-tile"><div class="kpi-label">Human time</div><div class="kpi-value">${fmtDur(v.claimed_human_seconds)}</div></div>
      <div class="kpi-card kpi-tile kpi-muted"><div class="kpi-label">Agent time (not counted)</div><div class="kpi-value">${fmtDur(v.excluded_agent_seconds)}</div></div>`;
    $('sub-review-controls').classList.toggle('hidden', !canReview);
    if (switched) {
      $('review-approved-sec').value = v.claimed_human_seconds ?? 0;
      $('review-claimed-hint').textContent = `Claimed: ${fmtDur(v.claimed_human_seconds)}`;
    }
    setDecisionsEnabled(canReview);
    const entries = s.bundle?.entries || [];
    $('sub-entries-tbody').innerHTML = entries.length
      ? entries.slice(0, 50).map((e) => `<tr><td>${e.seq}</td><td>${esc(categoryLabel(e.category))}</td><td>${fmtDur(e.duration_sec)}</td><td>${esc(e.harness)}</td><td>${e.struggle_score}/100</td><td><code>${esc(e.prompt_excerpt || '—')}</code></td><td>${e.hash ? `<code title="${esc(e.hash)}">${esc(e.hash.slice(0, 10))}…</code>` : '—'}</td></tr>`).join('')
      : emptyRow(7, 'No entries in this bundle.');
    $('sub-entries-note').textContent = entries.length > 50 ? `Showing 50 of ${entries.length} entries.` : '';
  } catch (err) {
    if (request !== state.submissionRequest) return;
    state.canReview = false;
    $('sub-detail-title').textContent = "Couldn't load this audit bundle";
    $('sub-detail-meta').innerHTML = `<span class="is-error">${esc(err.message)}</span>`;
    $('sub-trust-grid').replaceChildren();
    $('sub-review-controls').classList.add('hidden');
    setDecisionsEnabled(false);
    showMessage($('sub-entries-tbody'), `Couldn't load evidence: ${err.message}`, { cols: 7, error: true });
    $('sub-entries-note').textContent = '';
  }
}

function closeSubmissionDetail() {
  const previous = state.activeSubmissionId;
  state.submissionRequest++;
  state.activeSubmissionId = null;
  state.canReview = false;
  $('submission-detail').classList.add('hidden');
  markSubmissionSelection(null);
  document.querySelector(`[data-sub-id="${previous}"]`)?.focus();
}

function renderSnippet(apiKey = 'YOUR_API_KEY') {
  const snippet = $('setup-cli-snippet');
  snippet.textContent = `# macOS / Linux\ncurl -fsSL ${location.origin}/install.sh | sh -s -- ${apiKey}\n\n# Windows (PowerShell)\n$env:GOODVIBES_API_KEY = "${apiKey}"; irm ${location.origin}/install.ps1 | iex`;
}

function renderSetupAccount() {
  renderSnippet(state.lastApiKey || 'YOUR_API_KEY');
  const box = $('setup-account-box');
  if (!state.me) {
    box.innerHTML = `<div class="stack"><p class="muted-sm">Sign in with Hack Club Auth to generate API keys.</p><a class="btn btn-primary" id="setup-signin" href="${SIGN_IN}">Sign in with Hack Club</a></div>`;
    setSignIn($('setup-signin'));
    return;
  }
  box.innerHTML = `<div class="stack">
    <div class="row-gap"><strong>@${esc(state.me.username)}</strong></div>
    <div class="row-gap"><label><input type="checkbox" id="public-toggle" ${state.me.allow_public_stats_lookup ? 'checked' : ''} /> Public profile</label><span id="privacy-status" class="muted-sm" role="status"></span></div>
    <button class="btn btn-sm btn-primary" id="new-key-btn" type="button">Generate new key</button>
    <div id="new-key-output" role="status"></div>
  </div>`;
  $('public-toggle').onchange = togglePublic;
  $('new-key-btn').onclick = generateKey;
}

async function togglePublic(e) {
  const box = e.currentTarget;
  box.disabled = true;
  setStatus('privacy-status', 'Saving…');
  try {
    await api('/api/v1/auth/settings', { method: 'PATCH', body: JSON.stringify({ allow_public_stats_lookup: box.checked }) });
    setStatus('privacy-status', 'Saved');
  } catch (err) {
    box.checked = !box.checked;
    setStatus('privacy-status', `Couldn't save: ${err.message}`, true);
  } finally {
    box.disabled = false;
  }
}

// Opening the setup page fills a key into step 1, so the install command is ready to paste.
function ensureSetupKey() {
  if (!state.me || state.lastApiKey || state.keyPending) return;
  const saved = savedSetupKey();
  if (saved) showSetupKey(saved);
  else generateKey();
}

// One key per tab: a reload of the setup page reuses the key instead of minting another.
function savedSetupKey() {
  try {
    const saved = JSON.parse(sessionStorage.getItem(SETUP_KEY_STORE) || 'null');
    return saved && state.me && saved.username === state.me.username ? saved.key : null;
  } catch {
    return null;
  }
}

function rememberSetupKey(key) {
  try {
    sessionStorage.setItem(SETUP_KEY_STORE, JSON.stringify({ username: state.me.username, key }));
  } catch {
    // Storage can be blocked. The key still shows on the page.
  }
}

function showSetupKey(key) {
  state.lastApiKey = key;
  renderSnippet(key);
  $('new-key-output').innerHTML = `API key: <code>${esc(key)}</code><br>Copy it now and keep it safe. Step 1 on this page already includes it.`;
}

async function generateKey() {
  if (state.keyPending) return;
  state.keyPending = true;
  const btn = $('new-key-btn');
  const out = $('new-key-output');
  btn.disabled = true;
  btn.textContent = 'Generating…';
  out.textContent = 'Creating your API key…';
  try {
    const res = await api('/api/v1/auth/api-keys', { method: 'POST', body: JSON.stringify({ name: 'laptop' }) });
    rememberSetupKey(res.api_key);
    showSetupKey(res.api_key);
  } catch (err) {
    out.innerHTML = `<span class="is-error">Couldn't create a key: ${esc(err.message)}</span>`;
  } finally {
    state.keyPending = false;
    btn.disabled = false;
    btn.textContent = 'Generate new key';
  }
}

document.querySelectorAll('.nav-btn').forEach((b) => { b.onclick = () => switchView(b.dataset.view); });
$('profile-switcher').onchange = (e) => loadProfile(e.target.value);
$('window-select').onchange = (e) => {
  state.windowDays = Number(e.target.value);
  loadProfile(state.activeUsername, { silent: true });
};
$('close-sub-detail').onclick = closeSubmissionDetail;
$('submissions-list').addEventListener('click', (e) => {
  const row = e.target.closest('[data-sub-id]');
  if (row) openSubmissionDetail(Number(row.dataset.subId));
});

document.addEventListener('keydown', (e) => {
  if (e.key !== 'Escape' || $('view-audit').classList.contains('hidden') || $('submission-detail').classList.contains('hidden')) return;
  if (['INPUT', 'SELECT', 'TEXTAREA'].includes(e.target.tagName)) return;
  closeSubmissionDetail();
});

document.querySelectorAll('[data-decision]').forEach((btn) => {
  btn.onclick = async () => {
    const id = state.activeSubmissionId;
    const decision = btn.dataset.decision;
    if (!id || !state.canReview) return;
    if (decision === 'flagged' && !confirm('Flag this submission for the Fraud Squad?')) return;
    if (decision !== 'flagged' && $('review-approved-sec').value === '') {
      setStatus('review-status', 'Enter approved human seconds first.', true);
      return;
    }
    setDecisionsEnabled(false);
    setStatus('review-status', 'Saving…');
    try {
      await api(`/api/v1/submissions/${id}/review`, {
        method: 'POST',
        body: JSON.stringify({ status: decision, approved_human_seconds: Number($('review-approved-sec').value || 0), reviewer_notes: $('review-notes').value }),
      });
      await loadSubmissions({ silent: true });
      if (state.activeSubmissionId === id) {
        await openSubmissionDetail(id);
        setStatus('review-status', `Saved as ${decision}.`);
        if (!btn.disabled) btn.focus({ preventScroll: true });
      }
    } catch (err) {
      if (state.activeSubmissionId === id) {
        setStatus('review-status', `Couldn't save: ${err.message}`, true);
        setDecisionsEnabled(state.canReview);
      }
    }
  };
});

// Each step fails on its own, so a leaderboard or account error never blocks the dashboard.
(async () => {
  await loadMe().catch(() => renderSetupAccount());
  if (location.pathname === '/setup') switchView('setup');
  const users = await loadProfileDirectory().catch(() => []);
  const match = location.pathname.match(/^\/u\/([a-z0-9_-]+)/i);
  await loadProfile(match ? match[1] : state.me?.username || users[0]?.username || null);
})();
