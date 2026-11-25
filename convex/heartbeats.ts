// SPDX-License-Identifier: AGPL-3.0-or-later
import { v } from "convex/values";
import { heartbeatFields } from "./schema";
import { serverMutation, serverQuery, utcDay } from "./lib";

// Inserts heartbeats, skipping any whose fields_hash already exists, and
// folds each new one into its daily rollup row.
export const insertMany = serverMutation({
  args: { heartbeats: v.array(v.object(heartbeatFields)) },
  handler: async (ctx, { heartbeats }) => {
    const results: { id: string | null; duplicate: boolean }[] = [];
    for (const hb of heartbeats) {
      if (hb.fields_hash) {
        const dup = await ctx.db
          .query("heartbeats")
          .withIndex("by_fields_hash", (q) => q.eq("fields_hash", hb.fields_hash))
          .first();
        if (dup) {
          results.push({ id: dup._id, duplicate: true });
          continue;
        }
      }
      const id = await ctx.db.insert("heartbeats", hb);
      results.push({ id, duplicate: false });

      const day = utcDay(hb.time);
      const category = hb.category.toLowerCase();
      const scored = hb.human_seconds > 0 ? 1 : 0;
      const row = await ctx.db
        .query("daily_rollups")
        .withIndex("by_key", (q) =>
          q.eq("user_id", hb.user_id).eq("day", day).eq("project", hb.project).eq("category", category),
        )
        .unique();
      if (row) {
        await ctx.db.patch(row._id, {
          human_seconds: row.human_seconds + hb.human_seconds,
          agent_seconds: row.agent_seconds + hb.agent_seconds,
          struggle_sum: row.struggle_sum + scored * hb.struggle_score,
          struggle_count: row.struggle_count + scored,
          heartbeats: row.heartbeats + 1,
        });
      } else {
        await ctx.db.insert("daily_rollups", {
          user_id: hb.user_id,
          day,
          project: hb.project,
          category,
          human_seconds: hb.human_seconds,
          agent_seconds: hb.agent_seconds,
          struggle_sum: scored * hb.struggle_score,
          struggle_count: scored,
          heartbeats: 1,
        });
      }
    }
    return results;
  },
});

export const recent = serverQuery({
  args: { user_id: v.id("users"), limit: v.number() },
  handler: async (ctx, { user_id, limit }) => {
    const rows = await ctx.db
      .query("heartbeats")
      .withIndex("by_user_time", (q) => q.eq("user_id", user_id))
      .order("desc")
      .take(limit);
    return rows.map(({ _id, _creationTime, ...hb }) => ({ id: _id, ...hb }));
  },
});

// Most recent non-agent heartbeat times, oldest first, for span building.
export const humanTimes = serverQuery({
  args: { user_id: v.id("users"), limit: v.number() },
  handler: async (ctx, { user_id, limit }) => {
    const rows = await ctx.db
      .query("heartbeats")
      .withIndex("by_user_time", (q) => q.eq("user_id", user_id))
      .order("desc")
      .filter((q) => q.neq(q.field("category"), "agent_working"))
      .take(limit);
    return rows.map((r) => r.time).reverse();
  },
});

export const countEvidence = serverQuery({
  args: { user_id: v.id("users"), hashes: v.array(v.string()) },
  handler: async (ctx, { user_id, hashes }) => {
    let matched = 0;
    for (const h of hashes) {
      const hit = await ctx.db
        .query("heartbeats")
        .withIndex("by_user_evidence", (q) => q.eq("user_id", user_id).eq("evidence_hash", h))
        .first();
      if (hit) matched++;
    }
    return matched;
  },
});
