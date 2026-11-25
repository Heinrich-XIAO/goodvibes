#!/usr/bin/env node
// SPDX-License-Identifier: AGPL-3.0-or-later
// Claude Code hook bridge for GoodVibes.
// Reads Claude Code hook JSON from stdin and emits privacy-safe events to 127.0.0.1:42420.
// Ensures agent thinking/tool execution is marked `agent_working` (0s human time).

import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const DAEMON_URL = process.env.GOODVIBES_DAEMON_URL || 'http://127.0.0.1:42420/v1/events';
const STATE_FILE = path.join(os.tmpdir(), 'goodvibes-claude-code-state.json');

async function readStdin() {
  const chunks = [];
  for await (const c of process.stdin) chunks.push(c);
  const raw = Buffer.concat(chunks).toString('utf8').trim();
  return raw ? JSON.parse(raw) : {};
}

function loadState() {
  try {
    return JSON.parse(fs.readFileSync(STATE_FILE, 'utf8'));
  } catch {
    return {};
  }
}

function saveState(s) {
  try {
    fs.writeFileSync(STATE_FILE, JSON.stringify(s));
  } catch {}
}

async function emit(ev) {
  try {
    await fetch(DAEMON_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(ev),
    });
  } catch {
    // Daemon not running — fail silently so we never interrupt Claude Code
  }
}

async function main() {
  const hookEvent = process.argv[2] || 'UserPromptSubmit';
  const payload = await readStdin();
  const cwd = payload.cwd || process.cwd();
  const now = Math.floor(Date.now() / 1000);
  const st = loadState();

  if (hookEvent === 'UserPromptSubmit') {
    const prompt = String(payload.prompt || '');
    // Estimate active human composition time from elapsed time since last Stop
    // capped realistically by typing/VTT speed (word count * 1.6s, clamped 5s..180s)
    const words = prompt.trim().split(/\s+/).filter(Boolean).length;
    const estFromWords = Math.max(5, Math.min(180, Math.round(words * 1.6)));
    const elapsedSinceStop = st.lastStopAt ? Math.max(1, now - st.lastStopAt) : estFromWords;
    const composeSec = Math.min(elapsedSinceStop, estFromWords);

    await emit({
      type: 'prompt_submitted',
      category: process.env.GOODVIBES_VTT_ACTIVE === '1' ? 'prompt_vtt' : 'prompt_typing',
      duration_sec: composeSec,
      cwd,
      harness: 'claude-code',
      model: payload.model || '',
      prompt_text: prompt, // stays LOCAL ONLY in daemon vault!
      keystrokes: prompt.length,
    });
    saveState({ ...st, agentStartedAt: now });
  } else if (hookEvent === 'PreToolUse') {
    // Explicitly reinforce that the agent is working (NOT human time)
    await emit({
      type: 'agent_start',
      category: 'agent_working',
      cwd,
      harness: 'claude-code',
    });
  } else if (hookEvent === 'Stop') {
    const agentDur = st.agentStartedAt ? Math.max(1, now - st.agentStartedAt) : 5;
    await emit({
      type: 'agent_stop',
      category: 'agent_working',
      duration_sec: agentDur,
      cwd,
      harness: 'claude-code',
    });
    saveState({ ...st, agentStartedAt: 0, lastStopAt: now });
  }
}

main();
