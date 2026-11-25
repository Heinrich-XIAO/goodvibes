// SPDX-License-Identifier: AGPL-3.0-or-later
const state = { me: null, authConfigured: true, activeUsername: null, windowDays: 90, sse: null, activeSubmissionId: null };
const CAT_META = {
  prompt_typing: { label: 'Prompt Typing', color: '#ec3750' },
  prompt_vtt: { label: 'Voice-to-Text (VTT)', color: '#a633d6' },
  human_editing: { label: 'Direct Coding', color: '#33d6a6' },
  coding: { label: 'Coding', color: '#27ae60' },
  code_review: { label: 'Code Review', color: '#338eda' },
  testing: { label: 'Testing & Debug', color: '#ff8c37' },
};

const fmtDur = (s) => { s = Math.round(s || 0); return s < 60 ? `${s}s` : s < 3600 ? `${Math.round(s / 60)}m` : `${(s / 3600).toFixed(1)}h`; };
const esc = (s) => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
const fmtRel = (ts) => {
  if (!ts) return '—';
  const d = Math.max(0, Math.floor(Date.now() / 1000) - Math.floor(ts));
  return d < 60 ? `${d}s ago` : d < 3600 ? `${Math.floor(d / 60)}m ago` : d < 86400 ? `${Math.floor(d / 3600)}h ago` : `${Math.floor(d / 86400)}d ago`;
};

async function api(path, opts = {}) {
  const res = await fetch(path, { headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) }, ...opts });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

function switchView(name) {
  document.querySelectorAll('.view').forEach((v) => v.classList.add('hidden'));
  document.getElementById(`view-${name}`).classList.remove('hidden');
  document.querySelectorAll('.nav-btn').forEach((b) => b.classList.toggle('active', b.dataset.view === name));
  if (name === 'leaderboard') loadLeaderboard();
  if (name === 'audit') loadSubmissions();
  if (name === 'setup') renderSetupAccount();
}

async function loadMe() {
  const data = await api('/api/v1/auth/me');
  state.me = data.user;
  state.authConfigured = data.auth_configured !== false;
  const slot = document.getElementById('auth-slot');
  if (!state.me) {
    slot.innerHTML = `<a class="btn btn-sm btn-primary" href="/api/v1/auth/hackclub">Sign in with Hack Club</a>`;
  } else {
    slot.innerHTML = `<div class="row-gap"><a href="/u/${encodeURIComponent(state.me.username)}" style="color:var(--text);text-decoration:none">@${esc(state.me.username)}</a><button class="btn btn-sm btn-ghost" id="logout-btn">Log out</button></div>`;
    document.getElementById('logout-btn').onclick = async () => { await api('/api/v1/auth/logout', { method: 'POST' }); location.href = '/'; };
  }
  const banner = document.getElementById('auth-banner');
  if (!state.authConfigured) {
    banner.classList.remove('hidden');
    banner.innerHTML = `<strong>Hack Club Auth setup required:</strong> Set <code>HACKCLUB_CLIENT_ID</code> and <code>HACKCLUB_CLIENT_SECRET</code> with redirect URI <code>${esc(location.origin)}/oauth/callback</code>.`;
  } else {
    banner.classList.add('hidden');
  }
  renderSetupAccount();
}

async function loadProfileDirectory() {
  const data = await api('/api/v1/leaderboard?days=90');
  const sel = document.getElementById('profile-switcher');
  if (!data.users || !data.users.length) { sel.classList.add('hidden'); return []; }
  sel.classList.remove('hidden');
  sel.innerHTML = data.users.map((u) => `<option value="${esc(u.username)}" ${u.username === state.activeUsername ? 'selected' : ''}>@${esc(u.username)} (${fmtDur(u.human_seconds)})</option>`).join('');
  return data.users;
}

async function loadProfile(username) {
  if (!username) {
    document.getElementById('profile-content').classList.add('hidden');
    document.getElementById('empty-app-state').classList.remove('hidden');
    return;
  }
  state.activeUsername = username;
  try {
    const data = await api(`/api/v1/users/${encodeURIComponent(username)}/stats?days=${state.windowDays}`);
    document.getElementById('empty-app-state').classList.add('hidden');
    document.getElementById('profile-content').classList.remove('hidden');
    renderProfile(data.user, data.stats);
    connectSSE(username);
  } catch (err) {
    document.getElementById('profile-content').classList.add('hidden');
    document.getElementById('empty-app-state').classList.remove('hidden');
    document.getElementById('empty-app-msg').textContent = err.message;
  }
}

function connectSSE(username) {
  if (state.sse) state.sse.close();
  const es = new EventSource(`/api/v1/users/${encodeURIComponent(username)}/stream`);
  es.addEventListener('heartbeat', () => loadProfile(username));
  state.sse = es;
}

