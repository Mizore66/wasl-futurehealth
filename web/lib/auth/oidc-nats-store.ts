import "server-only";

import { JetStreamApiError } from "@nats-io/jetstream";
import { Kvm, type KV, type KvEntry } from "@nats-io/kv";
import { connect, type NatsConnection, type NodeConnectionOptions } from "@nats-io/transport-node";
import type { SessionStoreConfig } from "./oidc-config";
import {
  isCredential,
  isOpaqueId,
  LOGIN_LIFETIME_MS,
  randomOpaqueId,
  SESSION_LIFETIME_MS,
  sha256,
  validateLoginInput,
  type CreateLoginInput,
  type CreateSessionInput,
  type LoginTransaction,
  type ReviewSession,
  type SessionResolution,
  type SessionStore,
} from "./oidc-store";

type NatsStoreConfig = Extract<SessionStoreConfig, { kind: "nats" }>;

export const LOGIN_BUCKET = "WASL_OIDC_LOGIN";
export const SESSION_BUCKET = "WASL_OIDC_SESSION";
/** Entries outlive their own expiry briefly so callers see "expired" rather than "anonymous"; the stream then removes them. */
const LOGIN_RETENTION_MS = LOGIN_LIFETIME_MS + 10 * 60 * 1000;
const SESSION_RETENTION_MS = SESSION_LIFETIME_MS + 24 * 60 * 60 * 1000;
/** JetStream "wrong last sequence": another replica changed the key first. */
const WRONG_LAST_SEQUENCE = 10071;
const MAX_CAS_ATTEMPTS = 8;

type LoginRecord = {
  v: 1;
  correlationHash: string;
  nonceHash: string;
  pkceVerifier: string;
  returnTo: string;
  expiresAt: number;
  consumedAt: number | null;
};

type SessionRecord = {
  v: 1;
  issuer: string;
  subject: string;
  credential: ReviewSession["credential"];
  createdAt: number;
  expiresAt: number;
  revokedAt: number | null;
};

const encoder = new TextEncoder();
const encode = (value: LoginRecord | SessionRecord): Uint8Array => encoder.encode(JSON.stringify(value));

function parse(entry: KvEntry | null): unknown {
  if (!entry || entry.operation !== "PUT") return undefined;
  try {
    return JSON.parse(entry.string());
  } catch {
    return undefined;
  }
}

const isTime = (value: unknown): value is number => typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
const isHash = (value: unknown): value is string => typeof value === "string" && /^[0-9a-f]{64}$/.test(value);

/** Records are validated on every read; anything malformed fails closed as an unknown transaction or session. */
function loginRecord(value: unknown): LoginRecord | undefined {
  const record = value as Partial<LoginRecord> | undefined;
  if (!record || record.v !== 1 || !isHash(record.correlationHash) || !isHash(record.nonceHash)) return undefined;
  if (typeof record.pkceVerifier !== "string" || record.pkceVerifier.length < 43 || typeof record.returnTo !== "string") return undefined;
  if (!isTime(record.expiresAt) || (record.consumedAt !== null && !isTime(record.consumedAt))) return undefined;
  return record as LoginRecord;
}

function sessionRecord(value: unknown): SessionRecord | undefined {
  const record = value as Partial<SessionRecord> | undefined;
  if (!record || record.v !== 1 || typeof record.issuer !== "string" || typeof record.subject !== "string" || !record.subject) return undefined;
  if (!isCredential(record.credential) || !isTime(record.createdAt) || !isTime(record.expiresAt)) return undefined;
  if (record.revokedAt !== null && !isTime(record.revokedAt)) return undefined;
  return record as SessionRecord;
}

const wrongLastSequence = (error: unknown): boolean => error instanceof JetStreamApiError && error.code === WRONG_LAST_SEQUENCE;

export function natsConnectionOptions(config: NatsStoreConfig): NodeConnectionOptions {
  return {
    servers: config.servers,
    name: "wasl-web-oidc-sessions",
    ...(config.user ? { user: config.user, pass: config.password } : {}),
    ...(config.tls ? { tls: { caFile: config.tls.caFile, ...(config.tls.certFile ? { certFile: config.tls.certFile, keyFile: config.tls.keyFile } : {}) } } : {}),
    timeout: 5_000,
    maxReconnectAttempts: -1,
  };
}

/**
 * Shared login-transaction and review-session storage on JetStream key-value buckets, for more than one web replica.
 *
 * Atomicity comes from JetStream's per-key compare-and-set (expected last subject sequence): a login is consumed by the
 * one update that names the revision it read, so concurrent callbacks on different replicas cannot both succeed.
 * Revocation is a compare-and-set write too, retried on conflict, and never cleared. Keys are SHA-256 hashes of state
 * and session IDs; the stored PKCE verifier is protected by the cluster's JetStream encryption at rest and NATS auth,
 * as the SQLite file is by file permissions.
 */
export class NatsSessionStore implements SessionStore {
  private constructor(
    private readonly connection: NatsConnection,
    private readonly logins: KV,
    private readonly sessions: KV,
  ) {}

