import "server-only";

import { isReviewToken } from "@/components/review/auth";

export const OIDC_CREDENTIALS = ["reviewer", "reviewer_2", "reviewer_3", "clinician", "clinician_2", "clinician_3"] as const;
export type OidcCredential = (typeof OIDC_CREDENTIALS)[number];
/**
 * "demo": each reviewer pastes an assigned bearer token. "persona": the open hosted demo, where any visitor picks a demo
 * persona and the web server signs with that persona's server-side token. "oidc": the identity provider decides.
 */
export type ReviewAuthMode = "demo" | "persona" | "oidc";
export type OidcEnvironment = Record<string, string | undefined>;

export type OidcConfig = {
  mode: "oidc";
  issuer: string;
  clientId: string;
  clientSecret: string;
  redirectUri: string;
  subjectMap: ReadonlyMap<string, OidcCredential>;
  sessionStore: SessionStoreConfig;
  insecureLoopback: boolean;
};

/** Local single-instance SQLite, or the shared JetStream key-value store used by more than one web replica. */
export type SessionStoreConfig =
  | { kind: "sqlite"; path: string }
  | {
      kind: "nats";
      servers: string[];
      user?: string;
      password?: string;
      tls?: { caFile: string; certFile?: string; keyFile?: string };
      replicas: 1 | 3 | 5;
    };

export class OidcConfigurationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "OidcConfigurationError";
  }
}

export function reviewAuthMode(env: OidcEnvironment = process.env): ReviewAuthMode {
  const mode = env.WASL_REVIEW_AUTH_MODE ?? "demo";
  if (mode !== "demo" && mode !== "persona" && mode !== "oidc") throw new OidcConfigurationError("WASL_REVIEW_AUTH_MODE must be demo, persona or oidc");
  return mode;
}

function required(env: OidcEnvironment, name: string): string {
  const value = env[name]?.trim();
  if (!value) throw new OidcConfigurationError(`${name} is required in OIDC mode`);
  return value;
}

function exactUrl(value: string, name: string): URL {
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new OidcConfigurationError(`${name} must be an absolute URL`);
  }
  if (url.username || url.password || url.search || url.hash) throw new OidcConfigurationError(`${name} must not contain credentials, a query or a fragment`);
  return url;
}

export function isLoopback(url: URL): boolean {
  return url.hostname === "localhost" || url.hostname === "127.0.0.1" || url.hostname === "[::1]" || url.hostname === "::1";
}

/** Parses the flat JSON object lexically as well so escaped duplicate subject keys cannot be hidden by JSON.parse. */
function subjectKeys(json: string): string[] {
  let at = 0;
  const space = () => {
    while (/\s/.test(json[at] ?? "")) at += 1;
  };
  const string = () => {
    space();
    const start = at;
    if (json[at] !== '"') throw new OidcConfigurationError("WASL_OIDC_SUBJECT_MAP_JSON must map subjects to credential strings");
    at += 1;
    let escaped = false;
    while (at < json.length) {
      const char = json[at++];
      if (escaped) escaped = false;
      else if (char === "\\") escaped = true;
      else if (char === '"') return JSON.parse(json.slice(start, at)) as string;
    }
    throw new OidcConfigurationError("WASL_OIDC_SUBJECT_MAP_JSON contains an unterminated string");
  };
  const keys: string[] = [];
  space();
  if (json[at++] !== "{") throw new OidcConfigurationError("WASL_OIDC_SUBJECT_MAP_JSON must be a JSON object");
  space();
  if (json[at] === "}") return keys;
  while (at < json.length) {
    keys.push(string());
    space();
    if (json[at++] !== ":") throw new OidcConfigurationError("WASL_OIDC_SUBJECT_MAP_JSON is malformed");
    string();
    space();
    if (json[at] === "}") return keys;
    if (json[at++] !== ",") throw new OidcConfigurationError("WASL_OIDC_SUBJECT_MAP_JSON is malformed");
  }
  throw new OidcConfigurationError("WASL_OIDC_SUBJECT_MAP_JSON is malformed");
}

