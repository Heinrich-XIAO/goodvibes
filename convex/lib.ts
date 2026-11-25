// SPDX-License-Identifier: AGPL-3.0-or-later
import { customMutation, customQuery } from "convex-helpers/server/customFunctions";
import { v } from "convex/values";
import { Doc } from "./_generated/dataModel";
import { mutation, query } from "./_generated/server";

// Every function is public on the Convex URL, so each one requires the
// shared secret that only the Go server holds.
function assertServer(secret: string) {
  const expected = process.env.GOODVIBES_SERVER_SECRET ?? "";
  let diff = expected.length === 0 || secret.length !== expected.length ? 1 : 0;
  for (let i = 0; i < Math.min(secret.length, expected.length); i++) {
    diff |= secret.charCodeAt(i) ^ expected.charCodeAt(i);
  }
  if (diff !== 0) throw new Error("unauthorized");
}

const guard = {
  args: { secret: v.string() },
  input: async (_ctx: unknown, { secret }: { secret: string }) => {
    assertServer(secret);
    return { ctx: {}, args: {} };
  },
};

export const serverQuery = customQuery(query, guard);
export const serverMutation = customMutation(mutation, guard);

export function publicUser(u: Doc<"users"> | null) {
  if (!u) return null;
  const { _id, _creationTime, email, username_lower, ...rest } = u;
  return { id: _id, ...rest };
}

export function utcDay(unixSeconds: number) {
  return new Date(unixSeconds * 1000).toISOString().slice(0, 10);
}
