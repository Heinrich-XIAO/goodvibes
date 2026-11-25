#!/usr/bin/env node
// SPDX-License-Identifier: AGPL-3.0-or-later
// Cross-platform Voice-to-Text (VTT) Bridge for GoodVibes.
// Works with WhisperFlow, Superwhisper, macOS Dictation, Windows Voice Access, or Talon.
//
// Usage:
//   node vtt-bridge.mjs start --harness whisperflow --cwd /path/to/repo
//   node vtt-bridge.mjs stop  --harness whisperflow --words 45 --text "optional transcript"

import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const DAEMON_URL = process.env.GOODVIBES_DAEMON_URL || 'http://127.0.0.1:42420/v1/events';
const STATE_FILE = path.join(os.tmpdir(), 'goodvibes-vtt-session.json');

function arg(name, fallback = '') {
  const idx = process.argv.indexOf(`--${name}`);
  return idx !== -1 && process.argv[idx + 1] ? process.argv[idx + 1] : fallback;
}

const cmd = process.argv[2];
const harness = arg('harness', 'vtt');
const cwd = arg('cwd', process.cwd());
const now = Math.floor(Date.now() / 1000);

if (cmd === 'start') {
  fs.writeFileSync(STATE_FILE, JSON.stringify({ startedAt: now, harness, cwd }));
  console.log(`[goodvibes-vtt] Dictation started (${harness})`);
} else if (cmd === 'stop') {
  let st = { startedAt: now - 5, harness, cwd };
  try {
    st = JSON.parse(fs.readFileSync(STATE_FILE, 'utf8'));
    fs.unlinkSync(STATE_FILE);
  } catch {}
  const durationSec = Math.max(1, Math.min(300, now - st.startedAt));
  const text = arg('text', '');
  const words = Number(arg('words', '0')) || (text ? text.trim().split(/\s+/).length : Math.round(durationSec * 2.2));

  await fetch(DAEMON_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      type: 'prompt_submitted',
      category: 'prompt_vtt',
      duration_sec: durationSec,
      cwd: st.cwd || cwd,
      harness: st.harness || harness,
      vtt_words: words,
      prompt_text: text, // stays LOCAL ONLY in daemon vault
    }),
  });
  console.log(`[goodvibes-vtt] Logged ${durationSec}s of voice prompt (${words} words)`);
} else {
  console.log('Usage: node vtt-bridge.mjs <start|stop> [--harness whisperflow] [--words N] [--text "..."]');
}