export function parseSubjectMap(json: string): ReadonlyMap<string, OidcCredential> {
  let value: unknown;
  try {
    value = JSON.parse(json);
  } catch {
    throw new OidcConfigurationError("WASL_OIDC_SUBJECT_MAP_JSON must be valid JSON");
  }
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new OidcConfigurationError("WASL_OIDC_SUBJECT_MAP_JSON must be a JSON object");
  const keys = subjectKeys(json);
  if (keys.length === 0) throw new OidcConfigurationError("WASL_OIDC_SUBJECT_MAP_JSON must contain at least one subject");
  if (new Set(keys).size !== keys.length) throw new OidcConfigurationError("WASL_OIDC_SUBJECT_MAP_JSON contains a duplicate subject");
  const entries = Object.entries(value);
  if (entries.length !== keys.length) throw new OidcConfigurationError("WASL_OIDC_SUBJECT_MAP_JSON contains unknown fields");
  const map = new Map<string, OidcCredential>();
  const assigned = new Set<OidcCredential>();
  for (const [subject, credential] of entries) {
    if (subject.length === 0 || subject.length > 255 || subject.trim() !== subject || /[\u0000-\u001f\u007f]/.test(subject)) {
      throw new OidcConfigurationError("OIDC subjects must be non-empty, at most 255 characters and free of control characters or surrounding whitespace");
    }
    if (typeof credential !== "string" || !OIDC_CREDENTIALS.includes(credential as OidcCredential)) {
      throw new OidcConfigurationError("OIDC subject map contains an unknown credential");
    }
    if (assigned.has(credential as OidcCredential)) throw new OidcConfigurationError("Each OIDC credential may be assigned to only one subject");
    assigned.add(credential as OidcCredential);
    map.set(subject, credential as OidcCredential);
  }
  return map;
}

export function oidcConfig(env: OidcEnvironment = process.env): OidcConfig {
  if (reviewAuthMode(env) !== "oidc") throw new OidcConfigurationError("OIDC provider configuration requires WASL_REVIEW_AUTH_MODE=oidc");
  const issuer = required(env, "WASL_OIDC_ISSUER");
  const redirectUri = required(env, "WASL_OIDC_REDIRECT_URI");
  const issuerUrl = exactUrl(issuer, "WASL_OIDC_ISSUER");
  const redirectUrl = exactUrl(redirectUri, "WASL_OIDC_REDIRECT_URI");
  const insecureLoopback = env.WASL_OIDC_INSECURE_LOOPBACK === "1";
  if (env.WASL_OIDC_INSECURE_LOOPBACK && !insecureLoopback) throw new OidcConfigurationError("WASL_OIDC_INSECURE_LOOPBACK may only be 1");
  const loopback = insecureLoopback && env.WASL_SYNTHETIC_DATA_ONLY === "1" && isLoopback(issuerUrl) && isLoopback(redirectUrl);
  if (insecureLoopback && !loopback) throw new OidcConfigurationError("Insecure OIDC is limited to loopback synthetic testing");
  for (const [url, name] of [[issuerUrl, "WASL_OIDC_ISSUER"], [redirectUrl, "WASL_OIDC_REDIRECT_URI"]] as const) {
    if (url.protocol !== "https:" && !(loopback && url.protocol === "http:")) throw new OidcConfigurationError(`${name} must use HTTPS`);
  }
  const sessionStore = sessionStoreConfig(env, loopback);
  return {
    mode: "oidc",
    issuer,
    clientId: required(env, "WASL_OIDC_CLIENT_ID"),
    clientSecret: required(env, "WASL_OIDC_CLIENT_SECRET"),
    redirectUri,
    subjectMap: parseSubjectMap(required(env, "WASL_OIDC_SUBJECT_MAP_JSON")),
    sessionStore,
    insecureLoopback: loopback,
  };
}

