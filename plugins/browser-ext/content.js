// SPDX-License-Identifier: AGPL-3.0-or-later
// Tracks ONLY active human prompt composition (typing or VTT dictation) in T3 Code Web
// and web AI tools. Immediately switches to `agent_working` (0s billable) when submitted.

(() => {
  const DAEMON_URL = 'http://127.0.0.1:42420/v1/events';
  const isT3Code =
    location.hostname.endsWith('t3.codes') ||
    document.title.toLowerCase().includes('t3 code') ||
    Boolean(document.querySelector('[data-t3-composer], [data-testid*="composer"]'));

  // On localhost, only activate if this page actually looks like T3 Code
  if ((location.hostname === 'localhost' || location.hostname === '127.0.0.1') && !isT3Code) {
    return;
  }

  const harness = isT3Code ? 't3-code' : location.hostname.replace(/^www\./, '');
  let composeStartedAt = 0;
  let lastInputAt = 0;
  let keystrokes = 0;
  let activeComposeSec = 0;

  function emit(ev) {
    fetch(DAEMON_URL, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ harness, project: isT3Code ? '' : `web:${harness}`, ...ev }),
    }).catch(() => {});
  }

  document.addEventListener('input', (e) => {
    const el = e.target;
    if (!el || (!el.isContentEditable && el.tagName !== 'TEXTAREA')) return;
    const now = Date.now();
    if (!composeStartedAt) composeStartedAt = now;
    // Only accumulate active typing bursts (ignore pauses > 15s)
    if (lastInputAt && now - lastInputAt < 15000) {
      activeComposeSec += (now - lastInputAt) / 1000;
    }
    lastInputAt = now;
    keystrokes++;
  }, true);

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey && activeComposeSec >= 1) {
      const dur = Math.max(1, Math.round(activeComposeSec));
      const el = e.target;
      const text = (el?.value || el?.innerText || '').trim();
      const words = text.split(/\s+/).filter(Boolean).length;
      const isVtt = words >= 8 && keystrokes < words * 2;

      emit({
        type: 'prompt_submitted',
        category: isVtt ? 'prompt_vtt' : 'prompt_typing',
        duration_sec: dur,
        prompt_text: text, // stays LOCAL ONLY in daemon vault
        keystrokes,
        vtt_words: isVtt ? words : 0,
      });

      composeStartedAt = 0;
      lastInputAt = 0;
      keystrokes = 0;
      activeComposeSec = 0;
    }
  }, true);
})();
