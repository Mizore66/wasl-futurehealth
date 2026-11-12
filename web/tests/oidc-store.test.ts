import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { nonceForVerifier, randomOpaqueId, sha256, SqliteSessionStore } from "@/lib/auth/oidc-store";

const directories: string[] = [];
const stores: SqliteSessionStore[] = [];
function database() {
  const directory = mkdtempSync(path.join(tmpdir(), "wasl-oidc-store-"));
  directories.push(directory);
  return path.join(directory, "sessions.sqlite");
}
function open(file = ":memory:") {
  const store = new SqliteSessionStore(file);
  stores.push(store);
  return store;
}
afterEach(() => {
  while (stores.length) stores.pop()!.close();
  while (directories.length) rmSync(directories.pop()!, { recursive: true, force: true });
});

const login = (store: SqliteSessionStore, now = 1_000) => {
  const state = randomOpaqueId();
  const verifier = randomOpaqueId() + randomOpaqueId();
  const nonce = nonceForVerifier(verifier);
  const correlation = randomOpaqueId();
  store.createLogin({ state, correlation, nonce, pkceVerifier: verifier, returnTo: "/app/review/case", now });
  return { state, correlation, verifier, nonce };
};

describe("SQLite OIDC login transactions", () => {
  it("consumes a valid state and browser correlation atomically once", () => {
    const store = open();
    const { state, correlation, verifier, nonce } = login(store);
    const consumed = store.consumeLogin(state, correlation, 1_001);
    expect(consumed?.pkceVerifier).toBe(verifier);
    expect(consumed?.nonceHash).toBe(sha256(nonce));
    expect(store.consumeLogin(state, correlation, 1_002)).toBeUndefined();
  });

  it("rejects a different browser without consuming the initiating login", () => {
    const store = open();
    const { state, correlation, verifier } = login(store);
    expect(store.consumeLogin(state, randomOpaqueId(), 1_001)).toBeUndefined();
    expect(store.consumeLogin(state, correlation, 1_002)?.pkceVerifier).toBe(verifier);
  });

  it("rejects wrong and expired states", () => {
    const store = open();
    const { state, correlation } = login(store);
    expect(store.consumeLogin(randomOpaqueId(), correlation, 1_001)).toBeUndefined();
    expect(store.consumeLogin(state, correlation, 1_000 + 10 * 60 * 1000)).toBeUndefined();
  });

  it("persists hashes rather than raw state, correlation, nonce or session IDs", () => {
    const file = database();
    const store = open(file);
    const { state, correlation, nonce } = login(store);
    const { sessionId } = store.createSession({ issuer: "https://id.example", subject: "synthetic-r1", credential: "reviewer", now: 1_000 });
    store.close();
    stores.pop();
    const bytes = readFileSync(file).toString("utf8");
    expect(bytes).not.toContain(state);
    expect(bytes).not.toContain(correlation);
    expect(bytes).not.toContain(nonce);
    expect(bytes).not.toContain(sessionId);
    expect(bytes).toContain(sha256(state));
    expect(bytes).toContain(sha256(sessionId));
  });
});

describe("SQLite review sessions", () => {
  it("has a fixed eight-hour expiry and survives restart", () => {
    const file = database();
    const first = open(file);
    const { sessionId, session } = first.createSession({ issuer: "https://id.example", subject: "synthetic-r1", credential: "reviewer", now: 10_000 });
    expect(session.expiresAt - session.createdAt).toBe(8 * 60 * 60 * 1000);
    first.close();
    stores.pop();
    const second = open(file);
    expect(second.resolveSession(sessionId, 20_000)).toMatchObject({ ok: true, session: { subject: "synthetic-r1", credential: "reviewer" } });
    expect(second.resolveSession(sessionId, session.expiresAt)).toEqual({ ok: false, reason: "expired" });
    expect(second.resolveSession(sessionId, session.expiresAt + 100_000)).toEqual({ ok: false, reason: "expired" });
  });

  it("revokes an old cookie and preserves revocation after restart", () => {
    const file = database();
    const first = open(file);
    const { sessionId } = first.createSession({ issuer: "https://id.example", subject: "synthetic-r1", credential: "reviewer" });
    expect(first.revokeSession(sessionId)).toBe(true);
    expect(first.revokeSession(sessionId)).toBe(false);
    expect(first.resolveSession(sessionId)).toEqual({ ok: false, reason: "revoked" });
    first.close();
    stores.pop();
    expect(open(file).resolveSession(sessionId)).toEqual({ ok: false, reason: "revoked" });
  });

  it("supports subject-wide revocation without revoking another subject", () => {
    const store = open();
    const one = store.createSession({ issuer: "https://id.example", subject: "synthetic-r1", credential: "reviewer" });
    const two = store.createSession({ issuer: "https://id.example", subject: "synthetic-r2", credential: "reviewer_2" });
    expect(store.revokeSubject("https://id.example", "synthetic-r1")).toBe(1);
    expect(store.resolveSession(one.sessionId)).toEqual({ ok: false, reason: "revoked" });
    expect(store.resolveSession(two.sessionId).ok).toBe(true);
  });

  it("rejects malformed and unknown opaque cookies", () => {
    const store = open();
    expect(store.resolveSession(undefined)).toEqual({ ok: false, reason: "anonymous" });
    expect(store.resolveSession("short")).toEqual({ ok: false, reason: "anonymous" });
    expect(store.resolveSession(randomOpaqueId())).toEqual({ ok: false, reason: "anonymous" });
  });
});