function renderProfile(user, stats) {
  document.getElementById('profile-avatar').textContent = (user.display_name || user.username)[0].toUpperCase();
  document.getElementById('profile-display-name').textContent = user.display_name || user.username;
  document.getElementById('profile-username').textContent = `@${user.username}`;
  document.getElementById('profile-bio').textContent = user.bio || '';
  document.getElementById('profile-verified-badge').classList.toggle('hidden', user.verification_status !== 'verified');
  document.getElementById('kpi-human').textContent = fmtDur(stats.totals.human_seconds);
  document.getElementById('kpi-score').textContent = stats.totals.heartbeat_count > 0 ? `${stats.totals.avg_struggle_score}/100` : '—';
  document.getElementById('kpi-streak').textContent = `${stats.streaks.current}d`;
  document.getElementById('kpi-streak-sub').textContent = `Longest: ${stats.streaks.longest}d`;
  document.getElementById('kpi-agent').textContent = fmtDur(stats.totals.agent_seconds);

  const totalHuman = Object.keys(CAT_META).reduce((a, k) => a + (stats.categories[k] || 0), 0);
  document.getElementById('craft-bar').innerHTML = totalHuman > 0
    ? Object.entries(CAT_META).map(([k, m]) => `<div class="stacked-seg" style="width:${(((stats.categories[k] || 0) / totalHuman) * 100).toFixed(1)}%;background:${m.color}"></div>`).join('')
    : '';
  document.getElementById('craft-legend').innerHTML = Object.entries(CAT_META)
    .map(([k, m]) => `<div class="legend-item"><span class="swatch" style="background:${m.color}"></span><span>${m.label}: <strong>${fmtDur(stats.categories[k] || 0)}</strong></span></div>`).join('');

  document.getElementById('project-list').innerHTML = (stats.projects || []).length
    ? stats.projects.map((p) => `<div class="project-item"><div><strong>${esc(p.name)}</strong></div><div><strong>${fmtDur(p.human_seconds)}</strong> <span class="muted-sm">(${fmtDur(p.agent_seconds)} agent excl.)</span></div></div>`).join('')
    : `<p class="muted-sm">No projects logged yet.</p>`;

  const badgeUrl = `/api/v1/badge/${encodeURIComponent(user.username)}.svg`;
  document.getElementById('badge-preview').innerHTML = `<img src="${badgeUrl}" alt="GoodVibes badge" />`;
  document.getElementById('copy-badge-btn').onclick = () => navigator.clipboard?.writeText(`[![GoodVibes](${location.origin}${badgeUrl})](${location.origin}/u/${user.username})`);

  document.getElementById('activity-tbody').innerHTML = (stats.recent_activity || []).length
    ? stats.recent_activity.map((a) => `<tr><td>${fmtRel(a.timestamp)}</td><td><strong>${esc(a.project)}</strong></td><td><span class="pill ${a.category === 'agent_working' ? 'pill-gray' : 'pill-green'}">${esc(a.category)}</span></td><td>${fmtDur(a.human_seconds)}</td><td class="muted-sm">${a.agent_seconds ? fmtDur(a.agent_seconds) : '—'}</td><td>${esc(a.harness || a.editor)}</td><td>${a.category === 'agent_working' ? 'excl.' : `${a.struggle_score}/100`}</td></tr>`).join('')
    : `<tr><td colspan="7" class="muted-sm">No heartbeats logged yet.</td></tr>`;
}

async function loadLeaderboard() {
  const data = await api('/api/v1/leaderboard?days=30');
  document.getElementById('leaderboard-tbody').innerHTML = (data.users || []).length
    ? data.users.map((u, i) => `<tr><td>${i + 1}</td><td><a href="/u/${encodeURIComponent(u.username)}" style="color:var(--text);font-weight:700">@${esc(u.username)}</a></td><td><strong>${fmtDur(u.human_seconds)}</strong></td><td class="muted-sm">${fmtDur(u.agent_seconds)}</td><td>${Math.round(u.avg_struggle || 0)}/100</td><td>${u.project_count}</td></tr>`).join('')
    : `<tr><td colspan="6" class="muted-sm">No public builders yet.</td></tr>`;
}

async function loadSubmissions() {
  const data = await api('/api/v1/submissions');
  const list = document.getElementById('submissions-list');
  list.innerHTML = (data.submissions || []).length
    ? data.submissions.map((s) => `<div class="submission-item" data-sub-id="${s.id}"><div><strong>${esc(s.project_name)}</strong> <span class="muted-sm">by @${esc(s.username)} · ${esc(s.program)}</span></div><div class="row-gap"><span class="pill pill-blue">Trust ${s.trust_score}/100</span><strong>${fmtDur(s.claimed_human_seconds)} human</strong></div></div>`).join('')
    : `<p class="muted-sm">No audit bundles submitted yet.</p>`;
  document.querySelectorAll('[data-sub-id]').forEach((el) => { el.onclick = () => openSubmissionDetail(Number(el.dataset.subId)); });
}

