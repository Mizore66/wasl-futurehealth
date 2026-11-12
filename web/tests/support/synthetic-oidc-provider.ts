import { createHash, randomBytes } from "node:crypto";
import { createServer, type Server } from "node:http";
import { exportJWK, generateKeyPair, SignJWT, type CryptoKey, type JWK } from "jose";

export type SyntheticOidcOverrides = {
  discoveryIssuer?: string;
  issuer?: string;
  audience?: string;
  azp?: string;
  nonce?: string;
  signingKey?: "unknown";
  tamperToken?: boolean;
  expiresIn?: number;
};

type AuthorizationCode = {
  clientId: string;
  redirectUri: string;
  subject: string;
  nonce: string;
  challenge: string;
  used: boolean;
};

export type SyntheticOidcProvider = {
  issuer: string;
  clientId: string;
  clientSecret: string;
  redirectUri: string;
  subject: string;
  overrides: SyntheticOidcOverrides;
  tokenRequests: number;
  authorizationRequests: number;
  close(): Promise<void>;
};

const body = async (request: import("node:http").IncomingMessage): Promise<URLSearchParams> => {
  let raw = "";
  for await (const chunk of request) raw += chunk;
  return new URLSearchParams(raw);
};

/** Loopback-only authorization server for protocol tests. It never represents a real identity provider or person. */
export async function startSyntheticOidcProvider(options: { clientId?: string; clientSecret?: string; redirectUri?: string; subject?: string } = {}): Promise<SyntheticOidcProvider> {
  const keys = await generateKeyPair("RS256", { modulusLength: 2048, extractable: true });
  const unknownKeys = await generateKeyPair("RS256", { modulusLength: 2048 });
  const kid = "synthetic-rsa-1";
  const publicJwk: JWK = { ...(await exportJWK(keys.publicKey as CryptoKey)), kid, alg: "RS256", use: "sig" };
  const codes = new Map<string, AuthorizationCode>();
  const clientId = options.clientId ?? "synthetic-review-client";
  const clientSecret = options.clientSecret ?? "synthetic-client-secret-do-not-deploy";
  const redirectUri = options.redirectUri ?? "http://127.0.0.1:3298/app/auth/oidc/callback";
  let issuer = "";
  let activeSubject = options.subject ?? "synthetic-r1";
  const overrides: SyntheticOidcOverrides = {};
  let tokenRequests = 0;
  let authorizationRequests = 0;

  const json = (response: import("node:http").ServerResponse, status: number, value: unknown) => {
    response.writeHead(status, { "content-type": "application/json", "cache-control": "no-store" });
    response.end(JSON.stringify(value));
  };

  const server: Server = createServer(async (request, response) => {
    const url = new URL(request.url ?? "/", issuer || "http://127.0.0.1");
    if (request.method === "GET" && url.pathname === "/.well-known/openid-configuration") {
      return json(response, 200, {
        issuer: overrides.discoveryIssuer ?? issuer,
        authorization_endpoint: `${issuer}/authorize`,
        token_endpoint: `${issuer}/token`,
        jwks_uri: `${issuer}/jwks`,
        response_types_supported: ["code"],
        subject_types_supported: ["public"],
        id_token_signing_alg_values_supported: ["RS256"],
        code_challenge_methods_supported: ["S256"],
        token_endpoint_auth_methods_supported: ["client_secret_basic"],
      });
    }
    if (request.method === "GET" && url.pathname === "/jwks") return json(response, 200, { keys: [publicJwk] });
    if (request.method === "GET" && url.pathname === "/authorize") {
      authorizationRequests += 1;
      const required = {
        response_type: "code",
        client_id: clientId,
        redirect_uri: redirectUri,
        scope: "openid",
        code_challenge_method: "S256",
      };
      if (Object.entries(required).some(([key, value]) => url.searchParams.get(key) !== value)) return json(response, 400, { error: "invalid_request" });
      const state = url.searchParams.get("state");
      const nonce = url.searchParams.get("nonce");
      const challenge = url.searchParams.get("code_challenge");
      if (!state || !nonce || !challenge || ![...url.searchParams.keys()].every((key) => ["response_type", "client_id", "redirect_uri", "scope", "state", "nonce", "code_challenge", "code_challenge_method"].includes(key))) {
        return json(response, 400, { error: "invalid_request" });
      }
      const code = randomBytes(32).toString("base64url");
      codes.set(code, { clientId, redirectUri, subject: activeSubject, nonce, challenge, used: false });
      const callback = new URL(redirectUri);
      callback.searchParams.set("code", code);
      callback.searchParams.set("state", state);
      callback.searchParams.set("iss", overrides.discoveryIssuer ?? issuer);
      response.writeHead(302, { location: callback.href, "cache-control": "no-store" });
      return response.end();
    }
    if (request.method === "POST" && url.pathname === "/token") {
      tokenRequests += 1;
      const parameters = await body(request);
      const basic = Buffer.from(request.headers.authorization?.replace(/^Basic /, "") ?? "", "base64").toString("utf8");
      const [basicId, basicSecret] = basic.split(":").map((part) => decodeURIComponent(part.replace(/\+/g, " ")));
      if (basicId !== clientId || basicSecret !== clientSecret || parameters.get("client_secret") !== null) return json(response, 401, { error: "invalid_client" });
      const grant = codes.get(parameters.get("code") ?? "");
      if (!grant || grant.used || parameters.get("grant_type") !== "authorization_code" || parameters.get("redirect_uri") !== redirectUri) return json(response, 400, { error: "invalid_grant" });
      grant.used = true;
      const challenge = createHash("sha256").update(parameters.get("code_verifier") ?? "").digest("base64url");
      if (challenge !== grant.challenge) return json(response, 400, { error: "invalid_grant" });
      const now = Math.floor(Date.now() / 1000);
      const signingKeys = overrides.signingKey === "unknown" ? unknownKeys : keys;
      let idToken = await new SignJWT({ sub: grant.subject, nonce: overrides.nonce ?? grant.nonce, ...(overrides.azp ? { azp: overrides.azp } : {}) })
        .setProtectedHeader({ alg: "RS256", kid: overrides.signingKey === "unknown" ? "unknown-key" : kid })
        .setIssuer(overrides.issuer ?? issuer)
        .setAudience(overrides.audience ?? clientId)
        .setIssuedAt(now)
        .setExpirationTime(now + (overrides.expiresIn ?? 300))
        .sign(signingKeys.privateKey);
      if (overrides.tamperToken) {
        const [header, payload, signature] = idToken.split(".");
        const claims = JSON.parse(Buffer.from(payload, "base64url").toString("utf8")) as Record<string, unknown>;
        claims.sub = "tampered-subject";
        idToken = `${header}.${Buffer.from(JSON.stringify(claims)).toString("base64url")}.${signature}`;
      }
      return json(response, 200, { access_token: randomBytes(32).toString("base64url"), token_type: "Bearer", expires_in: 300, id_token: idToken });
    }
    return json(response, 404, { error: "not_found" });
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("Synthetic OIDC provider has no loopback port");
  issuer = `http://127.0.0.1:${address.port}`;
  return {
    issuer,
    clientId,
    clientSecret,
    redirectUri,
    get subject() {
      return activeSubject;
    },
    set subject(value: string) {
      activeSubject = value;
    },
    overrides,
    get tokenRequests() {
      return tokenRequests;
    },
    get authorizationRequests() {
      return authorizationRequests;
    },
    close: () => new Promise<void>((resolve, reject) => server.close((error) => (error ? reject(error) : resolve()))),
  };
}
