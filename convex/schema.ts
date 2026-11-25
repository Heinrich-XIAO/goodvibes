// SPDX-License-Identifier: AGPL-3.0-or-later
// Field names are snake_case so documents match the Go server's JSON tags 1:1.
import { defineSchema, defineTable } from "convex/server";
import { v } from "convex/values";

export const userFields = {
  hackclub_id: v.string(),
  username: v.string(),
  username_lower: v.string(),
  display_name: v.string(),
  email: v.string(),
  slack_uid: v.string(),
  github_uid: v.string(),
  github_username: v.string(),
  timezone: v.string(),
  country_code: v.string(),
  verification_status: v.string(),
  ysws_eligible: v.boolean(),
  bio: v.string(),
  allow_public_stats_lookup: v.boolean(),
  default_timezone_leaderboard: v.boolean(),
  trust_level: v.number(),
  admin_level: v.number(),
  hackatime_extension_text_type: v.number(),
};

export const heartbeatFields = {
  user_id: v.id("users"),
  time: v.number(),
  category: v.string(),
  project: v.string(),
  branch: v.string(),
  editor: v.string(),
  entity: v.string(),
  language: v.string(),
  machine: v.string(),
  operating_system: v.string(),
  type: v.string(),
  user_agent: v.string(),
  line_additions: v.number(),
  line_deletions: v.number(),
  lineno: v.number(),
  lines: v.number(),
  cursorpos: v.number(),
  project_root_count: v.number(),
  is_write: v.boolean(),
  fields_hash: v.string(),
  source_type: v.number(),
  ip_address: v.string(),
  ysws_program: v.number(),
  human_seconds: v.number(),
  agent_seconds: v.number(),
  harness: v.string(),
  model: v.string(),
  prompt_word_count: v.number(),
  prompt_tech_terms: v.number(),
  keystrokes: v.number(),
  struggle_score: v.number(),
  flags_json: v.string(),
  evidence_hash: v.string(),
};

export default defineSchema({
  users: defineTable(userFields)
    .index("by_hackclub_id", ["hackclub_id"])
    .index("by_username_lower", ["username_lower"])
    .index("by_slack_uid", ["slack_uid"])
    .index("by_public", ["allow_public_stats_lookup"]),

  sessions: defineTable({
    token: v.string(),
    user_id: v.id("users"),
    expires_at: v.number(),
  }).index("by_token", ["token"]),

  api_keys: defineTable({
    user_id: v.id("users"),
    name: v.string(),
    token: v.string(),
  }).index("by_token", ["token"]),

  heartbeats: defineTable(heartbeatFields)
    .index("by_user_time", ["user_id", "time"])
    .index("by_fields_hash", ["fields_hash"])
    .index("by_user_evidence", ["user_id", "evidence_hash"]),

  // One row per (user, UTC day, project, category). Stats and the
  // leaderboard read these instead of scanning raw heartbeats.
  daily_rollups: defineTable({
    user_id: v.id("users"),
    day: v.string(),
    project: v.string(),
    category: v.string(),
    human_seconds: v.number(),
    agent_seconds: v.number(),
    struggle_sum: v.number(),
    struggle_count: v.number(),
    heartbeats: v.number(),
  })
    .index("by_key", ["user_id", "day", "project", "category"])
    .index("by_day", ["day"]),

  audit_submissions: defineTable({
    user_id: v.id("users"),
    project_name: v.string(),
    program: v.string(),
    status: v.string(),
    claimed_human_seconds: v.number(),
    approved_human_seconds: v.optional(v.number()),
    excluded_agent_seconds: v.number(),
    avg_struggle_score: v.number(),
    chain_valid: v.boolean(),
    matched_heartbeats: v.number(),
    total_bundle_entries: v.number(),
    trust_score: v.number(),
    notes: v.string(),
    reviewer_notes: v.string(),
    reviewed_by: v.optional(v.id("users")),
    reviewed_at: v.optional(v.number()),
    bundle_json: v.string(),
    verification_json: v.string(),
  }),

  trust_level_audit_logs: defineTable({
    user_id: v.id("users"),
    changed_by_id: v.id("users"),
    previous_trust_level: v.string(),
    new_trust_level: v.string(),
    reason: v.string(),
    notes: v.string(),
  }),
});
