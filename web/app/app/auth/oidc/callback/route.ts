import { NextResponse } from "next/server";
import { mappedCredentialToken, oidcConfig, reviewAuthMode } from "@/lib/auth/oidc-config";
import { discoverProvider, exchangeAuthorizationCode, hasIdentityParameters, oidcFailureRedirect } from "@/lib/auth/oidc";
import { defaultSessionStore, nonceForVerifier, sha256 } from "@/lib/auth/oidc-store";
import { OIDC_REVIEW_COOKIE, REVIEW_COOKIE_PATH } from "@/components/review/auth";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const CALLBACK_PARAMETERS = new Set(["code", "state", "iss", "error", "error_description", "error_uri"]);

export async function GET(request: Request): Promise<Response> {
  const url = new URL(request.url);
  let returnTo = "/app/review";
  try {
    if (reviewAuthMode() !== "oidc" || hasIdentityParameters(url.searchParams)) return oidcFailureRedirect(request);
    for (const name of url.searchParams.keys()) {
      if (!CALLBACK_PARAMETERS.has(name) || url.searchParams.getAll(name).length !== 1) return oidcFailureRedirect(request);
    }
    if (url.searchParams.has("error")) return oidcFailureRedirect(request);
    const config = oidcConfig();
    const code = url.searchParams.get("code");
    const state = url.searchParams.get("state");
    if (!code || !state || (url.searchParams.has("iss") && url.searchParams.get("iss") !== config.issuer)) return oidcFailureRedirect(request);

    /* Consume state atomically before discovery or token exchange, making wrong/expired/replayed state fail closed. */
    const store = defaultSessionStore(config.sessionDb);
    const transaction = store.consumeLogin(state);
    if (!transaction) return oidcFailureRedirect(request);
    returnTo = transaction.returnTo;
    const nonce = nonceForVerifier(transaction.pkceVerifier);
    if (sha256(nonce) !== transaction.nonceHash) return oidcFailureRedirect(request, returnTo);

    const provider = await discoverProvider(config);
    const callback = new URL(config.redirectUri);
    callback.search = url.search;
    const identity = await exchangeAuthorizationCode(provider, config, callback, transaction, state, nonce);
    if (identity.issuer !== config.issuer || !mappedCredentialToken(config, identity.subject)) return oidcFailureRedirect(request, returnTo);
    const credential = config.subjectMap.get(identity.subject)!;
    const { sessionId, session } = store.createSession({ issuer: identity.issuer, subject: identity.subject, credential });

    const destination = new URL(returnTo, config.redirectUri);
    const response = NextResponse.redirect(destination, 303);
    response.headers.set("cache-control", "no-store");
    response.cookies.set(OIDC_REVIEW_COOKIE, sessionId, {
      httpOnly: true,
      secure: true,
      sameSite: "strict",
      path: REVIEW_COOKIE_PATH,
      maxAge: Math.max(1, Math.floor((session.expiresAt - Date.now()) / 1000)),
    });
    return response;
  } catch {
    return oidcFailureRedirect(request, returnTo);
  }
}
