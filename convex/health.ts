// SPDX-License-Identifier: AGPL-3.0-or-later
import { serverQuery } from "./lib";

export const ping = serverQuery({ args: {}, handler: async () => true });
