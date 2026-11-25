#!/usr/bin/env node
// SPDX-License-Identifier: AGPL-3.0-or-later
// T3 Code workspace bridge for GoodVibes.
// Can be run as a T3 Code project script or alongside T3 Code threads.
// Emits workspace git/test/review context tagged with `harness: "t3-code"`,
// and pairs with `plugins/claude-code/hook.mjs` + `plugins/browser-ext/` so
// prompt typing & VTT are counted while T3 Code agent turns are excluded.

import { execSync } from 'node:child_process';

const DAEMON_URL = process.env.GOODVIBES_DAEMON_URL || 'http://127.0.0.1:42420/v1/events';
const cwd = process.cwd();

async function emit(ev) {
  try {
    await fetch(DAEMON_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ harness: 't3-code', cwd, ...ev }),
    });
  } catch {
    // Daemon offline
  }
}

const mode = process.argv[2] || 'status';
if (mode === 'prompt') {
  const dur = Number(process.argv[3] || 30);
  const text = process.argv.slice(4).join(' ');
  await emit({
    type: 'prompt_submitted',
    category: process.env.GOODVIBES_VTT_ACTIVE === '1' ? 'prompt_vtt' : 'prompt_typing',
    duration_sec: dur,
    prompt_text: text,
  });
  console.log(`[goodvibes:t3-code] Logged ${dur}s human prompt composition`);
} else if (mode === 'agent-done') {
  const dur = Number(process.argv[3] || 60);
  await emit({
    type: 'agent_stop',
    category: 'agent_working',
    duration_sec: dur,
  });
  console.log(`[goodvibes:t3-code] Recorded ${dur}s excluded agent_working span`);
} else {
  let branch = '';
  try {
    branch = execSync('git rev-parse --abbrev-ref HEAD', { cwd, encoding: 'utf8' }).trim();
  } catch {}
  console.log(`GoodVibes T3 Code bridge ready (cwd=${cwd}, branch=${branch || 'n/a'}, daemon=${DAEMON_URL})`);
}
