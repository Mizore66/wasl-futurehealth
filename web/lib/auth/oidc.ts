import "server-only";

import * as client from "openid-client";
import { isLoopback, type OidcConfig } from "./oidc-config";

export type OidcProvider = client.Configuration;

export const OIDC_IDENTITY_PARAMETERS = ["actor", "role", "sub", "credential", "token", "api_token", "access_token", "id_token"] as const;

export function hasIdentityParameters(parameters: URLSearchParams): boolean {
  return OIDC_IDENTITY_PARAMETERS.some((name) => parameters.has(name));
}

/** A local app path only. Protocol-relative URLs, encoded backslashes and identity-bearing nested parameters are refused. */
export function safeReturnTo(value: string | null, requestOrigin: string): string {
  const fallback = "/app/review";
  if (!value || !value.startsWith("/app/") || value.startsWith("//") || value.includes("\\") || /%5c/i.test(value)) return fallback;
  try {
    const url = new URL(value, requestOrigin);
    if (url.origin !== requestOrigin || url.username || url.password || hasIdentityParameters(url.searchParams)) return fallback;
    return `${url.pathname}${url.search}${url.hash}`;
  } catch {
    return fallback;
  }
}

export function oidcFailureRedirect(request: Request, returnTo = "/app/review"): Response {
  const target = new URL(safeReturnTo(returnTo, new URL(request.url).origin), new URL(request.url).origin);
  target.searchParams.set("oidc_error", "1");
  return new Response(null, { status: 303, headers: { location: target.href, "cache-control": "no-store" } });
}

function requireEndpoint(value: unknown, name: string, insecureLoopback: boolean): string {
  if (typeof value !== "string") throw new Error(`OIDC discovery omitted ${name}`);
  const url = new URL(value);
  if (url.protocol !== "https:" && !(insecureLoopback && url.protocol === "http:" && isLoopback(url))) throw new Error(`OIDC ${name} must use HTTPS`);
  return url.href;
}

/** Discovery and metadata validation stay in openid-client; this adds the deployment's explicit RS256 and endpoint policy. */
export async function discoverProvider(config: OidcConfig): Promise<OidcProvider> {
  const provider = await client.discovery(
    new URL(config.issuer),
    config.clientId,
    { id_token_signed_response_alg: "RS256" },
    client.ClientSecretBasic(config.clientSecret),
    config.insecureLoopback ? { execute: [client.enableNonRepudiationChecks, client.allowInsecureRequests] } : { execute: [client.enableNonRepudiationChecks] },
  );
  const metadata = provider.serverMetadata();
  if (metadata.issuer !== config.issuer) throw new Error("OIDC discovery issuer mismatch");
  requireEndpoint(metadata.authorization_endpoint, "authorization endpoint", config.insecureLoopback);
  requireEndpoint(metadata.token_endpoint, "token endpoint", config.insecureLoopback);
  requireEndpoint(metadata.jwks_uri, "JWKS URI", config.insecureLoopback);
  if (!metadata.id_token_signing_alg_values_supported?.includes("RS256")) throw new Error("OIDC provider does not support RS256 ID tokens");
  return provider;
}

export function beginAuthorization(provider: OidcProvider, config: OidcConfig, input: { state: string; nonce: string; codeChallenge: string }): URL {
  const authorizationEndpoint = provider.serverMetadata().authorization_endpoint;
  if (!authorizationEndpoint) throw new Error("OIDC provider has no authorization endpoint");
  const url = new URL(authorizationEndpoint);
  /* Exactly the authorization-code parameters: no browser-selected identity, role, credential or token hints. */
  url.search = new URLSearchParams({
    response_type: "code",
    client_id: config.clientId,
    redirect_uri: config.redirectUri,
    scope: "openid",
    state: input.state,
    nonce: input.nonce,
    code_challenge: input.codeChallenge,
    code_challenge_method: "S256",
  }).toString();
  return url;
}

export async function exchangeAuthorizationCode(
  provider: OidcProvider,
  config: OidcConfig,
  callback: URL,
  transaction: { pkceVerifier: string; nonceHash: string },
  expectedState: string,
  expectedNonce: string,
): Promise<{ issuer: string; subject: string }> {
  const tokens = await client.authorizationCodeGrant(provider, callback, {
    expectedState,
    expectedNonce,
    idTokenExpected: true,
    pkceCodeVerifier: transaction.pkceVerifier,
  });
  const claims = tokens.claims();
  if (!claims || typeof claims.iss !== "string" || typeof claims.sub !== "string" || !claims.sub) throw new Error("OIDC ID token has no verified subject");
  if (claims.iss !== config.issuer) throw new Error("OIDC ID token issuer mismatch");
  if (claims.azp !== undefined && claims.azp !== config.clientId) throw new Error("OIDC ID token authorized party mismatch");
  /* Discard the response and all ID/access/refresh tokens immediately after extracting the verified identity. */
  return { issuer: claims.iss, subject: claims.sub };
}
