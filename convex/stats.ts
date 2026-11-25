// SPDX-License-Identifier: AGPL-3.0-or-later
import { v } from "convex/values";
import { Id } from "./_generated/dataModel";
import { publicUser, serverQuery } from "./lib";

const rollupOut = v.object({
  day: v.string(),
  project: v.string(),
  category: v.string(),
  human_seconds: v.number(),
  agent_seconds: v.number(),
  struggle_sum: v.number(),
  struggle_count: v.number(),
  heartbeats: v.number(),
});

// Rollup rows for one user, or every user when user_id is omitted,
// with day in [since_day, until_day].
export const rollups = serverQuery({
  args: { user_id: v.optional(v.id("users")), since_day: v.string(), until_day: v.string() },
  returns: v.array(rollupOut),
  handler: async (ctx, { user_id, since_day, until_day }) => {
    const rows = user_id
      ? await ctx.db
          .query("daily_rollups")
          .withIndex("by_key", (q) => q.eq("user_id", user_id).gte("day", since_day).lte("day", until_day))
          .collect()
      : await ctx.db
          .query("daily_rollups")
          .withIndex("by_day", (q) => q.gte("day", since_day).lte("day", until_day))
          .collect();
    return rows.map(({ _id, _creationTime, user_id: _u, ...r }) => r);
  },
});

export const leaderboard = serverQuery({
  args: { since_day: v.string(), limit: v.number() },
  handler: async (ctx, { since_day, limit }) => {
    const rows = await ctx.db
      .query("daily_rollups")
      .withIndex("by_day", (q) => q.gte("day", since_day))
      .collect();
    const totals = new Map<Id<"users">, { human: number; agent: number; sSum: number; sCount: number; projects: Set<string> }>();
    for (const r of rows) {
      const t = totals.get(r.user_id) ?? { human: 0, agent: 0, sSum: 0, sCount: 0, projects: new Set<string>() };
      t.human += r.human_seconds;
      t.agent += r.agent_seconds;
      t.sSum += r.struggle_sum;
      t.sCount += r.struggle_count;
      if (r.project) t.projects.add(r.project);
      totals.set(r.user_id, t);
    }

    const ranked = [...totals.entries()].sort((a, b) => b[1].human - a[1].human);
    const out = [];
    const seen = new Set<Id<"users">>();
    for (const [userId, t] of ranked) {
      if (out.length >= limit) break;
      const u = publicUser(await ctx.db.get(userId));
      if (!u || !u.allow_public_stats_lookup) continue;
      seen.add(userId);
      out.push({
        username: u.username,
        display_name: u.display_name,
        human_seconds: t.human,
        agent_seconds: t.agent,
        avg_struggle: t.sCount ? t.sSum / t.sCount : 0,
        project_count: t.projects.size,
      });
    }
    if (out.length < limit) {
      const newest = await ctx.db
        .query("users")
        .withIndex("by_public", (q) => q.eq("allow_public_stats_lookup", true))
        .order("desc")
        .take(limit);
      for (const u of newest) {
        if (out.length >= limit) break;
        if (seen.has(u._id)) continue;
        out.push({ username: u.username, display_name: u.display_name, human_seconds: 0, agent_seconds: 0, avg_struggle: 0, project_count: 0 });
      }
    }
    return out;
  },
});
