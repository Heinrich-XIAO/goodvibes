// SPDX-License-Identifier: AGPL-3.0-or-later
import { v } from "convex/values";
import { Id } from "./_generated/dataModel";
import { QueryCtx } from "./_generated/server";
import { publicUser, serverMutation, serverQuery } from "./lib";

async function byIdent(ctx: QueryCtx, ident: string) {
  const byName = await ctx.db
    .query("users")
    .withIndex("by_username_lower", (q) => q.eq("username_lower", ident.toLowerCase()))
    .first();
  if (byName) return byName;
  if (ident !== "") {
    const bySlack = await ctx.db
      .query("users")
      .withIndex("by_slack_uid", (q) => q.eq("slack_uid", ident))
      .first();
    if (bySlack) return bySlack;
  }
  const id = ctx.db.normalizeId("users", ident);
  return id ? await ctx.db.get(id) : null;
}

export const current = serverQuery({
  args: { api_key: v.string(), session_token: v.string(), now: v.number() },
  handler: async (ctx, { api_key, session_token, now }) => {
    let userId: Id<"users"> | null = null;
    if (api_key) {
      const key = await ctx.db
        .query("api_keys")
        .withIndex("by_token", (q) => q.eq("token", api_key))
        .unique();
      userId = key?.user_id ?? null;
    }
    if (!userId && session_token) {
      const s = await ctx.db
        .query("sessions")
        .withIndex("by_token", (q) => q.eq("token", session_token))
        .unique();
      if (s && s.expires_at > now) userId = s.user_id;
    }
    return userId ? publicUser(await ctx.db.get(userId)) : null;
  },
});

export const get = serverQuery({
  args: { ident: v.string() },
  handler: async (ctx, { ident }) => publicUser(await byIdent(ctx, ident)),
});

export const upsertHackClub = serverMutation({
  args: {
    hackclub_id: v.string(),
    email: v.string(),
    display_name: v.string(),
    slack_uid: v.string(),
    verification_status: v.string(),
    ysws_eligible: v.boolean(),
    admin_level: v.number(),
    username_base: v.string(),
    api_key: v.string(),
  },
  handler: async (ctx, a) => {
    const existing = await ctx.db
      .query("users")
      .withIndex("by_hackclub_id", (q) => q.eq("hackclub_id", a.hackclub_id))
      .unique();
    if (existing) {
      await ctx.db.patch(existing._id, {
        display_name: a.display_name,
        slack_uid: a.slack_uid || existing.slack_uid,
        verification_status: a.verification_status,
        ysws_eligible: a.ysws_eligible,
        admin_level: Math.max(existing.admin_level, a.admin_level),
      });
      return publicUser(await ctx.db.get(existing._id));
    }

    let username = a.username_base;
    for (let i = 1; await byIdent(ctx, username); i++) {
      username = `${a.username_base}${i}`;
    }
    const id = await ctx.db.insert("users", {
      hackclub_id: a.hackclub_id,
      username,
      username_lower: username.toLowerCase(),
      display_name: a.display_name,
      email: a.email,
      slack_uid: a.slack_uid,
      github_uid: "",
      github_username: "",
      timezone: "UTC",
      country_code: "",
      verification_status: a.verification_status,
      ysws_eligible: a.ysws_eligible,
      bio: "",
      allow_public_stats_lookup: true,
      default_timezone_leaderboard: true,
      trust_level: 0,
      admin_level: a.admin_level,
      hackatime_extension_text_type: 0,
    });
    await ctx.db.insert("api_keys", { user_id: id, name: "default", token: a.api_key });
    return publicUser(await ctx.db.get(id));
  },
});

export const createSession = serverMutation({
  args: { user_id: v.id("users"), token: v.string(), expires_at: v.number() },
  handler: async (ctx, a) => {
    await ctx.db.insert("sessions", a);
  },
});

export const deleteSession = serverMutation({
  args: { token: v.string() },
  handler: async (ctx, { token }) => {
    const s = await ctx.db
      .query("sessions")
      .withIndex("by_token", (q) => q.eq("token", token))
      .unique();
    if (s) await ctx.db.delete(s._id);
  },
});

export const createApiKey = serverMutation({
  args: { user_id: v.id("users"), name: v.string(), token: v.string() },
  handler: async (ctx, a) => {
    await ctx.db.insert("api_keys", a);
  },
});

export const updateSettings = serverMutation({
  args: {
    user_id: v.id("users"),
    allow_public_stats_lookup: v.optional(v.boolean()),
    bio: v.optional(v.string()),
    timezone: v.optional(v.string()),
  },
  handler: async (ctx, { user_id, ...patch }) => {
    await ctx.db.patch(user_id, patch);
    return publicUser(await ctx.db.get(user_id));
  },
});

export const setTrustLevel = serverMutation({
  args: {
    user_id: v.id("users"),
    changed_by_id: v.id("users"),
    previous_trust_level: v.string(),
    new_trust_level: v.string(),
    trust_level: v.number(),
    reason: v.string(),
    notes: v.string(),
  },
  handler: async (ctx, { trust_level, ...log }) => {
    await ctx.db.insert("trust_level_audit_logs", log);
    await ctx.db.patch(log.user_id, { trust_level });
  },
});
