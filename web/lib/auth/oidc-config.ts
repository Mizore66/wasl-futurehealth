import "server-only";

import { isReviewToken } from "@/components/review/auth";

export const OIDC_CREDENTIALS = ["reviewer", "reviewer_2", "reviewer_3", "clinician", "clinician_2", "clinician_3"] as const;
export type OidcCredential = (typeof OIDC_CREDENTIALS)[number];
export type ReviewAuthMode = "demo" | "oidc";
export type OidcEnvironment = Record<string, string | undefined>;

export type OidcConfig = {
  mode: "oidc";
  issuer: string;
  clientId: string;
  clientSecret: string;
  redirectUri: string;
  subjectMap: ReadonlyMap<string, OidcCredential>;
  sessionDb: string;
  insecureLoopback: boolean;
};

export class OidcConfigurationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "OidcConfigurationError";
  }
}

export function reviewAuthMode(env: OidcEnvironment = process.env): ReviewAuthMode {
  const mode = env.WASL_REVIEW_AUTH_MODE ?? "demo";
  if (mode !== "demo" && mode !== "oidc") throw new OidcConfigurationError("WASL_REVIEW_AUTH_MODE must be demo or oidc");
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
  const sessionDb = required(env, "WASL_OIDC_SESSION_DB");
  return {
    mode: "oidc",
    issuer,
    clientId: required(env, "WASL_OIDC_CLIENT_ID"),
    clientSecret: required(env, "WASL_OIDC_CLIENT_SECRET"),
    redirectUri,
    subjectMap: parseSubjectMap(required(env, "WASL_OIDC_SUBJECT_MAP_JSON")),
    sessionDb,
    insecureLoopback: loopback,
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
