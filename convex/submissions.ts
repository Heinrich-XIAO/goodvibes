// SPDX-License-Identifier: AGPL-3.0-or-later
import { v } from "convex/values";
import { Doc } from "./_generated/dataModel";
import { QueryCtx } from "./_generated/server";
import { serverMutation, serverQuery } from "./lib";

async function withUser(ctx: QueryCtx, s: Doc<"audit_submissions">) {
  const { _id, _creationTime, bundle_json, verification_json, reviewed_by, ...rest } = s;
  const u = await ctx.db.get(s.user_id);
  return {
    id: _id,
    created_at: Math.floor(_creationTime / 1000),
    username: u?.username ?? "",
    display_name: u?.display_name ?? "",
    ...rest,
  };
}

export const create = serverMutation({
  args: {
    user_id: v.id("users"),
    project_name: v.string(),
    program: v.string(),
    claimed_human_seconds: v.number(),
    excluded_agent_seconds: v.number(),
    avg_struggle_score: v.number(),
    chain_valid: v.boolean(),
    matched_heartbeats: v.number(),
    total_bundle_entries: v.number(),
    trust_score: v.number(),
    notes: v.string(),
    bundle_json: v.string(),
    verification_json: v.string(),
  },
  handler: async (ctx, a) => ctx.db.insert("audit_submissions", { ...a, status: "pending", reviewer_notes: "" }),
});

export const list = serverQuery({
  args: { limit: v.number() },
  handler: async (ctx, { limit }) => {
    const rows = await ctx.db.query("audit_submissions").order("desc").take(limit);
    return Promise.all(rows.map((s) => withUser(ctx, s)));
  },
});

export const get = serverQuery({
  args: { id: v.string() },
  handler: async (ctx, { id }) => {
    const sid = ctx.db.normalizeId("audit_submissions", id);
    const s = sid ? await ctx.db.get(sid) : null;
    if (!s) return null;
    return { ...(await withUser(ctx, s)), bundle_json: s.bundle_json, verification_json: s.verification_json };
  },
});

export const review = serverMutation({
  args: {
    id: v.string(),
    status: v.string(),
    approved_human_seconds: v.optional(v.number()),
    reviewer_notes: v.string(),
    reviewed_by: v.id("users"),
    reviewed_at: v.number(),
  },
  handler: async (ctx, { id, approved_human_seconds, ...patch }) => {
    const sid = ctx.db.normalizeId("audit_submissions", id);
    const s = sid ? await ctx.db.get(sid) : null;
    if (!s) return false;
    await ctx.db.patch(s._id, { ...patch, approved_human_seconds: approved_human_seconds ?? s.claimed_human_seconds });
    return true;
  },
});
