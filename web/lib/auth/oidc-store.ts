import "server-only";

import { createHash, randomBytes } from "node:crypto";
import { chmodSync, mkdirSync } from "node:fs";
import path from "node:path";
import { DatabaseSync } from "node:sqlite";
import type { OidcCredential } from "./oidc-config";

export const LOGIN_LIFETIME_MS = 10 * 60 * 1000;
export const SESSION_LIFETIME_MS = 8 * 60 * 60 * 1000;

export type LoginTransaction = {
  nonceHash: string;
  pkceVerifier: string;
  returnTo: string;
  expiresAt: number;
};

export type ReviewSession = {
  issuer: string;
  subject: string;
  credential: OidcCredential;
  createdAt: number;
  expiresAt: number;
};

export type SessionResolution =
  | { ok: true; session: ReviewSession }
  | { ok: false; reason: "anonymous" | "expired" | "revoked" };

export interface SessionStore {
  createLogin(input: { state: string; correlation: string; nonce: string; pkceVerifier: string; returnTo: string; now?: number }): LoginTransaction;
  consumeLogin(state: string, correlation: string, now?: number): LoginTransaction | undefined;
  createSession(input: { issuer: string; subject: string; credential: OidcCredential; now?: number }): { sessionId: string; session: ReviewSession };
  resolveSession(sessionId: string | undefined, now?: number): SessionResolution;
  revokeSession(sessionId: string | undefined): boolean;
  revokeSubject(issuer: string, subject: string): number;
  close(): void;
}

export const sha256 = (value: string): string => createHash("sha256").update(value).digest("hex");
export const randomOpaqueId = (): string => randomBytes(32).toString("base64url");

/**
 * The nonce is deterministically derived from the high-entropy, server-held PKCE verifier. This preserves 256 bits of
 * unpredictability without persisting a raw nonce; only its hash is stored, and the verifier reconstructs it at callback.
 */
export const nonceForVerifier = (verifier: string): string => createHash("sha256").update(`wasl-oidc-nonce:${verifier}`).digest("base64url");

type LoginRow = { nonce_hash: string; pkce_verifier: string; return_to: string; expires_at: number };
type SessionRow = {
  issuer: string;
  subject: string;
  credential: OidcCredential;
  created_at: number;
  expires_at: number;
  revoked_at: number | null;
};

/** Single-instance SQLite persistence. A shared deployment needs a separately verified shared SessionStore. */
export class SqliteSessionStore implements SessionStore {
  private readonly db: DatabaseSync;

  constructor(databasePath: string) {
    if (databasePath !== ":memory:") {
      mkdirSync(path.dirname(path.resolve(databasePath)), { recursive: true, mode: 0o700 });
    }
    this.db = new DatabaseSync(databasePath);
    if (databasePath !== ":memory:") chmodSync(databasePath, 0o600);
    this.db.exec("PRAGMA busy_timeout = 5000");
    if (databasePath !== ":memory:") this.db.exec("PRAGMA journal_mode = WAL");
    this.db.exec(`
      CREATE TABLE IF NOT EXISTS login_transaction (
        state_hash TEXT PRIMARY KEY,
        correlation_hash TEXT NOT NULL,
        nonce_hash TEXT NOT NULL,
        pkce_verifier TEXT NOT NULL,
        return_to TEXT NOT NULL,
        expires_at INTEGER NOT NULL,
        consumed_at INTEGER
      );
      CREATE TABLE IF NOT EXISTS review_session (
        session_id_hash TEXT PRIMARY KEY,
        issuer TEXT NOT NULL,
        subject TEXT NOT NULL,
        credential TEXT NOT NULL,
        created_at INTEGER NOT NULL,
        expires_at INTEGER NOT NULL,
        revoked_at INTEGER
      );
      CREATE INDEX IF NOT EXISTS review_session_subject ON review_session(issuer, subject);
    `);
    const loginColumns = this.db.prepare("PRAGMA table_info(login_transaction)").all() as { name: string }[];
    if (!loginColumns.some((column) => column.name === "correlation_hash")) {
      /* Existing transactions have no browser binding and therefore fail closed after this additive migration. */
      this.db.exec("ALTER TABLE login_transaction ADD COLUMN correlation_hash TEXT");
    }
  }

