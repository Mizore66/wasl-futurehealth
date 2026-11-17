import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { Kvm } from "@nats-io/kv";
import { connect } from "@nats-io/transport-node";
import type { SessionStoreConfig } from "@/lib/auth/oidc-config";
import { LOGIN_BUCKET, NatsSessionStore, SESSION_BUCKET } from "@/lib/auth/oidc-nats-store";
import { nonceForVerifier, randomOpaqueId, sha256, type SessionStore } from "@/lib/auth/oidc-store";
import { natsServerBinary, startNatsServer, type TestNatsServer } from "./support/nats-server";

/*
 * Two NatsSessionStore instances stand in for two web replicas: each has its own NATS connection and shares nothing
 * in-process, so every cross-instance assertion goes through the JetStream key-value buckets.
 */
const binary = natsServerBinary();
let server: TestNatsServer;
const open: NatsSessionStore[] = [];

const config = (): Extract<SessionStoreConfig, { kind: "nats" }> => ({ kind: "nats", servers: [`127.0.0.1:${server.port}`], replicas: 1 });
async function replica(): Promise<NatsSessionStore> {
  let lastError: unknown;
  /* JetStream can accept connections a moment before its API answers. */
  for (let attempt = 0; attempt < 50; attempt++) {
    try {
      const store = await NatsSessionStore.connect(config());
      open.push(store);
      return store;
    } catch (error) {
      lastError = error;
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
  }
  throw lastError;
}

const login = async (store: SessionStore, now = Date.now()) => {
  const state = randomOpaqueId();
  const verifier = randomOpaqueId() + randomOpaqueId();
  const nonce = nonceForVerifier(verifier);
  const correlation = randomOpaqueId();
  await store.createLogin({ state, correlation, nonce, pkceVerifier: verifier, returnTo: "/app/review/case", now });
  return { state, correlation, verifier, nonce };
};
const issuer = "https://id.example.test";

/** While the broker restarts or reconnects, store calls throw and route handlers fail closed; tests wait it out. */
async function untilAvailable<T>(operation: () => Promise<T>): Promise<T> {
  for (let attempt = 0; ; attempt++) {
    try {
      return await operation();
    } catch (error) {
      if (attempt >= 40) throw error;
      await new Promise((resolve) => setTimeout(resolve, 250));
    }
  }
}

describe.skipIf(!binary)("NATS shared OIDC session store", () => {
  beforeAll(async () => {
    server = await startNatsServer(binary!);
  });
  afterEach(async () => {
    while (open.length) await open.pop()!.close();
  });
  afterAll(async () => {
    await server?.close();
  });

  it("completes a login started on one replica exactly once on the other", async () => {
    const [a, b] = [await replica(), await replica()];
    const { state, correlation, verifier, nonce } = await login(a);
    const consumed = await b.consumeLogin(state, correlation);
    expect(consumed).toMatchObject({ pkceVerifier: verifier, nonceHash: sha256(nonce), returnTo: "/app/review/case" });
    expect(await b.consumeLogin(state, correlation)).toBeUndefined();
    expect(await a.consumeLogin(state, correlation)).toBeUndefined();
  });

  it("refuses a different browser without consuming the initiating login", async () => {
    const [a, b] = [await replica(), await replica()];
    const { state, correlation, verifier } = await login(a);
    expect(await b.consumeLogin(state, randomOpaqueId())).toBeUndefined();
    expect((await a.consumeLogin(state, correlation))?.pkceVerifier).toBe(verifier);
  });

  it("refuses unknown and expired states", async () => {
    const [a, b] = [await replica(), await replica()];
    const now = Date.now();
    const { state, correlation } = await login(a, now);
    expect(await b.consumeLogin(randomOpaqueId(), correlation, now + 1)).toBeUndefined();
    expect(await b.consumeLogin(state, correlation, now + 10 * 60 * 1000)).toBeUndefined();
  });

  it("lets exactly one of many concurrent callbacks across replicas consume a state", async () => {
    const replicas = [await replica(), await replica(), await replica()];
    for (let round = 0; round < 5; round++) {
      const { state, correlation } = await login(replicas[round % replicas.length]);
      const results = await Promise.all(Array.from({ length: 24 }, (_, index) => replicas[index % replicas.length].consumeLogin(state, correlation)));
      expect(results.filter(Boolean)).toHaveLength(1);
    }
  });

  it("shares sessions, logout and expiry across replicas", async () => {
    const [a, b] = [await replica(), await replica()];
    const now = Date.now();
    const { sessionId, session } = await a.createSession({ issuer, subject: "synthetic-r1", credential: "reviewer", now });
    expect(session.expiresAt - session.createdAt).toBe(8 * 60 * 60 * 1000);
    expect(await b.resolveSession(sessionId, now + 1)).toEqual({ ok: true, session });
    expect(await b.resolveSession(sessionId, session.expiresAt)).toEqual({ ok: false, reason: "expired" });
    expect(await a.resolveSession(sessionId, session.expiresAt)).toEqual({ ok: false, reason: "expired" });

    expect(await b.revokeSession(sessionId)).toBe(true);
    expect(await a.revokeSession(sessionId)).toBe(false);
    expect(await a.resolveSession(sessionId)).toEqual({ ok: false, reason: "revoked" });
    expect(await b.resolveSession(sessionId)).toEqual({ ok: false, reason: "revoked" });
  });

  it("records exactly one revocation when both replicas log the same session out at once", async () => {
    const [a, b] = [await replica(), await replica()];
    const { sessionId } = await a.createSession({ issuer, subject: "synthetic-r1", credential: "reviewer" });
    const results = await Promise.all([a.revokeSession(sessionId), b.revokeSession(sessionId), a.revokeSession(sessionId), b.revokeSession(sessionId)]);
    expect(results.filter(Boolean)).toHaveLength(1);
    expect(await a.resolveSession(sessionId)).toEqual({ ok: false, reason: "revoked" });
  });

  it("revokes every session of one subject without touching another subject", async () => {
    const [a, b] = [await replica(), await replica()];
    const one = await a.createSession({ issuer, subject: "synthetic-subject-wide", credential: "reviewer" });
    const two = await b.createSession({ issuer, subject: "synthetic-subject-wide", credential: "reviewer" });
    const other = await a.createSession({ issuer, subject: "synthetic-other", credential: "reviewer_2" });
    const foreignIssuer = await a.createSession({ issuer: "https://other.example.test", subject: "synthetic-subject-wide", credential: "reviewer" });
    expect(await b.revokeSubject(issuer, "synthetic-subject-wide")).toBe(2);
    expect(await a.resolveSession(one.sessionId)).toEqual({ ok: false, reason: "revoked" });
    expect(await a.resolveSession(two.sessionId)).toEqual({ ok: false, reason: "revoked" });
    expect((await b.resolveSession(other.sessionId)).ok).toBe(true);
    expect((await b.resolveSession(foreignIssuer.sessionId)).ok).toBe(true);
  });

  it("rejects malformed, unknown and tampered sessions", async () => {
    const a = await replica();
    expect(await a.resolveSession(undefined)).toEqual({ ok: false, reason: "anonymous" });
    expect(await a.resolveSession("short")).toEqual({ ok: false, reason: "anonymous" });
    expect(await a.resolveSession(randomOpaqueId())).toEqual({ ok: false, reason: "anonymous" });

    const connection = await connect({ servers: [`127.0.0.1:${server.port}`] });
    try {
      const sessions = await new Kvm(connection).open(SESSION_BUCKET);
      const forged = randomOpaqueId();
      await sessions.put(sha256(forged), JSON.stringify({ v: 1, issuer, subject: "synthetic-r1", credential: "auditor", createdAt: 0, expiresAt: Date.now() + 60_000, revokedAt: null }));
      expect(await a.resolveSession(forged)).toEqual({ ok: false, reason: "anonymous" });
      const garbage = randomOpaqueId();
      await sessions.put(sha256(garbage), "not json");
      expect(await a.resolveSession(garbage)).toEqual({ ok: false, reason: "anonymous" });
    } finally {
      await connection.close();
    }
  });

  it("stores only hashes of state, correlation, nonce and session IDs as keys and values", async () => {
    const a = await replica();
    const { state, correlation, nonce } = await login(a);
    const { sessionId } = await a.createSession({ issuer, subject: "synthetic-r1", credential: "reviewer" });
    const connection = await connect({ servers: [`127.0.0.1:${server.port}`] });
    try {
      const kvm = new Kvm(connection);
      const logins = await kvm.open(LOGIN_BUCKET);
      const sessions = await kvm.open(SESSION_BUCKET);
      const loginEntry = await logins.get(sha256(state));
      const sessionEntry = await sessions.get(sha256(sessionId));
      expect(loginEntry?.string()).toContain(sha256(correlation));
      for (const raw of [state, correlation, nonce, sessionId]) {
        expect(loginEntry?.string()).not.toContain(raw);
        expect(sessionEntry?.string()).not.toContain(raw);
      }
      const keys: string[] = [];
      for await (const key of await sessions.keys()) keys.push(key);
      expect(keys).toContain(sha256(sessionId));
      expect(keys).not.toContain(sessionId);
      expect(sessionEntry?.string()).not.toMatch(/token|secret/i);
    } finally {
      await connection.close();
    }
  });

  it("preserves sessions, revocations and consumed logins across a broker restart and reconnects open replicas", async () => {
    const a = await replica();
    const valid = await a.createSession({ issuer, subject: "synthetic-r1", credential: "reviewer" });
    const revoked = await a.createSession({ issuer, subject: "synthetic-r2", credential: "reviewer_2" });
    await a.revokeSession(revoked.sessionId);
    const pending = await login(a);
    const used = await login(a);
    expect(await a.consumeLogin(used.state, used.correlation)).toBeDefined();

    await server.restart();

    /* A web replica restarted with the broker sees the same state once JetStream has recovered. */
    const restarted = await replica();
    await untilAvailable(() => restarted.resolveSession(valid.sessionId));
    expect(await restarted.resolveSession(valid.sessionId)).toEqual({ ok: true, session: valid.session });
    expect(await restarted.resolveSession(revoked.sessionId)).toEqual({ ok: false, reason: "revoked" });
    expect(await restarted.consumeLogin(used.state, used.correlation)).toBeUndefined();
    expect((await restarted.consumeLogin(pending.state, pending.correlation))?.pkceVerifier).toBe(pending.verifier);
    /* The replica that stayed up reconnects on its own. */
    expect(await untilAvailable(() => a.resolveSession(valid.sessionId))).toEqual({ ok: true, session: valid.session });
  }, 30_000);
});