  static async connect(config: NatsStoreConfig, onClosed?: () => void): Promise<NatsSessionStore> {
    const connection = await connect(natsConnectionOptions(config));
    try {
      const kvm = new Kvm(connection);
      /* Creation is idempotent for identical configuration, so any replica may create the buckets first. */
      const [logins, sessions] = await Promise.all([
        kvm.create(LOGIN_BUCKET, { history: 1, ttl: LOGIN_RETENTION_MS, replicas: config.replicas, storage: "file" }),
        kvm.create(SESSION_BUCKET, { history: 1, ttl: SESSION_RETENTION_MS, replicas: config.replicas, storage: "file" }),
      ]);
      void connection.closed().then(() => onClosed?.());
      return new NatsSessionStore(connection, logins, sessions);
    } catch (error) {
      await connection.close().catch(() => undefined);
      throw error;
    }
  }

  async createLogin(input: CreateLoginInput): Promise<LoginTransaction> {
    validateLoginInput(input);
    const { state, correlation, nonce, pkceVerifier, returnTo, now = Date.now() } = input;
    const transaction = { nonceHash: sha256(nonce), pkceVerifier, returnTo, expiresAt: now + LOGIN_LIFETIME_MS };
    const record: LoginRecord = { v: 1, correlationHash: sha256(correlation), ...transaction, consumedAt: null };
    /* create() refuses an existing key, so a state can never overwrite another login. */
    await this.logins.create(sha256(state), encode(record));
    return transaction;
  }

  async consumeLogin(state: string, correlation: string, now = Date.now()): Promise<LoginTransaction | undefined> {
    if (state.length < 32 || correlation.length < 32) return undefined;
    const entry = await this.logins.get(sha256(state));
    const record = loginRecord(parse(entry));
    if (!entry || !record || record.consumedAt !== null || record.expiresAt <= now) return undefined;
    /* A different browser does not consume the initiating login. */
    if (record.correlationHash !== sha256(correlation)) return undefined;
    try {
      await this.logins.update(sha256(state), encode({ ...record, consumedAt: now }), entry.revision);
    } catch (error) {
      if (wrongLastSequence(error)) return undefined;
      throw error;
    }
    return { nonceHash: record.nonceHash, pkceVerifier: record.pkceVerifier, returnTo: record.returnTo, expiresAt: record.expiresAt };
  }

  async createSession({ issuer, subject, credential, now = Date.now() }: CreateSessionInput): Promise<{ sessionId: string; session: ReviewSession }> {
    const sessionId = randomOpaqueId();
    const session = { issuer, subject, credential, createdAt: now, expiresAt: now + SESSION_LIFETIME_MS };
    await this.sessions.create(sha256(sessionId), encode({ v: 1, ...session, revokedAt: null }));
    return { sessionId, session };
  }

  async resolveSession(sessionId: string | undefined, now = Date.now()): Promise<SessionResolution> {
    if (!isOpaqueId(sessionId)) return { ok: false, reason: "anonymous" };
    const record = sessionRecord(parse(await this.sessions.get(sha256(sessionId))));
    if (!record) return { ok: false, reason: "anonymous" };
    if (record.revokedAt !== null) return { ok: false, reason: "revoked" };
    if (record.expiresAt <= now) return { ok: false, reason: "expired" };
    const { issuer, subject, credential, createdAt, expiresAt } = record;
    return { ok: true, session: { issuer, subject, credential, createdAt, expiresAt } };
  }

  /** Marks one stored session revoked; false when it is unknown or already revoked. Retries only on concurrent writes. */
  private async revokeKey(key: string, now: number, matches: (record: SessionRecord) => boolean = () => true): Promise<boolean> {
    for (let attempt = 0; attempt < MAX_CAS_ATTEMPTS; attempt++) {
      const entry = await this.sessions.get(key);
      const record = sessionRecord(parse(entry));
      if (!entry || !record || record.revokedAt !== null || !matches(record)) return false;
      try {
        await this.sessions.update(key, encode({ ...record, revokedAt: now }), entry.revision);
        return true;
      } catch (error) {
        if (!wrongLastSequence(error)) throw error;
      }
    }
    throw new Error("OIDC session revocation did not converge");
  }

  async revokeSession(sessionId: string | undefined, now = Date.now()): Promise<boolean> {
    if (!isOpaqueId(sessionId)) return false;
    return this.revokeKey(sha256(sessionId), now);
  }

  async revokeSubject(issuer: string, subject: string, now = Date.now()): Promise<number> {
    let revoked = 0;
    const keys: string[] = [];
    for await (const key of await this.sessions.keys()) keys.push(key);
    for (const key of keys) {
      if (await this.revokeKey(key, now, (record) => record.issuer === issuer && record.subject === subject)) revoked += 1;
    }
    return revoked;
  }

  async close(): Promise<void> {
    if (!this.connection.isClosed()) await this.connection.drain().catch(() => this.connection.close());
  }
}