  createLogin({ state, correlation, nonce, pkceVerifier, returnTo, now = Date.now() }: { state: string; correlation: string; nonce: string; pkceVerifier: string; returnTo: string; now?: number }): LoginTransaction {
    if (state.length < 32 || correlation.length < 32 || nonce.length < 32 || pkceVerifier.length < 43) throw new Error("OIDC login parameters are too short");
    const transaction = { nonceHash: sha256(nonce), pkceVerifier, returnTo, expiresAt: now + LOGIN_LIFETIME_MS };
    this.db.prepare("DELETE FROM login_transaction WHERE expires_at <= ?").run(now);
    this.db
      .prepare("INSERT INTO login_transaction (state_hash, correlation_hash, nonce_hash, pkce_verifier, return_to, expires_at, consumed_at) VALUES (?, ?, ?, ?, ?, ?, NULL)")
      .run(sha256(state), sha256(correlation), transaction.nonceHash, pkceVerifier, returnTo, transaction.expiresAt);
    return transaction;
  }

  consumeLogin(state: string, correlation: string, now = Date.now()): LoginTransaction | undefined {
    this.db.exec("BEGIN IMMEDIATE");
    try {
      const row = this.db
        .prepare("SELECT nonce_hash, pkce_verifier, return_to, expires_at FROM login_transaction WHERE state_hash = ? AND correlation_hash = ? AND consumed_at IS NULL AND expires_at > ?")
        .get(sha256(state), sha256(correlation), now) as LoginRow | undefined;
      if (!row) {
        this.db.exec("COMMIT");
        return undefined;
      }
      const consumed = this.db
        .prepare("UPDATE login_transaction SET consumed_at = ? WHERE state_hash = ? AND correlation_hash = ? AND consumed_at IS NULL AND expires_at > ?")
        .run(now, sha256(state), sha256(correlation), now);
      if (consumed.changes !== 1) throw new Error("OIDC login transaction was already consumed");
      this.db.exec("COMMIT");
      return { nonceHash: row.nonce_hash, pkceVerifier: row.pkce_verifier, returnTo: row.return_to, expiresAt: row.expires_at };
    } catch (error) {
      this.db.exec("ROLLBACK");
      throw error;
    }
  }

  createSession({ issuer, subject, credential, now = Date.now() }: { issuer: string; subject: string; credential: OidcCredential; now?: number }): { sessionId: string; session: ReviewSession } {
    const sessionId = randomOpaqueId();
    const session = { issuer, subject, credential, createdAt: now, expiresAt: now + SESSION_LIFETIME_MS };
    this.db
      .prepare("INSERT INTO review_session (session_id_hash, issuer, subject, credential, created_at, expires_at, revoked_at) VALUES (?, ?, ?, ?, ?, ?, NULL)")
      .run(sha256(sessionId), issuer, subject, credential, session.createdAt, session.expiresAt);
    return { sessionId, session };
  }

  resolveSession(sessionId: string | undefined, now = Date.now()): SessionResolution {
    if (!sessionId || !/^[A-Za-z0-9_-]{43}$/.test(sessionId)) return { ok: false, reason: "anonymous" };
    const row = this.db
      .prepare("SELECT issuer, subject, credential, created_at, expires_at, revoked_at FROM review_session WHERE session_id_hash = ?")
      .get(sha256(sessionId)) as SessionRow | undefined;
    if (!row) return { ok: false, reason: "anonymous" };
    if (row.revoked_at !== null) return { ok: false, reason: "revoked" };
    if (row.expires_at <= now) return { ok: false, reason: "expired" };
    return { ok: true, session: { issuer: row.issuer, subject: row.subject, credential: row.credential, createdAt: row.created_at, expiresAt: row.expires_at } };
  }

  revokeSession(sessionId: string | undefined): boolean {
    if (!sessionId || !/^[A-Za-z0-9_-]{43}$/.test(sessionId)) return false;
    const result = this.db.prepare("UPDATE review_session SET revoked_at = ? WHERE session_id_hash = ? AND revoked_at IS NULL").run(Date.now(), sha256(sessionId));
    return result.changes === 1;
  }

  revokeSubject(issuer: string, subject: string): number {
    return Number(this.db.prepare("UPDATE review_session SET revoked_at = ? WHERE issuer = ? AND subject = ? AND revoked_at IS NULL").run(Date.now(), issuer, subject).changes);
  }

  close(): void {
    this.db.close();
  }
}

let shared: { path: string; store: SqliteSessionStore } | undefined;
export function defaultSessionStore(databasePath: string): SessionStore {
  if (!shared || shared.path !== databasePath) {
    shared?.store.close();
    shared = { path: databasePath, store: new SqliteSessionStore(databasePath) };
  }
  return shared.store;
}