function natsServer(value: string, loopback: boolean, tls: boolean): string {
  let url: URL;
  try {
    url = new URL(value);
  } catch {
    throw new OidcConfigurationError("WASL_OIDC_NATS_URL entries must be nats:// or tls:// URLs");
  }
  if ((url.protocol !== "nats:" && url.protocol !== "tls:") || !url.hostname || url.username || url.password || url.search || url.hash || (url.pathname && url.pathname !== "/")) {
    throw new OidcConfigurationError("WASL_OIDC_NATS_URL entries must be nats:// or tls:// host:port URLs without credentials, paths or queries");
  }
  /* Plaintext NATS is limited to the same loopback synthetic test mode as plaintext OIDC. */
  if (!tls && !(loopback && isLoopback(url))) throw new OidcConfigurationError("WASL_OIDC_NATS_TLS_CA is required unless NATS is loopback synthetic testing");
  return url.port ? `${url.hostname}:${url.port}` : url.hostname;
}

/** Shared session storage is opt-in; SQLite remains the default local and single-instance store. */
export function sessionStoreConfig(env: OidcEnvironment, loopback: boolean): SessionStoreConfig {
  const kind = env.WASL_OIDC_SESSION_STORE?.trim() || "sqlite";
  if (kind === "sqlite") return { kind, path: required(env, "WASL_OIDC_SESSION_DB") };
  if (kind !== "nats") throw new OidcConfigurationError("WASL_OIDC_SESSION_STORE must be sqlite or nats");
  const caFile = env.WASL_OIDC_NATS_TLS_CA?.trim();
  const certFile = env.WASL_OIDC_NATS_TLS_CERT?.trim();
  const keyFile = env.WASL_OIDC_NATS_TLS_KEY?.trim();
  if (Boolean(certFile) !== Boolean(keyFile)) throw new OidcConfigurationError("WASL_OIDC_NATS_TLS_CERT and WASL_OIDC_NATS_TLS_KEY must be set together");
  if (certFile && !caFile) throw new OidcConfigurationError("WASL_OIDC_NATS_TLS_CA is required with a NATS client certificate");
  const servers = required(env, "WASL_OIDC_NATS_URL")
    .split(",")
    .map((entry) => entry.trim())
    .filter(Boolean)
    .map((entry) => natsServer(entry, loopback, Boolean(caFile)));
  if (servers.length === 0) throw new OidcConfigurationError("WASL_OIDC_NATS_URL is required in OIDC mode");
  const user = env.WASL_OIDC_NATS_USER?.trim();
  const password = env.WASL_OIDC_NATS_PASSWORD;
  if (Boolean(user) !== Boolean(password)) throw new OidcConfigurationError("WASL_OIDC_NATS_USER and WASL_OIDC_NATS_PASSWORD must be set together");
  if (!user && !loopback) throw new OidcConfigurationError("WASL_OIDC_NATS_USER is required unless NATS is loopback synthetic testing");
  const replicasText = env.WASL_OIDC_NATS_REPLICAS?.trim() || "3";
  if (!["1", "3", "5"].includes(replicasText)) throw new OidcConfigurationError("WASL_OIDC_NATS_REPLICAS must be 1, 3 or 5");
  return {
    kind,
    servers,
    ...(user ? { user, password } : {}),
    ...(caFile ? { tls: { caFile, ...(certFile ? { certFile, keyFile } : {}) } } : {}),
    replicas: Number(replicasText) as 1 | 3 | 5,
  };
}

export function credentialVariable(credential: OidcCredential): string {
  return `WASL_API_${credential.toUpperCase()}_TOKEN`;
}

/** Resolves only the current server-side mapping. The credential itself never enters a route response. */
export function mappedCredentialToken(config: OidcConfig, subject: string, expectedCredential?: OidcCredential, env: OidcEnvironment = process.env): string | undefined {
  const credential = config.subjectMap.get(subject);
  if (!credential || (expectedCredential && credential !== expectedCredential)) return undefined;
  const token = env[credentialVariable(credential)]?.trim();
  return isReviewToken(token) ? token : undefined;
}
