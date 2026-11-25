# GoodVibes for T3 Code

GoodVibes supports **T3 Code** via three complementary layers:

1. **T3 Code Web / Browser Extension** (`plugins/browser-ext/`)
   - Matches `https://*.t3.codes/*` and local T3 Code web instances (`http://localhost:*`).
   - Measures active prompt typing & VTT dictation inside the T3 Code composer, and immediately stops the clock on Enter while the agent executes.
2. **Underlying Provider Hooks** (`plugins/claude-code/`)
   - When T3 Code runs Claude Code threads, `plugins/claude-code/hook.mjs` automatically captures `UserPromptSubmit` (human prompt time) and `PreToolUse`/`Stop` (`agent_working`, excluded!).
3. **T3 Code Workspace Bridge** (`plugins/t3-code/t3-bridge.mjs`)
   - Lightweight Node bridge you can bind in T3 Code project scripts or call from VTT/shell hooks to tag events with `harness: "t3-code"`.

> Note: T3 Code's native desktop app does not yet expose a first-party composer keystroke hook to external processes. Until it does, T3 Code Web uses the browser extension directly, and T3 Code Desktop threads use the Claude Code / VTT / VS Code hooks.
