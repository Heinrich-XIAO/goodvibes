# GoodVibes: Architecture, Privacy Model & Hosting Cost Analysis

## 1. Measuring Productive Struggle (Not Agent Idle Time or Slop)

Traditional time trackers (and naive editor plugins) break down for vibecoding because:
1. A teenager types `"fix it"` (2 seconds), watches an AI agent churn for 4 minutes, and logs 4 minutes of "coding."
2. Reviewers (e.g. Hack Club Stardance: 123,400 hours submitted vs. 27,600 approved) have no way to separate genuine engineering struggle from passive rubber-stamping.

GoodVibes solves this at the **state-machine level** (enforced both in the local Go daemon and server-side):

- **Counted (`human_seconds`)**:
  - `prompt_typing` — active human keystroke bursts composing prompts (idle gaps >15s trimmed).
  - `prompt_vtt` — active human Voice-to-Text dictation (WhisperFlow, Superwhisper, macOS Dictation, Windows Voice Access, Talon).
  - `human_editing` — direct human code edits (bulk AI file rewrites >120 chars/event are filtered out).
  - `code_review` — inspecting diffs, reading outputs, debugging.
  - `testing` — running tests, builds, or playtesting the app.
- **Strictly Excluded (`agent_seconds`, 0% billable)**:
  - `agent_working` — whenever an AI agent is thinking, streaming, or executing tools. Tracked transparently alongside human hours so both the teenager and reviewers see the exact human-vs-agent split.

## 2. Two-Tier Privacy Model (Local-First Passive vs. Opt-In Audit)

| Data Type | Passive Daily Tracking | Explicit YSWS Audit Submission |
|---|---|---|
| Raw prompts & code | **Never uploaded** (stays in `~/.goodvibes/vault/evidence.jsonl`) | Only redacted excerpts for the **one submitted project & date range** |
| Secrets / API keys / emails | Scrubbed locally before disk write | Scrubbed locally before export |
| Heartbeats sent to server | Timestamp, duration, category, project, git metadata, craft score, SHA-256 `evidence_hash` | Cross-checked against bundle's SHA-256 hash chain |
| User profile | Public by default (like Hackatime), 1-click toggle to private | Only submitted bundle visible to YSWS reviewers / Fraud Squad |

## 3. Hosting Cost Analysis (Target: <= $500/month for 15,488 MAU & 109,759 hrs/month)

### Traffic Math (Hackatime 30-day scale)
- **Monthly Active Users**: 15,488
- **Monthly Logged Hours**: 109,759 hours = 3,658 hours/day = ~152 average concurrent users (~1,500 peak).
- **Heartbeat Ingestion**: ~1 heartbeat / 30s active = ~13.2M heartbeats/month = **~5 req/sec average, ~50 req/sec peak**.
- **Storage Growth**: ~250 bytes/heartbeat × 13.2M = **~3.3 GB/month** (~40 GB/year raw, <15 GB compressed/archived).
- **Audit Bundles**: Only uploaded for submitted projects (~2,000–5,000/month × ~100 KB gzipped = **~0.5 GB/month**).

### Recommended Production Footprint
- **Compute**: the stateless Go server on Vercel (serverless) or 1× small VPS running the `Dockerfile` — **$0–$25/month**.
- **Database**: Convex. The Go server calls Convex functions over HTTP with a shared secret (`GOODVIBES_SERVER_SECRET`). Each heartbeat batch is one mutation that also updates a `daily_rollups` row, so stats and leaderboards never scan raw heartbeats. Estimate (check current Convex pricing): with daemons batching ~10 heartbeats per request, ~1.3M ingestion mutations plus ~2–4M auth and dashboard queries a month fit a Convex Professional plan, **~$25–$100/month** as storage grows ~3.3 GB/month.
- **Audit Bundle archive** on Cloudflare R2 / Backblaze B2: **~$3/month**.
- **Total Monthly Cost**: **~$30–$130/month**, well under the $500/month cap.

### Low-Spec Client Footprint (10-year-old Windows Laptop)
- Local daemon is a **single static Go binary** (`CGO_ENABLED=0`, zero runtime dependencies — no Electron, no Node, no Python required).
- Memory: **~8–12 MB RSS**.
- CPU: **<0.3% average** (event-driven + 10s idle tick).
