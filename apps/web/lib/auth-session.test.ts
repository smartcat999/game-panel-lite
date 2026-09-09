import { describe, expect, it } from "vitest";
import { accountCacheKey } from "./auth-session";
import type { UserAccount } from "./types";

describe("account cache identity", () => {
  const account: UserAccount = { id: "alice", username: "Alice", role: "member", platformRole: "user" };
  it("separates accounts, roles and explicit permission changes", () => {
    const key = accountCacheKey(account);
    expect(accountCacheKey({ ...account, id: "bob" })).not.toBe(key);
    expect(accountCacheKey({ ...account, role: "viewer" })).not.toBe(key);
    expect(accountCacheKey({ ...account, platformRole: "platform_admin" })).not.toBe(key);
    expect(accountCacheKey({ ...account, permissions: [] })).not.toBe(key);
    expect(accountCacheKey({ ...account, permissions: ["server.view"] })).not.toBe(key);
  });
  it("does not reset a cache for a display name or permission order change", () => {
    expect(accountCacheKey({ ...account, username: "New Name" })).toBe(accountCacheKey(account));
    expect(accountCacheKey({ ...account, permissions: ["server.view", "server.create"] }))
      .toBe(accountCacheKey({ ...account, permissions: ["server.create", "server.view"] }));
  });
});
