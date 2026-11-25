// SPDX-License-Identifier: AGPL-3.0-or-later
// Lightweight VS Code / Cursor / Windsurf extension (<2MB RAM).
// Distinguishes genuine human typing & diff review from bulk AI agent file rewrites.

const vscode = require('vscode');
const path = require('path');

const DAEMON_URL = 'http://127.0.0.1:42420/v1/events';
let lastSentAt = 0;

function detectHarness() {
  const name = (vscode.env.appName || '').toLowerCase();
  if (name.includes('cursor')) return 'cursor';
  if (name.includes('windsurf')) return 'windsurf';
  return 'vscode';
}

async function emitPulse(category, doc, keystrokes = 0) {
  const now = Date.now();
  if (now - lastSentAt < 5000) return; // throttle to <= 1 event per 5s
  lastSentAt = now;

  const cwd = doc?.uri?.fsPath
    ? path.dirname(doc.uri.fsPath)
    : vscode.workspace.workspaceFolders?.[0]?.uri?.fsPath || '';

  try {
    await fetch(DAEMON_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        type: 'pulse',
        category,
        cwd,
        harness: detectHarness(),
        language: doc?.languageId || '',
        keystrokes,
      }),
    });
  } catch {
    // Local daemon offline; ignore silently
  }
}

function activate(context) {
  // 1. Human editing vs. AI bulk rewrite filter:
  // AI tools apply large multi-line edits in a single change event (>120 chars at once).
  // Genuine human typing produces small incremental edits.
  context.subscriptions.push(
    vscode.workspace.onDidChangeTextDocument((e) => {
      if (e.document.uri.scheme !== 'file' || e.contentChanges.length === 0) return;
      const totalInserted = e.contentChanges.reduce((sum, c) => sum + c.text.length, 0);
      if (totalInserted > 120) {
        // Likely AI agent edit or paste — do NOT count as human_editing
        emitPulse('agent_working', e.document, 0);
        return;
      }
      emitPulse('human_editing', e.document, totalInserted);
    })
  );

  // 2. Active code review / diff inspection (scrolling or selecting code)
  context.subscriptions.push(
    vscode.window.onDidChangeTextEditorSelection((e) => {
      if (e.textEditor.document.uri.scheme === 'file' || e.textEditor.document.uri.scheme === 'git') {
        emitPulse('code_review', e.textEditor.document, 0);
      }
    })
  );

  // 3. Running tests / builds in integrated terminal
  context.subscriptions.push(
    vscode.window.onDidStartTerminalShellExecution?.((e) => {
      emitPulse('testing', vscode.window.activeTextEditor?.document, 0);
    }) || { dispose() {} }
  );
}

function deactivate() {}

module.exports = { activate, deactivate };