async function openSubmissionDetail(id) {
  state.activeSubmissionId = id;
  const { submission: s } = await api(`/api/v1/submissions/${id}`);
  const v = s.verification;
  document.getElementById('submission-detail').classList.remove('hidden');
  document.getElementById('sub-detail-title').textContent = `Audit #${s.id}: ${s.project_name} (@${s.username})`;
  document.getElementById('sub-detail-meta').textContent = `${s.program} · Status: ${s.status} · ${v.recommendation}`;
  document.getElementById('sub-trust-grid').innerHTML = `
    <div class="card kpi-card"><div class="kpi-label">Trust Score</div><div class="kpi-value">${v.trust_score}/100</div></div>
    <div class="card kpi-card"><div class="kpi-label">Heartbeat Match</div><div class="kpi-value">${v.heartbeat_match_rate}%</div></div>
    <div class="card kpi-card"><div class="kpi-label">Human Time</div><div class="kpi-value">${fmtDur(v.claimed_human_seconds)}</div></div>
    <div class="card kpi-card kpi-muted"><div class="kpi-label">Agent Excl.</div><div class="kpi-value">${fmtDur(v.excluded_agent_seconds)}</div></div>`;
  document.getElementById('sub-review-controls').classList.toggle('hidden', !(state.me && state.me.admin_level >= 1));
  document.getElementById('sub-entries-tbody').innerHTML = (s.bundle?.entries || []).slice(0, 50)
    .map((e) => `<tr><td>${e.seq}</td><td>${esc(e.category)}</td><td>${fmtDur(e.duration_sec)}</td><td>${esc(e.harness)}</td><td>${e.struggle_score}/100</td><td><code>${esc(e.prompt_excerpt || '—')}</code></td><td><code>${esc((e.hash || '').slice(0, 10))}…</code></td></tr>`).join('');
}

function renderSetupAccount() {
  const snippet = document.getElementById('setup-cli-snippet');
  if (snippet) snippet.textContent = `# macOS / Linux\ncurl -fsSL ${location.origin}/install.sh | sh -s -- YOUR_API_KEY\n\n# Windows (PowerShell)\n$env:GOODVIBES_API_KEY = "YOUR_API_KEY"; irm ${location.origin}/install.ps1 | iex`;
  const box = document.getElementById('setup-account-box');
  if (!state.me) {
    box.innerHTML = `<p class="muted-sm">Sign in with Hack Club Auth to generate API keys.</p><a class="btn btn-primary btn-sm" href="/api/v1/auth/hackclub">Sign in with Hack Club</a>`;
    return;
  }
  box.innerHTML = `<div class="stack"><div class="row-gap"><strong>@${esc(state.me.username)}</strong><label class="muted-sm"><input type="checkbox" id="public-toggle" ${state.me.allow_public_stats_lookup ? 'checked' : ''} /> Public profile</label></div><button class="btn btn-sm btn-primary" id="new-key-btn">Generate API Key</button><div id="new-key-output" class="muted-sm"></div></div>`;
  document.getElementById('public-toggle').onchange = async (e) => { await api('/api/v1/auth/settings', { method: 'PATCH', body: JSON.stringify({ allow_public_stats_lookup: e.target.checked }) }); };
  document.getElementById('new-key-btn').onclick = async () => {
    const res = await api('/api/v1/auth/api-keys', { method: 'POST', body: JSON.stringify({ name: 'laptop' }) });
    document.getElementById('new-key-output').innerHTML = `API Key: <code>${esc(res.api_key)}</code>`;
  };
}

document.querySelectorAll('.nav-btn').forEach((b) => { b.onclick = () => switchView(b.dataset.view); });
document.getElementById('profile-switcher').onchange = (e) => loadProfile(e.target.value);
document.getElementById('window-select').onchange = (e) => { state.windowDays = Number(e.target.value); loadProfile(state.activeUsername); };
document.getElementById('close-sub-detail').onclick = () => document.getElementById('submission-detail').classList.add('hidden');
document.querySelectorAll('[data-decision]').forEach((btn) => {
  btn.onclick = async () => {
    await api(`/api/v1/submissions/${state.activeSubmissionId}/review`, {
      method: 'POST',
      body: JSON.stringify({ status: btn.dataset.decision, approved_human_seconds: Number(document.getElementById('review-approved-sec').value || 0), reviewer_notes: document.getElementById('review-notes').value }),
    });
    await loadSubmissions();
    await openSubmissionDetail(state.activeSubmissionId);
  };
});

(async () => {
  await loadMe();
  const users = await loadProfileDirectory();
  const match = location.pathname.match(/^\/u\/([a-z0-9_-]+)/i);
  await loadProfile(match ? match[1] : state.me?.username || users[0]?.username || null);
})();
