# GoodVibes Plugin & Harness Protocol Specification (v1)

GoodVibes works with **every AI model and every harness** (CLI agents, IDE copilots, Voice-to-Text tools, and browser-based vibecoding tools) via a tiny, language-agnostic local HTTP API exposed by the `goodvibes daemon` at:

```
POST http://127.0.0.1:42420/v1/events
Content-Type: application/json
```

(Or via the CLI fallback: `goodvibes emit --type ... --category ...`)

## Core Rule: Never Count Agent Execution Time

Plugins MUST distinguish **active human effort** from **AI agent execution**:

| Category | Counts as Human Time? | When to emit |
|---|---|---|
| `prompt_typing` | **Yes** | Human is actively typing a prompt into a harness input |
| `prompt_vtt` | **Yes** | Human is actively dictating a prompt via Voice-to-Text (VTT) |
| `human_editing` | **Yes** | Human is directly editing project code (not bulk AI paste/rewrite) |
| `code_review` | **Yes** | Human is scrolling/inspecting diffs, reading agent output, or debugging |
| `testing` | **Yes** | Human is running tests, builds, or manually testing the running app |
| `agent_working` | **NO (0s human)** | AI agent is thinking, streaming, or executing tools |
| `idle` | **NO (0s human)** | No human input |

## Event Payload Schema

```json
{
  "type": "prompt_submitted | pulse | start | stop | agent_start | agent_stop",
  "category": "prompt_typing | prompt_vtt | human_editing | code_review | testing | agent_working",
  "duration_sec": 45,
  "cwd": "/path/to/project",
  "project": "optional-override",
  "harness": "claude-code | cursor | codex | whisperflow | web-claude | custom",
  "model": "claude-sonnet-4",
  "language": "TypeScript",
  "prompt_text": "OPTIONAL — stays LOCAL ONLY in ~/.goodvibes/vault/ (secret-scrubbed), NEVER sent in heartbeats",
  "keystrokes": 142,
  "vtt_words": 38
}
```

## Privacy Guarantee

`prompt_text` is **never transmitted** to the centralized server during normal passive tracking. The local daemon uses it strictly on-device to compute word count, technical term specificity, low-effort slop checks, and a secret-scrubbed local vault entry bound by a SHA-256 hash chain.
